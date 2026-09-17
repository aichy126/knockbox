package api

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

// 表单里的 reply 要能用。
//
// 带图的消息只能用 multipart 发，而 multipart 里表达不出嵌套对象，
// 所以调用方多半会把 reply 写成一段 JSON 塞进表单字段。
// 不接的话，「带图 + 要回复」这个组合永远发不出来 —— 而且不报错，
// 只是那条消息静默变成单向的。
func TestMultipartAcceptsReplyJSON(t *testing.T) {
	s, r, ch := newSendServer(t, 0, 0)

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("title", "带图的问题")
	_ = w.WriteField("text", "看得清吗")
	_ = w.WriteField("reply",
		`{"type":"choice","options":["清楚","看不清"],"webhook":"https://x.test/hook"}`)
	_ = w.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/send/"+ch.Token, &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("发送失败：%d %s", rec.Code, rec.Body.String())
	}
	var hook string
	if _, err := s.DAO.Engine().SQL(
		"SELECT reply_webhook FROM message ORDER BY id DESC LIMIT 1").Get(&hook); err != nil {
		t.Fatal(err)
	}
	if hook != "https://x.test/hook" {
		t.Errorf("表单里的 reply 没生效，reply_webhook = %q", hook)
	}
}

// 写坏的 reply 要报错，不能当成「没写」放行。
// 放行的话那条消息静默变成单向的，而发送方还在等一个永远不来的答案。
func TestMalformedReplyIsRejected(t *testing.T) {
	_, r, ch := newSendServer(t, 0, 0)
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/send/"+ch.Token+"?title=x&reply={坏掉的json", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if !bytes.Contains(rec.Body.Bytes(), []byte("reply")) {
		t.Errorf("写坏的 reply 应当明确报错，得到：%s", rec.Body.String())
	}
}
