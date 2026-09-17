package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	_ "github.com/aichy126/igo/db"
	"github.com/gin-gonic/gin"
)

// 对外页面按浏览器语言选中英文，**默认英文**。
//
// 这几条断言看着像在测 web.PickLang，其实测的是「语言有没有一路传到页面上」——
// 中间少接一个字段不会编译失败，只会悄悄渲染出另一种语言。
func TestPublicPagesPickLanguage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	_, r := newJoinServer(t, true, 100)

	cases := []struct {
		name, path, accept, want, notWant string
	}{
		{"默认英文", "/", "", "Scan the code below", "扫下面的二维码"},
		{"没有 Accept-Language 也是英文", "/", "", `<html lang="en"`, `<html lang="zh-CN"`},
		{"中文浏览器给中文", "/", "zh-CN,zh;q=0.9,en;q=0.8", "扫下面的二维码", "Scan the code below"},
		{"英文浏览器给英文", "/", "en-US,en;q=0.9", "Scan the code below", "扫下面的二维码"},
		{"查询参数压过浏览器", "/?lang=zh", "en-US,en;q=0.9", "扫下面的二维码", "Scan the code below"},
		{"说明页同样跟随", "/docs", "zh-CN,zh;q=0.9", "怎么发", "Sending"},
		{"说明页默认英文", "/docs", "", "Sending", "怎么发"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, c.path, nil)
			if c.accept != "" {
				req.Header.Set("Accept-Language", c.accept)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("返回 %d", w.Code)
			}
			body := w.Body.String()
			if !strings.Contains(body, c.want) {
				t.Errorf("页面里没有 %q", c.want)
			}
			if strings.Contains(body, c.notWant) {
				t.Errorf("页面里不该出现 %q", c.notWant)
			}
		})
	}
}

// 一个拼错的 ?lang= 不该把页面打没，回落英文即可。
func TestUnknownLangFallsBackToEnglish(t *testing.T) {
	gin.SetMode(gin.TestMode)
	_, r := newJoinServer(t, true, 100)
	w := get(r, "/?lang=klingon", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("返回 %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Scan the code below") {
		t.Error("未知语言应当回落英文")
	}
}
