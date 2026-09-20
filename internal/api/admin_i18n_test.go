package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode"

	"github.com/aichy126/knockbox/internal/api/web"
	"github.com/aichy126/knockbox/internal/service"
)

// seedChannelAndMessage 造一个频道和一条消息，好让详情页有东西可渲染。
// 内容刻意全用英文——扫汉字的那条断言要能分得清「页面自己的文案」
// 和「我们塞进去的数据」。
func seedChannelAndMessage(t *testing.T, s *Server, member int64) (channel, msg string) {
	t.Helper()
	ch, err := service.NewChannel(s.DAO).Create(member, service.ChannelInput{})
	if err != nil {
		t.Fatal(err)
	}
	out, err := service.NewSend(s.DAO).Deliver(ch, service.SendInput{Title: "a message"}, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	return ch.Id, out.UID
}

// adminPages 后台会渲染 HTML 的每一页。新增页面时加进来——
// 少一条，就少一页被下面两个断言盯着。
func adminPages(member int64, channel, msg string) []string {
	return []string{
		"/admin",
		"/admin/users",
		"/admin/users/" + itoa(member),
		"/admin/messages",
		"/admin/messages/" + msg,
		"/admin/channels/" + channel,
		"/admin/pair",
		"/admin/settings",
		"/admin/settings?tab=server",
	}
}

// 英文界面里不该出现一个汉字。
//
// 这一条是整次提取的守门员：admin_pages.go 那一批字符串是手工搬进语料的，
// 漏一句在中文下完全看不出来——它本来就是中文。只有拿英文渲染一遍再扫汉字，
// 漏掉的那句才会自己跳出来。
//
// 它也防以后：谁再往这些页面里写死一句中文，这里立刻红。
func TestAdminPagesHaveNoChineseInEnglish(t *testing.T) {
	s, r, cookie, member := adminWith(t, "someone")
	channel, msg := seedChannelAndMessage(t, s, member)

	for _, p := range append(adminPages(member, channel, msg), "/login") {
		t.Run(p, func(t *testing.T) {
			body := visibleText(langGet(t, r, p, cookie, "en").Body.String())
			if han := firstHan(body); han != "" {
				t.Errorf("英文页面里出现了汉字：%q\n（有一句文案没搬进 locales）", han)
			}
		})
	}
}

// 反过来：cookie 说中文，页面就得真的是中文。
// 上面那条断言的是「没漏」，这条断言的是「真的在按语言切」——
// 只有前者的话，把所有文案都写成英文硬编码也能过。
func TestAdminPagesFollowLanguageCookie(t *testing.T) {
	_, r, cookie, _ := adminWith(t, "someone")
	body := langGet(t, r, "/admin", cookie, "zh").Body.String()
	for _, want := range []string{"概览", "成员", "设置"} {
		if !strings.Contains(body, want) {
			t.Errorf("cookie=zh 的后台里没有 %q", want)
		}
	}
}

// 语言开关本身：认得的语言才落 cookie，next 只能回到本站。
func TestLangSwitch(t *testing.T) {
	_, r := newServerWith(t, false, 0, nil)

	t.Run("认得的语言会记下来", func(t *testing.T) {
		w := plainGet(t, r, "/lang?to=zh&next=/admin/users")
		if !strings.Contains(w.Header().Get("Set-Cookie"), web.LangCookie+"=zh") {
			t.Errorf("没有落 cookie：%q", w.Header().Get("Set-Cookie"))
		}
		if got := w.Header().Get("Location"); got != "/admin/users" {
			t.Errorf("跳回了 %q，应当是 /admin/users", got)
		}
	})

	t.Run("不认得的语言不落 cookie", func(t *testing.T) {
		w := plainGet(t, r, "/lang?to=zzz&next=/admin")
		if c := w.Header().Get("Set-Cookie"); strings.Contains(c, web.LangCookie+"=") {
			t.Errorf("给一个不存在的语言落了 cookie：%q", c)
		}
	})

	// 不校验 next 的话，这条路由就是一个挂在你域名下的开放重定向。
	for _, bad := range []string{
		"https://evil.example",
		"//evil.example",
		"/\\evil.example",
		"http://evil.example/x",
	} {
		t.Run("拒绝跳到站外 "+bad, func(t *testing.T) {
			w := plainGet(t, r, "/lang?to=en&next="+bad)
			if got := w.Header().Get("Location"); got != "/admin" {
				t.Errorf("next=%q 跳到了 %q，应当退回 /admin", bad, got)
			}
		})
	}
}

func langGet(t *testing.T, r http.Handler, path, cookie, lang string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Cookie", cookie+"; "+web.LangCookie+"="+lang)
	req.Header.Set("Accept", "text/html")
	// 故意和 cookie 唱反调：cookie 该压过它。
	req.Header.Set("Accept-Language", map[string]string{"en": "zh-CN,zh;q=0.9"}[lang])
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func plainGet(t *testing.T, r http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	return w
}

// visibleText 去掉样式、脚本和注释——那三处的中文是写给读源码的人的，
// 不是界面文案。剩下的才是访客真正读到的字。
func visibleText(html string) string {
	for _, pair := range [][2]string{
		{"<style>", "</style>"},
		{"<script>", "</script>"},
		{"<!--", "-->"},
		{"/*", "*/"}, // <style> 之外还有内联 style 属性里的注释
		// 语言开关显示的是【别的语言自己的名字】，所以英文界面上那个链接
		// 写着「中文」是对的，不是漏翻。
		{`<nav class="langsw`, "</nav>"},
	} {
		for {
			i := strings.Index(html, pair[0])
			if i < 0 {
				break
			}
			j := strings.Index(html[i:], pair[1])
			if j < 0 {
				html = html[:i]
				break
			}
			html = html[:i] + html[i+j+len(pair[1]):]
		}
	}
	return html
}

// firstHan 返回第一个汉字所在的那一小段，空串表示一个都没有。
// 返回上下文而不只是那个字——光报一个「设」，没人找得到它在哪。
func firstHan(s string) string {
	for i, r := range s {
		if unicode.Is(unicode.Han, r) {
			lo := max(0, i-60)
			hi := min(len(s), i+60)
			return s[lo:hi]
		}
	}
	return ""
}

// 图标要真的发得出来，而且每一页都引用了它。
// 少了这一条，「标签页是空白的」只会在有人截图时才被发现。
func TestIconsAreServedAndLinked(t *testing.T) {
	_, r := newServerWith(t, false, 0, nil)

	for _, p := range []string{"/favicon.ico", "/favicon.png", "/apple-touch-icon.png"} {
		w := plainGet(t, r, p)
		if w.Code != http.StatusOK {
			t.Errorf("%s 回了 %d", p, w.Code)
		}
		if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "image/") {
			t.Errorf("%s 的 Content-Type 是 %q", p, ct)
		}
		if w.Body.Len() == 0 {
			t.Errorf("%s 是空的", p)
		}
	}

	// 陌生人会走到的那几页都该带上图标链接
	for _, p := range []string{"/docs", "/login"} {
		if !strings.Contains(plainGet(t, r, p).Body.String(), `rel="icon"`) {
			t.Errorf("%s 的 <head> 里没有图标链接", p)
		}
	}
}
