// Package apns 直连 Apple 推送服务。
//
// 服务端自己持有 APNs 密钥，因此不经过任何中继：消息从这台服务器直接进
// api.push.apple.com。这也是正文可以不进 payload 的前提——payload 只带摘要与
// 消息 id，正文留在库里由客户端按需取。
package apns

import (
	"context"
	_ "embed"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/sideshow/apns2"
	"github.com/sideshow/apns2/token"
)

// 内置的公开密钥：topic-specific，只绑 com.miramiao.knockbox 这一个 bundle id。
//
// 随仓库公开是刻意的。APNs 的 device token 绑定在 bundle id 上，自建者若没有
// 这把密钥，就推不到 App Store 上的那个 app，等于必须自备 Apple 开发者账号。
// 选 topic-specific 而不是 Team 级密钥，是为了把影响范围锁死在这一个 bundle id：
// 同一开发者账号下的其它 app 用不了它。
//
// 沙箱密钥不在这里，它是私有的：只有自行用 Xcode Debug 构建时才需要。
// 不公开它也意味着这把生产密钥推不到任何人的 Debug 构建。
//
//go:embed embedded/AuthKey_TXYBPWPD2L.p8
var embeddedProductionKey []byte

const embeddedProductionKeyID = "TXYBPWPD2L"

// Config 一个环境的连接参数。
type Config struct {
	Topic     string
	TeamID    string
	KeyID     string
	KeyFile   string
	KeyBase64 string
}

// Client 一个进程一个实例。
//
// ⚠️ *token.Token 必须全局共享：它内部有 mutex 和 50 分钟的缓存窗口，正好落在
// Apple 要求的「签发间隔 > 20 分钟、token 年龄 < 60 分钟」之间。每次推送新建一个
// 会触发 403 TooManyProviderTokenUpdates，而这个错误在低频测试下不复现、放量才炸。
type Client struct {
	prod    *apns2.Client
	sandbox *apns2.Client
	topic   string
}

// New 按配置建客户端。production / sandbox 各自可以有自己的密钥——
// topic-specific 的密钥只能绑单一环境（Apple 的限制，保存后不可改），所以是两把。
func New(topic string, prod, sandbox Config) (*Client, error) {
	if topic == "" {
		return nil, errors.New("apns.topic is not set (it is the app's bundle id)")
	}
	c := &Client{topic: topic}

	pk, err := loadToken(prod, true)
	if err != nil {
		return nil, fmt.Errorf("production key: %w", err)
	}
	if pk != nil {
		// ⚠️ apns2.DefaultHost 是 Development。不显式 .Production() 的表现是
		// 400 BadDeviceToken —— 一个指向错误方向的错误码。
		c.prod = apns2.NewTokenClient(pk).Production()
	}

	sk, err := loadToken(sandbox, false)
	if err != nil {
		return nil, fmt.Errorf("sandbox key: %w", err)
	}
	if sk != nil {
		c.sandbox = apns2.NewTokenClient(sk).Development()
	}
	if c.prod == nil && c.sandbox == nil {
		return nil, errors.New("no usable APNs key")
	}
	return c, nil
}

// loadToken 取一个环境的密钥。优先级：key_base64 → key_file → 内置（仅生产环境）。
func loadToken(cfg Config, allowEmbedded bool) (*token.Token, error) {
	var raw []byte
	keyID := cfg.KeyID

	switch {
	case strings.TrimSpace(cfg.KeyBase64) != "":
		b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(cfg.KeyBase64))
		if err != nil {
			return nil, fmt.Errorf("cannot decode base64: %w", err)
		}
		raw = b
	case strings.TrimSpace(cfg.KeyFile) != "":
		b, err := os.ReadFile(cfg.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("cannot read %s: %w", cfg.KeyFile, err)
		}
		raw = b
	case allowEmbedded:
		raw, keyID = embeddedProductionKey, embeddedProductionKeyID
	default:
		return nil, nil // 沙箱没配就是没有，不是错误
	}

	if keyID == "" {
		return nil, errors.New("a key was given without its key_id")
	}
	if cfg.TeamID == "" {
		return nil, errors.New("apns.team_id is not set")
	}
	key, err := token.AuthKeyFromBytes(raw)
	if err != nil {
		return nil, fmt.Errorf("cannot parse the .p8 key: %w", err)
	}
	return &token.Token{AuthKey: key, KeyID: keyID, TeamID: cfg.TeamID}, nil
}

// Has 这个环境有没有配密钥。
func (c *Client) Has(env string) bool {
	if env == "sandbox" {
		return c.sandbox != nil
	}
	return c.prod != nil
}

// Environments 已装载的环境，用于启动日志——把当前跑在哪个环境打出来，
// 是排查「为什么收不到推送」最省时间的一行。
func (c *Client) Environments() []string {
	var out []string
	if c.prod != nil {
		out = append(out, "production")
	}
	if c.sandbox != nil {
		out = append(out, "sandbox")
	}
	return out
}

func (c *Client) clientFor(env string) *apns2.Client {
	if env == "sandbox" {
		return c.sandbox
	}
	return c.prod
}

// Result 一次推送的结果。
type Result struct {
	StatusCode int
	APNsID     string
	Reason     string
	// Timestamp 仅在 410 Unregistered 时有意义：Apple 认为这个 token 失效的时刻。
	Timestamp int64
	// SwitchedEnv 触发了环境纠偏并成功。调用方据此把设备的 apns_env 改过来。
	SwitchedEnv string
}

func (r Result) OK() bool { return r.StatusCode == 200 }

// Push 发一条。
//
// 环境自动纠偏：生产推送收到 400 BadDeviceToken 时用另一个环境重试一次。
// 用户很容易把环境搞混，而这一条能省掉自建场景下大部分「为什么收不到推送」。
func (c *Client) Push(ctx context.Context, env string, n *apns2.Notification) (Result, error) {
	n.Topic = c.topic
	cl := c.clientFor(env)
	if cl == nil {
		return Result{}, fmt.Errorf("no APNs key configured for the %s environment", env)
	}
	resp, err := cl.PushWithContext(ctx, n)
	if err != nil {
		return Result{}, err
	}
	r := Result{StatusCode: resp.StatusCode, APNsID: resp.ApnsID, Reason: resp.Reason}
	if !resp.Timestamp.IsZero() {
		r.Timestamp = resp.Timestamp.Unix()
	}
	if r.OK() || resp.Reason != apns2.ReasonBadDeviceToken {
		return r, nil
	}

	other := "sandbox"
	if env == "sandbox" {
		other = "production"
	}
	if alt := c.clientFor(other); alt != nil {
		resp2, err2 := alt.PushWithContext(ctx, n)
		if err2 == nil && resp2.Sent() {
			return Result{StatusCode: resp2.StatusCode, APNsID: resp2.ApnsID, SwitchedEnv: other}, nil
		}
	}
	return r, nil
}
