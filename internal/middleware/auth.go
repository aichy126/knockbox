// Package middleware HTTP 中间件。
//
// 三套鉴权的边界是刻意分开的，别合并：
//
//	SendAuth   频道 token —— 只写，只能往绑定的那一个频道发，读不到任何东西。
//	           泄露了最多被人刷消息，拿不到历史、拿不到别的频道。
//	DeviceAuth 设备 token —— 读本 user 的全部消息、管自己的频道与设备。
//	AdminAuth  管理员会话 —— 管理界面的全部能力，包括浏览所有人的消息。
//	           这是有意的全权限：它就是这台服务器的主人。
package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/url"
	"strings"

	"github.com/aichy126/knockbox/internal/dao"
	"github.com/aichy126/knockbox/internal/models"
	"github.com/gin-gonic/gin"
)

// gin.Context 里的 key。
const (
	CtxChannel = "kb.channel"
	CtxDevice  = "kb.device"
	CtxUserID  = "kb.user_id"
	CtxAdmin   = "kb.admin"
)

// Hash 用 sha256 而不是 bcrypt：token 是 256 位高熵随机串，不需要抗字典，
// 而每个请求跑一次 bcrypt 太贵（登录密码另算，那里用 bcrypt）。
func Hash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// SendToken 从多个位置取发送凭据，让 curl 无论怎么写都能发出去。
//
// 顺序：URL 路径参数最优先，然后是 Authorization: Bearer、token 头、
// ?token= 查询参数、表单字段。
//
// 路径优先的依据是「越具体越优先」：/send/<token> 的 URL 里已经写明了要发给
// 哪个频道，而 Authorization 是整套 API 通用的凭据位。若让头优先，已持有设备
// token 的客户端在调用发送接口时会带上设备 token，服务端拿它去查频道必然落空，
// 得到一个与实际原因无关的 401。
func SendToken(c *gin.Context) string {
	if v := c.Param("token"); v != "" {
		return strings.TrimSpace(v)
	}
	if v := c.GetHeader("Authorization"); strings.HasPrefix(v, "Bearer ") {
		return strings.TrimSpace(v[7:])
	}
	if v := c.GetHeader("token"); v != "" {
		return strings.TrimSpace(v)
	}
	if v := c.Query("token"); v != "" {
		return strings.TrimSpace(v)
	}
	return strings.TrimSpace(c.PostForm("token"))
}

func fail(c *gin.Context, status int, msg string) {
	c.AbortWithStatusJSON(status, gin.H{"code": 1, "msg": msg})
}

// SendAuth 校验频道 token。只放行「往这个频道发消息」。
func SendAuth(d *dao.DAO) gin.HandlerFunc {
	return func(c *gin.Context) {
		tok := SendToken(c)
		if tok == "" {
			fail(c, http.StatusUnauthorized, "缺少频道 token")
			return
		}
		var ch models.Channel
		has, err := d.Engine().Where("token = ?", tok).Get(&ch)
		if err != nil {
			fail(c, http.StatusInternalServerError, "查询频道失败: "+err.Error())
			return
		}
		// 不存在和已停用给同一句话：不让调用方用错误信息探测 token 是否存在。
		if !has || ch.Status != models.StatusActive {
			fail(c, http.StatusUnauthorized, "频道 token 无效")
			return
		}
		c.Set(CtxChannel, &ch)
		c.Set(CtxUserID, ch.UserId)
		c.Next()
	}
}

// DeviceAuth 校验设备 token。
func DeviceAuth(d *dao.DAO) gin.HandlerFunc {
	return func(c *gin.Context) {
		v := c.GetHeader("Authorization")
		if !strings.HasPrefix(v, "Bearer ") {
			fail(c, http.StatusUnauthorized, "缺少设备 token")
			return
		}
		var dev models.Device
		has, err := d.Engine().Where("auth_hash = ?", Hash(strings.TrimSpace(v[7:]))).Get(&dev)
		if err != nil {
			fail(c, http.StatusInternalServerError, "查询设备失败: "+err.Error())
			return
		}
		if !has {
			fail(c, http.StatusUnauthorized, "设备 token 无效")
			return
		}
		// 主动登出的设备要能明确知道自己被登出了，好回到配对界面；
		// 而 APNs 失效（410）只是推不动，读历史照常，不该把人挡在外面。
		if dev.Status == models.DeviceLoggedOut {
			fail(c, http.StatusUnauthorized, "设备已登出，请重新配对")
			return
		}
		c.Set(CtxDevice, &dev)
		c.Set(CtxUserID, dev.UserId)
		c.Next()
	}
}

// Channel 取 SendAuth 放进去的频道。
func Channel(c *gin.Context) *models.Channel {
	v, _ := c.Get(CtxChannel)
	ch, _ := v.(*models.Channel)
	return ch
}

// Device 取 DeviceAuth 放进去的设备。
func Device(c *gin.Context) *models.Device {
	v, _ := c.Get(CtxDevice)
	dev, _ := v.(*models.Device)
	return dev
}

// UserID 取当前请求归属的用户。
func UserID(c *gin.Context) int64 {
	v, _ := c.Get(CtxUserID)
	id, _ := v.(int64)
	return id
}

// SessionCookie 管理会话的 cookie 名。
const SessionCookie = "kb_session"

// AdminAuth 校验管理员会话。
//
// 这是有意的全权限：管理员就是这台服务器的主人，能看到所有人的消息。
// app 端主动扫码授权接入这个服务端，就已经接受了「服务端持有消息」这个前提。
func AdminAuth(verify func(string) (*models.User, error)) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw, err := c.Cookie(SessionCookie)
		if err != nil || raw == "" {
			unauthorized(c, "未登录")
			return
		}
		u, err := verify(raw)
		if err != nil {
			unauthorized(c, "会话无效或已过期，请重新登录")
			return
		}
		c.Set(CtxUserID, u.Id)
		c.Set(CtxAdmin, u)
		c.Next()
	}
}

// unauthorized 浏览器跳登录页，接口回 JSON。
//
// 统一回 JSON 的话，用户在地址栏敲一个后台地址会得到一屏 {"code":1,"msg":"未登录"}——
// 那不是错误处理，那是把内部表示扔给用户看。按 Accept 头分流。
func unauthorized(c *gin.Context, msg string) {
	if strings.Contains(c.GetHeader("Accept"), "text/html") {
		// 带上原地址，登录完能回到他本来要去的页面
		c.Redirect(http.StatusFound, "/login?next="+url.QueryEscape(c.Request.URL.RequestURI()))
		c.Abort()
		return
	}
	fail(c, http.StatusUnauthorized, msg)
}

// Admin 取当前登录的管理员。
func Admin(c *gin.Context) *models.User {
	v, _ := c.Get(CtxAdmin)
	u, _ := v.(*models.User)
	return u
}
