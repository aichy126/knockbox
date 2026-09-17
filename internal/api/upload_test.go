package api

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	_ "github.com/aichy126/igo/db"
	"github.com/aichy126/knockbox/internal/dao"
	"github.com/aichy126/knockbox/internal/middleware"
	"github.com/aichy126/knockbox/internal/migrate"
	"github.com/aichy126/knockbox/internal/service"
	"github.com/gin-gonic/gin"
	"xorm.io/xorm"
)

// newUploadServer 起一个只挂了上传接口的服务端，鉴权换成固定用户，
// 这里要验的是大小上限，不是鉴权。maxMB 是 storage.max_upload_mb。
func newUploadServer(t *testing.T, maxMB int) (*Server, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	e, err := xorm.NewEngine("sqlite3", filepath.Join(dir, "t.db")+"?"+migrate.DSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.Close() })
	if err := migrate.Run(e); err != nil {
		t.Fatal(err)
	}
	d := dao.New(e)
	s := &Server{
		DAO:   d,
		Files: service.NewFile(d, filepath.Join(dir, "blobs"), maxMB, 1600, 600, 70, "http://x", 3600),
	}
	r := gin.New()
	r.POST("/upload", func(c *gin.Context) {
		c.Set(middleware.CtxUserID, int64(1))
		s.upload(c)
	})
	return s, r
}

func multipartBody(t *testing.T, field, name string, payload []byte) (string, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile(field, name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return w.FormDataContentType(), &buf
}

// 上传接口原来是 io.ReadAll(src)，完全没有上限：大小判断在 Store 里，
// 而那是把整个文件读进内存【之后】才做的。传一个超大的文件就是等量的常驻内存。
func TestUploadRejectsOversizeAttachment(t *testing.T) {
	s, r := newUploadServer(t, 1) // 上限 1 MB

	ct, body := multipartBody(t, "file", "big.bin", bytes.Repeat([]byte("x"), 2<<20))
	req := httptest.NewRequest(http.MethodPost, "/upload", body)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("响应不是 JSON: %s", w.Body.String())
	}
	if code, _ := out["code"].(float64); code == 0 {
		t.Fatalf("超过上限的附件被接受了: %s", w.Body.String())
	}

	// 更要紧的是别留下半截数据
	var n int64
	_, _ = s.DAO.Engine().SQL("SELECT COUNT(*) FROM file").Get(&n)
	if n != 0 {
		t.Errorf("被拒绝的上传仍然在 file 表里留了 %d 行", n)
	}
}

// 限制是对的，但不能把正常大小的附件也挡掉。
func TestUploadAcceptsAttachmentWithinLimit(t *testing.T) {
	s, r := newUploadServer(t, 4)

	ct, body := multipartBody(t, "file", "small.png", testPNG(40, 30))
	req := httptest.NewRequest(http.MethodPost, "/upload", body)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var out struct {
		Code int            `json:"code"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("响应不是 JSON: %s", w.Body.String())
	}
	if out.Code != 0 {
		t.Fatalf("正常大小的附件被拒了: %s", w.Body.String())
	}
	if out.Data["uid"] == nil {
		t.Fatalf("返回里没有附件 uid: %s", w.Body.String())
	}
	var n int64
	_, _ = s.DAO.Engine().SQL("SELECT COUNT(*) FROM file").Get(&n)
	if n != 1 {
		t.Errorf("file 表里应当有 1 行，实际 %d 行", n)
	}
}
