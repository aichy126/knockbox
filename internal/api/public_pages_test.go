package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode"

	"github.com/aichy126/knockbox/internal/service"
)

// 陌生人会走到的那几页：接入页、发送说明页、错误页。它们仍然是服务端直出的，
// 不需要 JavaScript——换成 SPA 的只有管理界面。

// 英文界面里不该出现一个汉字。
//
// 文案是从语料取的，漏一句在中文下完全看不出来——它本来就是中文。只有拿英文
// 渲染一遍再扫汉字，漏掉的那句才会自己跳出来。管理界面那半在 admin/test/。
func TestPublicPagesHaveNoChineseInEnglish(t *testing.T) {
	s, r := newServerWith(t, true, 10, nil)
	// 发送说明页要一个真频道才渲染得出来；/s/nope 走的是错误页那一支。
	u, err := service.NewAdmin(s.DAO).CreateMember("someone")
	if err != nil {
		t.Fatal(err)
	}
	c, err := service.NewChannel(s.DAO).Create(u.Id, service.ChannelInput{})
	if err != nil {
		t.Fatal(err)
	}
	ch := c.Token

	for _, p := range []string{"/", "/join", "/docs", "/s/" + ch, "/s/nope"} {
		t.Run(p, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, p, nil)
			req.Header.Set("Accept", "text/html")
			req.Header.Set("Accept-Language", "en-US,en;q=0.9")
			r.ServeHTTP(w, req)
			if han := firstHan(visibleText(w.Body.String())); han != "" {
				t.Errorf("英文页面里出现了汉字：%q\n（有一句文案没搬进 locales）", han)
			}
		})
	}
}

// 反过来：Accept-Language 说中文，页面就得真的是中文。
// 上面那条断言的是「没漏」，这条断言的是「真的在按语言切」——
// 只有前者的话，把所有文案都写成英文硬编码也能过。
func TestPublicPagesFollowAcceptLanguage(t *testing.T) {
	_, r := newServerWith(t, true, 10, nil)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/docs", nil)
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	r.ServeHTTP(w, req)
	if !strings.Contains(w.Body.String(), "怎么用") {
		t.Error("中文下的发送说明页不是中文")
	}
}

// 图标要真的发得出来，而且公开页引用了它。
// 少了这一条，「标签页是空白的」只会在有人截图时才被发现。
func TestIconsAreServedAndLinked(t *testing.T) {
	_, r := newServerWith(t, true, 10, nil)
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
	if !strings.Contains(plainGet(t, r, "/docs").Body.String(), `rel="icon"`) {
		t.Error("/docs 的 <head> 里没有图标链接")
	}
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
		{"/*", "*/"},
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
			return s[max(0, i-60):min(len(s), i+60)]
		}
	}
	return ""
}
