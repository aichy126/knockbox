package cli

import (
	"time"

	"github.com/aichy126/igo/log"
	"github.com/aichy126/igo/web"
	"github.com/aichy126/knockbox/internal/api"
	"github.com/aichy126/knockbox/internal/dao"
	kapns "github.com/aichy126/knockbox/internal/library/apns"
	"github.com/aichy126/knockbox/internal/migrate"
	"github.com/aichy126/knockbox/internal/service"
	"github.com/spf13/cobra"
)

func newServeCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "serve",
		Short:   "Start the server",
		Example: "  knockbox serve -c /etc/knockbox/config.toml",
		RunE: func(cmd *cobra.Command, args []string) error {
			app, engine, err := openApp()
			if err != nil {
				return err
			}
			ver, _ := migrate.Applied(engine)
			log.Info("schema ready", log.Any("migration_version", ver))

			conf := app.Conf
			d := dao.New(engine)

			// 库里一个账号都没有时，自动建出第一个管理员并把凭据打在屏幕上。
			// 为什么不让人自己跑 `user add`：见 service.EnsureFirstAdmin 的注释，
			// 简而言之是「进容器敲命令」这道门槛会把非程序员挡在登录页外面。
			first, err := service.NewAccount(d).EnsureFirstAdmin(
				conf.GetString("bootstrap.username"),
				conf.GetString("bootstrap.password"))
			if err != nil {
				return err
			}
			if first != nil {
				printFirstAdmin(first, conf.GetString("server.external_url"))
			}
			notify := func() {}

			// 一律用完整 key 读配置：viper 的 AutomaticEnv 只在按完整 key 调 Get* 时生效，
			// GetStringMap 再 decode 会【静默】吞掉环境变量覆盖。
			settings := service.NewSettings(d, service.Defaults{
				PublicEnabled:   conf.GetBool("public.enabled"),
				RegisterPerHour: conf.GetInt("public.register_per_hour"),
				MaxChannels:     conf.GetInt("public.max_channels"),
				MaxPerDay:       conf.GetInt("public.max_messages_per_day"),
				RetentionDays:   conf.GetInt("public.retention_days"),
				SiteName:        conf.GetString("public.site_name"),
			})
			files := service.NewFile(d,
				conf.GetString("storage.dir"),
				conf.GetInt("storage.max_upload_mb"),
				conf.GetInt("storage.image_max_px"),
				conf.GetInt("storage.thumb_max_px"),
				conf.GetInt("storage.jpeg_quality"),
				conf.GetString("server.external_url"),
				conf.GetInt64("storage.url_ttl"))

			if conf.GetBool("apns.enabled") {
				cl, err := kapns.New(
					conf.GetString("apns.topic"),
					kapns.Config{
						TeamID:    conf.GetString("apns.team_id"),
						KeyID:     conf.GetString("apns.production.key_id"),
						KeyFile:   conf.GetString("apns.production.key_file"),
						KeyBase64: conf.GetString("apns.production.key_base64"),
					},
					kapns.Config{
						TeamID:    conf.GetString("apns.team_id"),
						KeyID:     conf.GetString("apns.sandbox.key_id"),
						KeyFile:   conf.GetString("apns.sandbox.key_file"),
						KeyBase64: conf.GetString("apns.sandbox.key_base64"),
					})
				if err != nil {
					return err
				}
				// 把当前跑在哪个环境打出来：忘了 .Production() 的表现是
				// 400 BadDeviceToken，一个指向错误方向的错误码，这一行能省很多时间。
				log.Info("APNs ready",
					log.Any("topic", conf.GetString("apns.topic")),
					log.Any("environments", cl.Environments()))

				p := service.NewPusher(d, cl, files, service.PusherConfig{
					Concurrency: conf.GetInt("apns.concurrency"),
					MaxAttempts: conf.GetInt("apns.max_attempts"),
					Expiration:  time.Duration(conf.GetInt("apns.default_expiration")) * time.Second,
					Timeout:     time.Duration(conf.GetInt("apns.timeout_ms")) * time.Millisecond,
				})
				go p.Run(app.GetShutdownContext())
				notify = p.Notify
			} else {
				log.Warn("APNs is off: messages are stored but not pushed")
			}

			// 回复的回调投递。
			//
			// allow_private 在【公共模式下】才有意义：自建的服务器就在你自己的网里，
			// Home Assistant 这类接收方多半也在，不许打内网等于这个功能没法用。
			// 公共实例上则必须关：否则任何拿到一个发送 token 的陌生人，
			// 都能让这台服务器去访问它内网里的任意地址。
			// 传函数而不是布尔：公共模式能在后台开关，而 worker 是常驻的。
			hooks := service.NewWebhook(d, service.WebhookConfig{
				Timeout:     time.Duration(conf.GetIntWithDefault("reply.webhook_timeout_ms", 5000)) * time.Millisecond,
				MaxAttempts: conf.GetIntWithDefault("reply.webhook_max_attempts", 3),
				AllowPrivate: func() bool {
					if !settings.PublicEnabled() {
						return true
					}
					return conf.GetBool("reply.public_allow_private")
				},
			})
			go hooks.Run(app.GetShutdownContext())

			// 保留策略与附件回收。每小时一轮，启动时先跑一次——
			// 服务可能停了很久，回来时该把欠的补上。
			go service.NewGC(d, files, settings, conf.GetInt("retention.days")).
				Run(app.GetShutdownContext())

			// 自动备份。VACUUM INTO 一句话拿一致性快照且不锁写——
			// 直接 cp 数据库文件是错的，WAL 里的内容不在那个文件里。
			if h := conf.GetInt("retention.backup_every_hours"); h > 0 {
				dir := conf.GetString("retention.backup_dir")
				if dir == "" {
					dir = "./data/backup"
				}
				go service.NewBackup(d, dir, conf.GetInt("retention.backup_keep"), h).
					Run(app.GetShutdownContext())
			}

			app.EnableHealthCheck()
			app.Web.Router.Use(web.Cors())
			api.Router(app.Web.Router, &api.Server{
				Name:        conf.GetString("server.name"),
				Version:     Version,
				ExternalURL: conf.GetString("server.external_url"),
				DAO:         d,
				Notify:      notify,
				NotifyHook:  hooks.Notify,
				Hooks:       hooks,
				Files:       files,
				Settings:    settings,
				// 三个数值在启动时定。缺项时 api.Router 会兜上默认值，
				// 那几个默认值和 config.toml.example 里写的一致。
				PairTTL:      time.Duration(conf.GetInt("server.pair_ttl")) * time.Second,
				PairPerMin:   conf.GetInt("limit.pair_per_min"),
				BodyMaxBytes: int64(conf.GetInt("limit.body_max_kb")) << 10,
				// 用 WithDefault 而不是 GetInt：键【不存在】时要开着限流
				// （老的 config.toml 里没有这两行，不该因此完全不设防），
				// 而显式写 send_qps = 0 才表示关闭。GetInt 分不出这两种情况。
				SendQPS:   conf.GetIntWithDefault("limit.send_qps", 20),
				SendBurst: conf.GetIntWithDefault("limit.send_burst", 60),
				Quota: service.NewQuota(d, service.Limits{
					Enabled:       conf.GetBool("public.enabled"),
					MaxChannels:   conf.GetInt("public.max_channels"),
					MaxPerDay:     conf.GetInt("public.max_messages_per_day"),
					RetentionDays: conf.GetInt("public.retention_days"),
				}),
				Public: api.PublicConfig{
					Enabled:         conf.GetBool("public.enabled"),
					RegisterPerHour: conf.GetInt("public.register_per_hour"),
					MaxChannels:     conf.GetInt("public.max_channels"),
					MaxPerDay:       conf.GetInt("public.max_messages_per_day"),
					RetentionDays:   conf.GetInt("public.retention_days"),
					SiteName:        conf.GetString("public.site_name"),
					DocsURL:         conf.GetString("public.docs_url"),
				},
			})
			return app.Run()
		},
	}
}
