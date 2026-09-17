package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/aichy126/igo/res"
	"github.com/aichy126/knockbox/internal/library/bytesize"
	"github.com/aichy126/knockbox/internal/library/urlsign"
	"github.com/aichy126/knockbox/internal/middleware"
	"github.com/aichy126/knockbox/internal/models"
	"github.com/aichy126/knockbox/internal/service"
	"github.com/gin-gonic/gin"
)

func (s *Server) fileView(f *models.File) gin.H {
	return gin.H{
		"uid": f.UID, "name": f.Name, "mime": f.Mime, "size": f.Size,
		"width": f.Width, "height": f.Height,
		"url":   s.Files.URL(f.UID, false),
		"thumb": s.Files.URL(f.UID, true),
	}
}

// upload 先传后发：批量投递时 N 个频道共用同一个 uid，内容只存一份。
func (s *Server) upload(c *gin.Context) {
	max := s.Files.MaxBytes()
	// 上限必须在解析 multipart 【之前】设，否则超大的请求体已经落进临时文件了。
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, max+multipartSlack)
	fh, err := c.FormFile("file")
	if err != nil {
		res.Rfail(c, "no file field in the request")
		return
	}
	if fh.Size > max {
		res.Rfail(c, fmt.Sprintf("attachment exceeds the size limit (%s)", bytesize.Human(max)))
		return
	}
	src, err := fh.Open()
	if err != nil {
		res.Rfail(c, err.Error())
		return
	}
	defer func() { _ = src.Close() }()
	// 即便声明的 Size 达标也照样封顶：Size 来自请求方，不可信。
	data, err := io.ReadAll(io.LimitReader(src, max+1))
	if err != nil {
		res.Rfail(c, err.Error())
		return
	}
	if int64(len(data)) > max {
		res.Rfail(c, fmt.Sprintf("attachment exceeds the size limit (%s)", bytesize.Human(max)))
		return
	}
	f, err := s.Files.Store(middleware.UserID(c), fh.Filename, data)
	if err != nil {
		res.Rfail(c, err.Error())
		return
	}
	res.Rsucc(c, s.fileView(f))
}

// serveFile 走签名校验，不需要登录态——通知服务扩展就是靠这条路下图的。
func (s *Server) serveFile(c *gin.Context) {
	uid := c.Param("uid")
	if err := urlsign.Verify(s.Files.SignKey(), uid, c.Request.URL.Query()); err != nil {
		c.String(http.StatusForbidden, err.Error())
		return
	}
	f, err := s.Files.ByUID(uid)
	if err != nil {
		c.String(http.StatusNotFound, err.Error())
		return
	}
	fh, mime, err := s.Files.Open(f, c.Query("t") == "1")
	if err != nil {
		c.String(http.StatusNotFound, "附件内容已不在")
		return
	}
	defer func() { _ = fh.Close() }()
	info, err := fh.Stat()
	if err != nil {
		c.String(http.StatusNotFound, "附件内容已不在")
		return
	}
	// 内容寻址 = 内容永不变，可以放心长缓存
	c.Header("Cache-Control", "private, max-age=604800, immutable")
	// Content-Type 先设好，ServeContent 就不会再去按扩展名猜——
	// 主文件可能根本没有扩展名，而 file.mime 是存的时候量过的。
	c.Header("Content-Type", mime)
	// 流式返回而不是整个读进内存：附件最大 20 MB，并发下载时每一路都要占一份。
	// 顺带得到 Range 支持，弱网下通知扩展取图可以续传。
	http.ServeContent(c.Writer, c.Request, f.Name, info.ModTime(), fh)
}

// withFileURLs 往消息的 extra 里补上附件的签名链接。
//
// **必须在响应这一刻生成，不能存库**：链接里带过期时间和 HMAC 签名，
// 存下来的那份很快就只剩 403。客户端也不该自己拼——拼错了同样是 403。
func (s *Server) withFileURLs(raw string) string {
	if raw == "" || s.Files == nil {
		return raw
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return raw
	}
	uid, _ := m["file"].(string)
	if uid == "" {
		return raw
	}
	m["file_url"] = s.Files.URL(uid, false)
	m["thumb_url"] = s.Files.URL(uid, true)
	// 尺寸也要给。
	//
	// 少了它，客户端在图下载完之前只能先占一个固定高度的灰框，图到了再跳成
	// 真实比例——整个消息流跟着重排一次，进频道时肉眼可见地抖一下。
	// 有了宽高，占位框从一开始就是对的，图换进去时一个像素都不动。
	if f, err := s.Files.ByUID(uid); err == nil && f.Width > 0 && f.Height > 0 {
		m["file_w"], m["file_h"] = f.Width, f.Height
	}
	b, err := json.Marshal(m)
	if err != nil {
		return raw
	}
	return string(b)
}

// fillFileURLs 批量处理一组消息视图。
func (s *Server) fillFileURLs(list []service.MessageView) {
	for i := range list {
		list[i].Extra = s.withFileURLs(list[i].Extra)
	}
}
