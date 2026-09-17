package api

import (
	"strconv"
	"strings"

	"github.com/aichy126/igo/log"
	"github.com/aichy126/igo/res"
	"github.com/aichy126/knockbox/internal/api/web"
	"github.com/aichy126/knockbox/internal/middleware"
	"github.com/aichy126/knockbox/internal/service"
	"github.com/aichy126/knockbox/internal/uierr"
	"github.com/gin-gonic/gin"
)

// 后台会改数据的那批接口。与只读的分开放，让「哪些 handler 有副作用」
// 在文件树上一眼可见。

// badRequest 请求体本身读不了。这不是「用户的下一步不同」那类失败，
// 是调用方写错了，所以不进 uierr——读它的是写这个前端的人。
func badRequest(c *gin.Context, err error) {
	res.Rfail(c, "cannot parse the request: "+err.Error())
}

// ── 登出 ──────────────────────────────────────────────

// adminAPILogout 用 POST 不用 GET。
//
// 现存的 GET /logout 一个 <img src="/logout"> 就能把人踢下线——SameSite=Lax
// 不拦顶层 GET 导航，也不拦这个。那条路留给服务端直出的页面，第二步一起删。
func (s *Server) adminAPILogout(c *gin.Context) {
	if raw, err := c.Cookie(middleware.SessionCookie); err == nil && raw != "" {
		if err := service.NewSession(s.DAO).Logout(raw); err != nil {
			log.Error("admin api: logout", log.Any("error", err.Error()))
		}
	}
	s.setSessionCookie(c, "", -1)
	res.Rsucc(c, gin.H{"ok": true})
}

// ── 成员 ──────────────────────────────────────────────

func (s *Server) adminAPIUnlimited(c *gin.Context) {
	var in struct {
		On bool `json:"on"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		badRequest(c, err)
		return
	}
	id := pathID(c)
	if err := s.admin().SetUnlimited(id, in.On); err != nil {
		s.adminFail(c, "set unlimited", err)
		return
	}
	// 回写后的真实状态，前端不用猜自己那一次点击有没有落地。
	res.Rsucc(c, gin.H{"id": id, "unlimited": in.On})
}

// ── 设备 ──────────────────────────────────────────────

// adminAPIRevokeDevice 注销一台设备。
//
// 路径用 revoke 不用 delete：它做的是撤销凭据（软登出），不是删行。
func (s *Server) adminAPIRevokeDevice(c *gin.Context) {
	id := pathID(c)
	owner, err := s.admin().RevokeDevice(id)
	if err != nil {
		s.adminFail(c, "revoke device", err)
		return
	}
	res.Rsucc(c, gin.H{"id": id, "owner": owner, "status": 2})
}

// ── 频道 ──────────────────────────────────────────────

func (s *Server) adminAPIPurge(c *gin.Context) {
	var in struct {
		// BeforeMsgID 0 = 服务端取当前 MAX(id) 做快照。
		// 暴露出来是因为 PurgeChannel 本就支持它，而它的存在是有理由的：
		// 删的是「id <= 快照值」，清空过程中到达的新消息不会被静默吃掉。
		BeforeMsgID int64 `json:"before_msg_id"`
	}
	// 空 body 也算合法：前端正常情况下什么都不传。
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&in); err != nil {
			badRequest(c, err)
			return
		}
	}
	id := c.Param("id")
	ch, _, err := s.admin().Channel(id)
	if err != nil {
		s.adminFail(c, "channel", err)
		return
	}
	if in.BeforeMsgID <= 0 {
		if in.BeforeMsgID, err = s.admin().ChannelMaxMsgId(id); err != nil {
			s.adminFail(c, "channel max id", err)
			return
		}
	}
	out, err := service.NewSync(s.DAO).PurgeChannel(ch.UserId, id, in.BeforeMsgID)
	if err != nil {
		s.adminFail(c, "purge channel", err)
		return
	}
	res.Rsucc(c, out)
}

// ── 配对 ──────────────────────────────────────────────

type pairIssued struct {
	Code     string `json:"code"`
	DeepLink string `json:"deep_link"`
	// QRSVG 服务端生成的二维码，原样是一段 SVG。
	//
	// #15 把它单列为「无论如何留在服务端」的三样之一，理由是页面因此不需要
	// 任何外部脚本。把 SVG 当数据下发，那条性质完整保住。
	QRSVG     string `json:"qr_svg"`
	ExpiresAt int64  `json:"expires_at"`
	Member    struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
		// Created 这次是不是顺手建的。界面上要说得出「新建了一个成员」。
		Created bool `json:"created"`
	} `json:"member"`
}

func (s *Server) adminAPIPairIssue(c *gin.Context) {
	var in struct {
		MemberID int64  `json:"member_id"`
		NewName  string `json:"new_name"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		badRequest(c, err)
		return
	}
	var (
		uid     int64
		name    string
		created bool
	)
	switch {
	case in.MemberID != 0:
		u, err := s.admin().Member(in.MemberID)
		if err != nil {
			s.adminFail(c, "member", err)
			return
		}
		uid, name = u.Id, u.Name
	case strings.TrimSpace(in.NewName) != "":
		u, err := s.admin().CreateMember(strings.TrimSpace(in.NewName))
		if err != nil {
			s.adminFail(c, "create member", err)
			return
		}
		uid, name, created = u.Id, u.Name, true
	default:
		s.fail(c, uierr.New(uierr.PairNoTarget))
		return
	}

	code, err := service.NewPair(s.DAO).Issue(uid, s.ExternalURL, "admin", s.PairTTL)
	if err != nil {
		s.adminFail(c, "issue pair code", err)
		return
	}
	qr, err := web.QRSVG(code.DeepLink, 240)
	if err != nil {
		// 码本身是好的，只是画不出来。别让一张图把整个签发废掉——
		// Display 那串字手输同样能配对。
		log.Error("admin api: qr render", log.Any("error", err.Error()))
	}
	out := pairIssued{
		Code: code.Display, DeepLink: code.DeepLink, QRSVG: qr,
		ExpiresAt: code.ExpiresAt.Unix(),
	}
	out.Member.ID, out.Member.Name, out.Member.Created = uid, name, created
	res.Rsucc(c, out)
}

// ── 设置 ──────────────────────────────────────────────

// settingsPatch 每个字段都是指针。
//
// Settings.Save 的语义是「没提到的键保持不变」，而表单那一版只能用「空串当没改」
// 去近似它——于是想把一个数字清零和没填这个框长得一样。JSON 能区分「缺省」
// 和「显式给了值」，所以那个近似可以去掉，原来的意图反而完整了。
type settingsPatch struct {
	PublicEnabled   *bool   `json:"public_enabled"`
	RegisterPerHour *int    `json:"register_per_hour"`
	MaxChannels     *int    `json:"max_channels"`
	MaxPerDay       *int    `json:"max_per_day"`
	RetentionDays   *int    `json:"retention_days"`
	SiteName        *string `json:"site_name"`
}

// siteNameMax 站点名的上限。它会出现在接入页的标题上，
// 没有上限的话一个超长名字会直接把那一页撑坏。
const siteNameMax = 64

func (s *Server) adminAPISettingsSave(c *gin.Context) {
	var in settingsPatch
	if err := c.ShouldBindJSON(&in); err != nil {
		badRequest(c, err)
		return
	}
	vals := map[string]string{}
	// 四个数字字段不接受负数。存成负数不会报错，只会让配额判断在别处
	// 表现成谁也想不到的样子。
	for _, f := range []struct {
		key string
		p   *int
	}{
		{"register_per_hour", in.RegisterPerHour},
		{"max_channels", in.MaxChannels},
		{"max_per_day", in.MaxPerDay},
		{"retention_days", in.RetentionDays},
	} {
		if f.p == nil {
			continue
		}
		if *f.p < 0 {
			s.fail(c, uierr.New(uierr.SettingBadValue, f.key))
			return
		}
		vals[f.key] = strconv.Itoa(*f.p)
	}
	if in.SiteName != nil {
		name := strings.TrimSpace(*in.SiteName)
		if len([]rune(name)) > siteNameMax {
			s.fail(c, uierr.New(uierr.SettingBadValue, "site_name"))
			return
		}
		vals["site_name"] = name
	}
	if in.PublicEnabled != nil {
		vals["public_enabled"] = map[bool]string{true: "1", false: "0"}[*in.PublicEnabled]
	}
	if len(vals) > 0 {
		if err := s.Settings.Save(vals); err != nil {
			s.adminFail(c, "save settings", err)
			return
		}
	}
	// 回写后的状态，前端不用再 GET 一次。
	res.Rsucc(c, s.settingsSnapshot())
}

// ── 改密码 ──────────────────────────────────────────────

func (s *Server) adminAPIPassword(c *gin.Context) {
	var in struct {
		Current string `json:"current"`
		New     string `json:"new"`
		Confirm string `json:"confirm"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		badRequest(c, err)
		return
	}
	me := middleware.Admin(c)
	if me == nil {
		s.fail(c, uierr.New(uierr.AdminSessionExpired))
		return
	}
	acc := service.NewAccount(s.DAO)
	// 顺序照旧，别为了「一次报全部错误」把它打散：
	// 先验当前密码——会话 cookie 被偷走的人不该顺手就能把密码换掉。
	if _, err := acc.Verify(me.Username, in.Current); err != nil {
		s.fail(c, uierr.New(uierr.PasswordWrong))
		return
	}
	if in.New != in.Confirm {
		s.fail(c, uierr.New(uierr.PasswordMismatch))
		return
	}
	if err := service.ValidatePassword(in.New); err != nil {
		s.fail(c, uierr.New(uierr.PasswordWeak))
		return
	}
	if err := acc.SetPassword(me.Username, in.New); err != nil {
		s.adminFail(c, "change password", err)
		return
	}
	// SetPassword 清掉了这个账号的全部会话，当前这条也在内。
	// cookie 一并清掉，否则浏览器会带着一个已经没用的 cookie 再来一次。
	s.setSessionCookie(c, "", -1)
	res.Rsucc(c, gin.H{"ok": true, "signed_out": true})
}
