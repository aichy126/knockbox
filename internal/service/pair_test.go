package service

import (
	"strings"
	"testing"
	"time"

	_ "github.com/aichy126/igo/db"
)

const testHost = "https://push.example.com"

// 接入页刷新时要能复用上一张码，否则 pair_code 的行数等于首页被打开的次数。
func TestPendingReusesUnusedOpenRegistrationCode(t *testing.T) {
	d, _ := newTestDAO(t)
	p := NewPair(d)
	issued, err := p.Issue(0, testHost, "public", 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	got, ok := p.Pending(issued.Code, testHost, time.Minute)
	if !ok {
		t.Fatal("刚签发的开放注册码应当可以复用")
	}
	if got.Code != issued.Code {
		t.Fatalf("复用到了别的码：%s ≠ %s", got.Code, issued.Code)
	}
	if got.DeepLink != issued.DeepLink {
		t.Errorf("深链对不上：%q ≠ %q", got.DeepLink, issued.DeepLink)
	}
	if !got.ExpiresAt.After(time.Now()) {
		t.Error("复用到的码已经过期了")
	}
}

// 用掉的、快过期的、属于某个成员的，都不能复用。
//
// 第三条尤其要守住：管理员签发的码一旦能在公开接入页上被复用，
// 扫码的人就会落进那个成员名下，而不是新建一个自己的身份。
func TestPendingRefusesUnreusableCodes(t *testing.T) {
	d, _ := newTestDAO(t)
	p := NewPair(d)
	uid := mustUser(t, d)

	used, err := p.Issue(0, testHost, "public", 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Engine().Exec("UPDATE pair_code SET used_at=? WHERE code=?",
		time.Now().Unix(), used.Code); err != nil {
		t.Fatal(err)
	}

	expiring, err := p.Issue(0, testHost, "public", 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Engine().Exec("UPDATE pair_code SET expires_at=? WHERE code=?",
		time.Now().Add(10*time.Second).Unix(), expiring.Code); err != nil {
		t.Fatal(err)
	}

	owned, err := p.Issue(uid, testHost, "web", 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct{ name, code string }{
		{"已用掉的码", used.Code},
		{"只剩几秒的码", expiring.Code},
		{"属于某个成员的码", owned.Code},
		{"根本不存在的码", "ZZZZZZZZ"},
		{"空串", ""},
	} {
		if _, ok := p.Pending(tc.code, testHost, time.Minute); ok {
			t.Errorf("%s 不该被复用", tc.name)
		}
	}
}

// 深链里的 host 必须转义。命令行配对走的就是这条路，
// 三处拼法各写一遍的话，其中没转义的那几处在 host 带参数时会拼出坏链接。
func TestDeepLinkEscapesHost(t *testing.T) {
	d, _ := newTestDAO(t)
	code, err := NewPair(d).Issue(0, testHost, "public", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(code.DeepLink, Scheme+"://pair?h=") {
		t.Fatalf("深链形状不对: %s", code.DeepLink)
	}
	if strings.Contains(code.DeepLink, "h="+testHost) {
		t.Errorf("host 没有转义: %s", code.DeepLink)
	}
	if !strings.Contains(code.DeepLink, "c="+code.Code) {
		t.Errorf("深链里没带配对码: %s", code.DeepLink)
	}
}
