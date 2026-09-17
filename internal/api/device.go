package api

import (
	"github.com/aichy126/igo/res"
	"github.com/aichy126/knockbox/internal/middleware"
	"github.com/aichy126/knockbox/internal/service"
	"github.com/gin-gonic/gin"
)

// pair 用一次性配对码换设备身份。这个接口【不需要】设备鉴权——它就是拿身份的地方。
func (s *Server) pair(c *gin.Context) {
	var in service.RegisterInput
	if err := c.ShouldBindJSON(&in); err != nil {
		res.Rfail(c, "解析请求失败: "+err.Error())
		return
	}
	out, err := service.NewDevice(s.DAO).Pair(in, s.Name, s.Version)
	if err != nil {
		s.fail(c, err)
		return
	}
	res.Rsucc(c, out)
}

type pushTokenBody struct {
	APNsToken  string `json:"apns_token"`
	APNsEnv    string `json:"apns_env"`
	AppVersion string `json:"app_version"`
	OSVersion  string `json:"os_version"`
}

// pushToken 客户端每次拿到 APNs token 都无条件调它，服务端比对不同才写库。
func (s *Server) pushToken(c *gin.Context) {
	var b pushTokenBody
	if err := c.ShouldBindJSON(&b); err != nil {
		res.Rfail(c, "解析请求失败: "+err.Error())
		return
	}
	dev := middleware.Device(c)
	if err := service.NewDevice(s.DAO).UpdatePushToken(dev, b.APNsToken, b.APNsEnv, b.AppVersion, b.OSVersion); err != nil {
		s.fail(c, err)
		return
	}
	res.Rsucc(c, gin.H{"ok": true})
}

func (s *Server) listDevices(c *gin.Context) {
	out, err := service.NewDevice(s.DAO).ListByUser(middleware.UserID(c))
	if err != nil {
		s.fail(c, err)
		return
	}
	type row struct {
		UUID, Name, Platform, Model, OSVersion, AppVersion, APNsEnv, AuthPrefix string
		Status                                                                  int
		SyncRev, LastSeenAt, LastPushAt                                         int64
		CanPush                                                                 bool
	}
	rows := make([]row, 0, len(out))
	for _, d := range out {
		rows = append(rows, row{
			UUID: d.UUID, Name: d.Name, Platform: d.Platform, Model: d.Model,
			OSVersion: d.OSVersion, AppVersion: d.AppVersion, APNsEnv: d.APNsEnv,
			AuthPrefix: d.AuthPrefix, Status: d.Status, SyncRev: d.SyncRev,
			LastSeenAt: d.LastSeenAt, LastPushAt: d.LastPushAt,
			CanPush: d.APNsToken != "",
		})
	}
	res.Rsucc(c, gin.H{"list": rows, "count": len(rows)})
}

// deleteDevice uuid 传 self 表示登出自己。
func (s *Server) deleteDevice(c *gin.Context) {
	uuid := c.Param("uuid")
	if uuid == "self" {
		uuid = middleware.Device(c).UUID
	}
	if err := service.NewDevice(s.DAO).Logout(middleware.UserID(c), uuid); err != nil {
		s.fail(c, err)
		return
	}
	res.Rsucc(c, gin.H{"ok": true})
}

// issuePair 已配对的设备给新设备出码。
//
// **这是多设备的唯一入口**：公共实例上没有账号密码，没有这条路的话，
// 一个人就只能有一台设备——而「手机收到了想在 iPad 上也看」是最自然的诉求。
// 码属于【当前设备所属的那个用户】，所以新设备扫完就是同一个身份，
// 消息两边都到、已读互通。
func (s *Server) issuePair(c *gin.Context) {
	code, err := service.NewPair(s.DAO).Issue(
		middleware.UserID(c), s.ExternalURL, "device", s.PairTTL)
	if err != nil {
		s.fail(c, err)
		return
	}
	res.Rsucc(c, gin.H{
		"code": code.Code,
		"host": s.ExternalURL,
		// unix 秒。**接口里所有时间都是秒级整数**——
		// 直接扔 time.Time 会被序列化成 RFC3339 字符串，
		// 与其它字段不一致，客户端按整数解就崩。
		"expires": code.ExpiresAt.Unix(),
		// 二维码内容由服务端给：拼错了扫出来是个不能用的链接，
		// 而那种错误要等到有人拿另一台设备扫的时候才暴露。
		"link": code.DeepLink,
	})
}
