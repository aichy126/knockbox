package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aichy126/knockbox/internal/models"
)

// 回调要真的送到，而且签名要能验。
// 签名用频道 token 做密钥，是因为双方本来就都持有它，不必再发一把新的。
func TestWebhookDeliversSignedPayload(t *testing.T) {
	d := newDAO(t)
	ch := fixture(t, d, 1, false)

	var gotBody []byte
	var gotSig string
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		gotBody, _ = io.ReadAll(r.Body)
		gotSig = r.Header.Get(SignatureHeader)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	spec := choiceSpec()
	spec.Webhook = srv.URL
	uid := replyable(t, d, ch, spec)
	if _, err := NewReply(d).Submit(ch.UserId, uid, "不要打开"); err != nil {
		t.Fatal(err)
	}

	// httptest 起在 127.0.0.1，所以这里必须允许私网——这正是自建模式的配置。
	w := NewWebhook(d, WebhookConfig{AllowPrivate: func() bool { return true }})
	if _, err := w.drain(context.Background()); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 1 {
		t.Fatalf("回调应被调用一次，实际 %d 次", hits.Load())
	}

	mac := hmac.New(sha256.New, []byte(ch.Token))
	mac.Write(gotBody)
	want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if gotSig != want {
		t.Errorf("签名对不上\n收到 %s\n期望 %s", gotSig, want)
	}

	var hooks []models.ReplyHook
	if err := d.Engine().Find(&hooks); err != nil {
		t.Fatal(err)
	}
	if hooks[0].Status != models.HookOK {
		t.Errorf("投递成功后状态应是 HookOK，得到 %d（error=%s）", hooks[0].Status, hooks[0].Error)
	}
}

// 失败要重试，但有上限；到顶了置终态，不能永远在队列里被捞出来又丢掉。
func TestWebhookRetriesThenGivesUp(t *testing.T) {
	d := newDAO(t)
	ch := fixture(t, d, 1, false)

	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	spec := choiceSpec()
	spec.Webhook = srv.URL
	uid := replyable(t, d, ch, spec)
	if _, err := NewReply(d).Submit(ch.UserId, uid, "打开"); err != nil {
		t.Fatal(err)
	}

	w := NewWebhook(d, WebhookConfig{
		MaxAttempts:  3,
		AllowPrivate: func() bool { return true },
	})
	// 每轮把 next_at 拨到现在，跳过退避等待——这里要测的是「到顶了会不会停」，
	// 不是退避本身有没有等够。
	for i := 0; i < 5; i++ {
		if _, err := d.Engine().Exec("UPDATE reply_hook SET next_at = 0"); err != nil {
			t.Fatal(err)
		}
		if _, err := w.drain(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if got := hits.Load(); got != 3 {
		t.Errorf("应恰好尝试 3 次后放弃，实际 %d 次", got)
	}
	var hooks []models.ReplyHook
	if err := d.Engine().Find(&hooks); err != nil {
		t.Fatal(err)
	}
	if hooks[0].Status != models.HookAbandon {
		t.Errorf("到上限后应置为 HookAbandon，得到 %d", hooks[0].Status)
	}
	// 原因要留下来：用户那边显示「已回复」、发送方什么都没收到，
	// 两边都不会自己发现这件事，后台那一行是唯一线索。
	if hooks[0].Error == "" || hooks[0].StatusCode != 500 {
		t.Errorf("应记下失败原因与状态码，得到 code=%d error=%q", hooks[0].StatusCode, hooks[0].Error)
	}
}

// 公共实例上不许把回调打到内网。
// 这是 SSRF：任何拿到一个发送 token 的陌生人，都能让这台服务器去访问它内网里的地址。
func TestWebhookRefusesPrivateAddressInPublicMode(t *testing.T) {
	d := newDAO(t)
	ch := fixture(t, d, 1, false)

	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	spec := choiceSpec()
	spec.Webhook = srv.URL // httptest 一定在 127.0.0.1
	uid := replyable(t, d, ch, spec)
	if _, err := NewReply(d).Submit(ch.UserId, uid, "打开"); err != nil {
		t.Fatal(err)
	}

	w := NewWebhook(d, WebhookConfig{AllowPrivate: func() bool { return false }})
	if _, err := w.drain(context.Background()); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 0 {
		t.Fatalf("公共模式下不该连上内网地址，实际被调用 %d 次", hits.Load())
	}
	var hooks []models.ReplyHook
	if err := d.Engine().Find(&hooks); err != nil {
		t.Fatal(err)
	}
	// 策略拒绝是终态，不是暂时故障：重试多少次都一样，
	// 留在队列里只会每轮被捞出来再丢掉。
	if hooks[0].Status != models.HookAbandon {
		t.Errorf("策略拒绝应直接置终态，得到 status=%d", hooks[0].Status)
	}
}

// 自建模式就是要能打内网：服务器在你自己的网里，接收方多半也在。
func TestWebhookAllowsPrivateAddressWhenSelfHosted(t *testing.T) {
	d := newDAO(t)
	ch := fixture(t, d, 1, false)

	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	spec := choiceSpec()
	spec.Webhook = srv.URL
	uid := replyable(t, d, ch, spec)
	if _, err := NewReply(d).Submit(ch.UserId, uid, "打开"); err != nil {
		t.Fatal(err)
	}
	w := NewWebhook(d, WebhookConfig{AllowPrivate: func() bool { return true }})
	if _, err := w.drain(context.Background()); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 1 {
		t.Errorf("自建模式应当打得通内网地址，实际被调用 %d 次", hits.Load())
	}
}

// 不跟随重定向。跟随的话，私网检查形同虚设：
// 一个公网地址回一个 302 指向 169.254.169.254 就绕过去了。
func TestWebhookDoesNotFollowRedirects(t *testing.T) {
	d := newDAO(t)
	ch := fixture(t, d, 1, false)

	var target atomic.Int32
	dest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		target.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer dest.Close()
	front := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, dest.URL, http.StatusFound)
	}))
	defer front.Close()

	spec := choiceSpec()
	spec.Webhook = front.URL
	uid := replyable(t, d, ch, spec)
	if _, err := NewReply(d).Submit(ch.UserId, uid, "打开"); err != nil {
		t.Fatal(err)
	}
	w := NewWebhook(d, WebhookConfig{AllowPrivate: func() bool { return true }})
	if _, err := w.drain(context.Background()); err != nil {
		t.Fatal(err)
	}
	if target.Load() != 0 {
		t.Errorf("不该跟随重定向，重定向目标被访问了 %d 次", target.Load())
	}
	// 3xx 算「收到了」：对面做出了响应。当失败去重试只会把同一个回调投好几遍。
	var hooks []models.ReplyHook
	if err := d.Engine().Find(&hooks); err != nil {
		t.Fatal(err)
	}
	if hooks[0].Status != models.HookOK {
		t.Errorf("3xx 应当算投递完成，得到 status=%d", hooks[0].Status)
	}
}

// 发送时先判一次地址，让发送方立刻知道地址不行——
// 而不是等用户在手机上回复完，回调却在服务端静默失败。
func TestCheckAddrRejectsPrivateBeforeSending(t *testing.T) {
	d := newDAO(t)
	pub := NewWebhook(d, WebhookConfig{AllowPrivate: func() bool { return false }})
	for _, u := range []string{
		"http://127.0.0.1:8123/hook",
		"http://192.168.1.10/api/webhook",
		"http://169.254.169.254/latest/meta-data/",
		"http://[::1]:9000/hook",
	} {
		if err := pub.CheckAddr(u); err == nil {
			t.Errorf("公共模式下应拒绝 %s", u)
		}
	}
	// 自建模式一律放行，这些地址正是它要打的
	self := NewWebhook(d, WebhookConfig{AllowPrivate: func() bool { return true }})
	if err := self.CheckAddr("http://192.168.1.10/api/webhook"); err != nil {
		t.Errorf("自建模式不该拦内网地址: %v", err)
	}
}

// IPv4-mapped 的 IPv6 地址要先还原再判，否则 ::ffff:127.0.0.1 会漏过去。
func TestIsPrivateIPCoversMappedV4(t *testing.T) {
	for _, s := range []string{
		"127.0.0.1", "10.0.0.1", "192.168.0.1", "172.16.0.1",
		"169.254.169.254", "::1", "fe80::1", "::ffff:127.0.0.1", "::ffff:10.0.0.1",
		"0.0.0.0", "224.0.0.1",
	} {
		if !isPrivateIP(net.ParseIP(s)) {
			t.Errorf("%s 应被判为内网/特殊地址", s)
		}
	}
	for _, s := range []string{"1.1.1.1", "8.8.8.8", "2606:4700::1111"} {
		if isPrivateIP(net.ParseIP(s)) {
			t.Errorf("%s 是公网地址，不该被拦", s)
		}
	}
}

// 慢端点不能把 worker 拖住。
func TestWebhookTimesOut(t *testing.T) {
	d := newDAO(t)
	ch := fixture(t, d, 1, false)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	spec := choiceSpec()
	spec.Webhook = srv.URL
	uid := replyable(t, d, ch, spec)
	if _, err := NewReply(d).Submit(ch.UserId, uid, "打开"); err != nil {
		t.Fatal(err)
	}
	w := NewWebhook(d, WebhookConfig{
		Timeout: 200 * time.Millisecond, MaxAttempts: 1,
		AllowPrivate: func() bool { return true },
	})
	start := time.Now()
	if _, err := w.drain(context.Background()); err != nil {
		t.Fatal(err)
	}
	if el := time.Since(start); el > time.Second {
		t.Errorf("应在超时时间内返回，实际耗时 %v", el)
	}
}
