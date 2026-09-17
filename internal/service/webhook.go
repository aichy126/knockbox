package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"syscall"
	"time"

	"github.com/aichy126/igo/log"
	"github.com/aichy126/knockbox/internal/dao"
	"github.com/aichy126/knockbox/internal/models"
)

// 回调的退避梯度。比推送短得多：发送方多半正卡在一个分支上等这个答案，
// 半小时之后才送到的「不要打开」已经没有意义了。
var hookBackoff = []time.Duration{5 * time.Second, 30 * time.Second, 2 * time.Minute}

// SignatureHeader 回调的签名头。接收方用频道 token 作密钥，
// 对整个请求体算 HMAC-SHA256 即可验证。
const SignatureHeader = "X-Knockbox-Signature"

// Webhook 消费 reply_hook 队列，把用户的回复投给发送方。
//
// ⚠️ **和推送队列一样，同一个数据库上只能跑一个实例**：claim 没有租约，
// 靠「一个进程只有一条 Run 循环」保证不重复投递。回调是有副作用的请求
// （对面可能真的去开窗帘），重复投递比重复推送严重得多。
type Webhook struct {
	d           *dao.DAO
	cl          *http.Client
	maxAttempts int
	wake        chan struct{}
	// allowPrivate 允不允许把回调打到私网地址。
	//
	// 自建模式下就是要能打内网：服务器在你自己的网里，Home Assistant 也在。
	// 公共实例上必须关：否则任何拿到发送 token 的陌生人都能让这台服务器
	// 去访问它内网里的任意地址，这是标准的 SSRF。
	allowPrivate func() bool
}

type WebhookConfig struct {
	Timeout     time.Duration
	MaxAttempts int
	// AllowPrivate 运行时判定，不是启动时的快照：
	// 公共模式可以在后台开关，而这个 worker 是常驻的。
	AllowPrivate func() bool
}

func NewWebhook(d *dao.DAO, cfg WebhookConfig) *Webhook {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Second
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 3
	}
	if cfg.AllowPrivate == nil {
		cfg.AllowPrivate = func() bool { return true }
	}
	w := &Webhook{
		d: d, maxAttempts: cfg.MaxAttempts,
		wake: make(chan struct{}, 1), allowPrivate: cfg.AllowPrivate,
	}
	dialer := &net.Dialer{
		Timeout: cfg.Timeout,
		// Control 在【拨号那一刻】拿到真实 IP 再判一次。
		//
		// 只在发送时用 LookupIP 查一遍是不够的：DNS 可以在两次查询之间换一个答案
		// （DNS rebinding），第一次回公网 IP 过检查，真正连的时候回 127.0.0.1。
		// 这里判的是即将连上的那个地址，绕不过去 —— 所以安全边界在这里，
		// 发送时那次检查只是为了让发送方早点知道。
		Control: func(_, address string, _ syscall.RawConn) error {
			if w.allowPrivate() {
				return nil
			}
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			ip := net.ParseIP(host)
			if ip == nil {
				return fmt.Errorf("cannot resolve %s", address)
			}
			if isPrivateIP(ip) {
				return errHookPrivateAddr
			}
			return nil
		},
	}
	w.cl = &http.Client{
		Timeout: cfg.Timeout,
		// 不跟随重定向。
		//
		// 跟随的话，上面那道私网检查形同虚设：一个公网地址回一个 302 指向
		// 169.254.169.254 就绕过去了。而回调本来也不需要重定向 ——
		// 接收方是发送方自己写的端点，它知道自己的地址。
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: &http.Transport{
			DialContext:         dialer.DialContext,
			DisableKeepAlives:   true, // 每条回调都是一次性的，连接池留着没用
			TLSHandshakeTimeout: cfg.Timeout,
		},
	}
	return w
}

var errHookPrivateAddr = errors.New("this instance does not allow webhooks to private addresses")

// isPrivateIP 判定一个地址是不是不该从公共实例访问的。
//
// 覆盖：回环、私有网段、链路本地（含云厂商的 169.254.169.254 元数据服务）、
// 未指定地址、组播、以及 IPv6 的唯一本地地址。
// IPv4-mapped 的 IPv6 地址要先还原成 IPv4 再判，否则 ::ffff:127.0.0.1 会漏过去。
func isPrivateIP(ip net.IP) bool {
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() ||
		ip.IsInterfaceLocalMulticast()
}

// CheckAddr 在【发送那一刻】先判一次地址。
//
// 投递时还会在拨号点再判一次（那次才是安全边界）。这里判是为了让发送方
// 立刻收到「这个地址不行」，而不是等用户在手机上回复完、回调却投不出去 ——
// 那时候错误只有服务器管理员看得到，发送方一直在等一个永远不来的答案。
func (w *Webhook) CheckAddr(rawURL string) error {
	if w.allowPrivate() {
		return nil
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("reply.webhook is not a valid URL: %w", err)
	}
	host := u.Hostname()
	if ip := net.ParseIP(host); ip != nil {
		if isPrivateIP(ip) {
			return errHookPrivateAddr
		}
		return nil
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("cannot parse the host in reply.webhook: %w", err)
	}
	for _, ip := range ips {
		if isPrivateIP(ip) {
			return errHookPrivateAddr
		}
	}
	return nil
}

// Notify 叫醒一轮。回复落库之后调它，用户按下按钮到发送方收到之间不多等一次轮询。
func (w *Webhook) Notify() {
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

// Run 常驻循环。30 秒兜底扫一次，捡起重试到期的和进程重启前没投完的。
func (w *Webhook) Run(ctx context.Context) {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		if n, err := w.drain(ctx); err != nil {
			log.Error("webhook queue failed", log.Any("error", err.Error()))
		} else if n > 0 {
			log.Info("webhook delivered", log.Any("count", n))
		}
		select {
		case <-ctx.Done():
			return
		case <-w.wake:
		case <-t.C:
		}
	}
}

func (w *Webhook) drain(ctx context.Context) (int, error) {
	total := 0
	for {
		var hooks []models.ReplyHook
		if err := w.d.Engine().
			Where("status IN (?, ?) AND next_at <= ?", models.HookPending, models.HookRetry, time.Now().Unix()).
			OrderBy("id").Limit(100).Find(&hooks); err != nil {
			return total, err
		}
		if len(hooks) == 0 {
			return total, nil
		}
		// 串行投递，不像推送那样并发。
		//
		// 回调的量级是「用户按了几次按钮」，一分钟能有几条就不错了，并发省不下什么；
		// 而串行让一个慢端点只拖慢它自己那条，不占住一批 goroutine。
		for i := range hooks {
			if ctx.Err() != nil {
				return total, nil
			}
			w.deliver(ctx, &hooks[i])
			total++
		}
	}
}

func (w *Webhook) deliver(ctx context.Context, h *models.ReplyHook) {
	code, err := w.post(ctx, h)
	now := time.Now().Unix()
	if err == nil {
		_, _ = w.d.Engine().Exec(
			"UPDATE reply_hook SET status = ?, status_code = ?, error = '', attempt = attempt + 1, updated_at = ? WHERE id = ?",
			models.HookOK, code, now, h.Id)
		return
	}

	attempt := h.Attempt + 1
	msg := err.Error()
	// 私网地址在公共实例上是策略拒绝，不是暂时故障，重试多少次都一样。
	// 直接置终态，免得它在队列里反复被捞出来又丢掉。
	if errors.Is(err, errHookPrivateAddr) || attempt >= w.maxAttempts {
		_, _ = w.d.Engine().Exec(
			"UPDATE reply_hook SET status = ?, status_code = ?, error = ?, attempt = ?, updated_at = ? WHERE id = ?",
			models.HookAbandon, code, msg, attempt, now, h.Id)
		// 这条日志是排障的唯一线索：用户那边显示「已回复」，而发送方什么都没收到，
		// 两边都不会自己发现这件事。
		log.Warn("webhook delivery given up",
			log.Any("hook", h.Id), log.Any("url", h.URL),
			log.Any("attempt", attempt), log.Any("error", msg))
		return
	}
	delay := hookBackoff[min(attempt-1, len(hookBackoff)-1)]
	_, _ = w.d.Engine().Exec(
		"UPDATE reply_hook SET status = ?, status_code = ?, error = ?, attempt = ?, next_at = ?, updated_at = ? WHERE id = ?",
		models.HookRetry, code, msg, attempt, now+int64(delay.Seconds()), now, h.Id)
}

// post 投一次。返回 HTTP 状态码（拿不到时为 0）和失败原因。
func (w *Webhook) post(ctx context.Context, h *models.ReplyHook) (int, error) {
	body := []byte(h.Payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.URL, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Knockbox")
	req.Header.Set(SignatureHeader, "sha256="+signHook([]byte(h.Secret), body))

	resp, err := w.cl.Do(req)
	if err != nil {
		// Transport.Control 返回的错误被 url.Error 包着，Is 能穿过去。
		if errors.Is(err, errHookPrivateAddr) {
			return 0, errHookPrivateAddr
		}
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	// 把响应体读掉一小段再丢。不读的话连接不能复用，而且有些服务端会因为
	// 请求方提前关闭而在自己那边记一条错误——回调成功了却让对面看到异常，没必要。
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))

	// 2xx 和 3xx 都算收到了。
	// 3xx 算成功是因为我们不跟随重定向：对面回 302 说明它收到了请求并做出了响应，
	// 把它当失败去重试，只会让同一个回调被投好几遍。
	if resp.StatusCode < 400 {
		return resp.StatusCode, nil
	}
	return resp.StatusCode, fmt.Errorf("webhook returned %d", resp.StatusCode)
}

func signHook(key, body []byte) string {
	m := hmac.New(sha256.New, key)
	m.Write(body)
	return hex.EncodeToString(m.Sum(nil))
}
