package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	_ "github.com/aichy126/igo/db"
	"github.com/aichy126/knockbox/internal/library/urlsign"
	"github.com/gin-gonic/gin"
)

// 附件原来是整个读进内存再吐。改成流式之后，两件事都要守住：
// 内容一个字节不能差，Range 请求要真的生效。
func TestServeFileStreamsWithRangeSupport(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, r := newServerWith(t, false, 100, nil)

	png := testPNG(60, 40)
	f, err := s.Files.Store(1, "chart.png", png)
	if err != nil {
		t.Fatal(err)
	}
	url := "/api/v1/file/" + f.UID + "?" +
		urlsign.Sign(s.Files.SignKey(), f.UID, false, time.Hour)

	// 完整下载
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, url, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("完整下载返回 %d", w.Code)
	}
	body := w.Body.Bytes()
	if len(body) != int(f.Size) {
		t.Fatalf("长度对不上：收到 %d，file.size 是 %d", len(body), f.Size)
	}
	if ct := w.Header().Get("Content-Type"); ct != f.Mime {
		t.Errorf("Content-Type 应当用存库时量到的 %q，得到 %q", f.Mime, ct)
	}
	if cc := w.Header().Get("Cache-Control"); cc == "" {
		t.Error("内容寻址的附件应当带长缓存头")
	}
	if w.Header().Get("Accept-Ranges") != "bytes" {
		t.Error("流式返回应当声明支持 Range")
	}

	// 断点续传：弱网下通知扩展取图靠它
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Range", "bytes=0-9")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusPartialContent {
		t.Fatalf("Range 请求应当返回 206，得到 %d", w.Code)
	}
	if got := w.Body.Len(); got != 10 {
		t.Fatalf("应当只回 10 字节，得到 %d", got)
	}
	for i := 0; i < 10; i++ {
		if w.Body.Bytes()[i] != body[i] {
			t.Fatalf("第 %d 个字节对不上", i)
		}
	}
}

// 签名不对一律 403，流式改造不能把这层绕过去。
func TestServeFileStillRequiresSignature(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, r := newServerWith(t, false, 100, nil)
	f, err := s.Files.Store(1, "a.png", testPNG(10, 10))
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct{ name, url string }{
		{"没有签名", "/api/v1/file/" + f.UID},
		{"签名是别人的", "/api/v1/file/" + f.UID + "?" +
			urlsign.Sign(s.Files.SignKey(), "someone-else", false, time.Hour)},
		{"链接已过期", "/api/v1/file/" + f.UID + "?" +
			urlsign.Sign(s.Files.SignKey(), f.UID, false, -time.Minute)},
	} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.url, nil))
		if w.Code != http.StatusForbidden {
			t.Errorf("%s：应当 403，得到 %d", tc.name, w.Code)
		}
	}
}

// blob 被回收之后记录可能还在，这时要 404 而不是 500。
func TestServeFileMissingBlobReturns404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, r := newServerWith(t, false, 100, nil)
	f, err := s.Files.Store(1, "a.png", testPNG(10, 10))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DAO.Engine().Exec(
		"UPDATE file SET path = 'zz/not-there.jpg' WHERE id = ?", f.Id); err != nil {
		t.Fatal(err)
	}
	url := "/api/v1/file/" + f.UID + "?" +
		urlsign.Sign(s.Files.SignKey(), f.UID, false, time.Hour)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, url, nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("应当 404，得到 %d", w.Code)
	}
}
