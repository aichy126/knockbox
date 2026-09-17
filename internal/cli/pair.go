package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/aichy126/knockbox/internal/service"
	"github.com/mdp/qrterminal/v3"
	"github.com/spf13/cobra"
)

func newPairCmd() *cobra.Command {
	var username string
	var ttl time.Duration
	c := &cobra.Command{
		Use:   "pair",
		Short: "签发配对码，直接在终端里打出二维码",
		Long: "签发一次性配对码，并在终端里打出二维码给 app 扫。\n\n" +
			"不需要先起服务、再开浏览器——这是第一次接入和「手机丢了」时的主路径。",
		Example: "  knockbox pair\n" +
			"  knockbox pair --user alice --ttl 1h",
		RunE: func(cmd *cobra.Command, args []string) error {
			app, d, err := openDAO()
			if err != nil {
				return err
			}
			acc := service.NewAccount(d)

			var userID int64
			if username != "" {
				u, err := acc.GetByUsername(username)
				if err != nil {
					return fmt.Errorf("账号 %q 不存在。看看有哪些：knockbox user list", username)
				}
				userID = u.Id
			} else {
				us, err := acc.List()
				if err != nil {
					return err
				}
				if len(us) == 0 {
					return fmt.Errorf("还没有任何账号。启动一次服务（knockbox serve）会自动建出管理员，或者手动建：knockbox user add <用户名>")
				}
				if len(us) > 1 {
					return fmt.Errorf("有 %d 个账号，用 --user 指明给谁配对（knockbox user list 可以看）", len(us))
				}
				userID = us[0].Id
			}

			host := app.Conf.GetString("server.external_url")
			if ttl <= 0 {
				ttl = time.Duration(app.Conf.GetInt("server.pair_ttl")) * time.Second
			}
			p, err := service.NewPair(d).Issue(userID, host, "cli", ttl)
			if err != nil {
				return err
			}

			fmt.Println()
			qrterminal.GenerateHalfBlock(p.DeepLink, qrterminal.L, os.Stdout)
			fmt.Printf("\n  服务器   %s\n", host)
			fmt.Printf("  配对码   %s\n", p.Display)
			fmt.Printf("  有效期   %s 之前（%s）\n",
				p.ExpiresAt.Format("15:04:05"), time.Until(p.ExpiresAt).Round(time.Second))
			fmt.Printf("\n  用 Knockbox 扫上面的二维码；扫不了就在 app 里手动填服务器地址和配对码。\n")
			fmt.Printf("  配对码只能用一次。\n\n")
			return nil
		},
	}
	c.Flags().StringVar(&username, "user", "", "给哪个账号配对（只有一个账号时可省略）")
	c.Flags().DurationVar(&ttl, "ttl", 0, "有效期，留空用配置里的 server.pair_ttl")
	return c
}
