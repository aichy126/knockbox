package service

import (
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/aichy126/knockbox/internal/dao"
	"github.com/aichy126/knockbox/internal/library/idgen"
	"github.com/aichy126/knockbox/internal/models"
	"xorm.io/xorm"
)

// Scheme app 的自定义 URL scheme。
//
// 只能用 custom scheme，不能用 Universal Link：associated-domains 是编译期
// 写死的域名列表，而每个自建者的域名都不一样，永远进不了那个列表。
const Scheme = "knockbox"

type Pair struct{ d *dao.DAO }

func NewPair(d *dao.DAO) *Pair { return &Pair{d: d} }

type PairCode struct {
	Code      string
	Display   string // 给人看的分组形式 K7M2-9XQP
	DeepLink  string
	ExpiresAt time.Time
	// UserID 这个码属于谁。0 = 开放注册（兑换时才建用户）。
	// 界面上要能说清「扫完这台设备就属于谁」，所以调用方得知道。
	UserID int64
}

// Issue 签发一次性配对码。
//
// 二维码里放的是配对码而不是长期 token：二维码会被拍照、截屏、贴进聊天记录，
// 而配对码单次使用、十分钟过期，漏了也换不来什么。
func (p *Pair) Issue(userID int64, host, issuedBy string, ttl time.Duration) (*PairCode, error) {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	if host == "" {
		return nil, errors.New("server.external_url 没配，生成的配对链接会指向错误的地址")
	}
	now := time.Now()
	rec := &models.PairCode{
		Code:      idgen.PairCode(),
		UserId:    userID,
		IssuedBy:  issuedBy,
		ExpiresAt: now.Add(ttl).Unix(),
		Ctime:     now.Unix(),
	}
	if err := p.d.Tx(func(sess *xorm.Session) error {
		_, err := sess.Insert(rec)
		return err
	}); err != nil {
		return nil, err
	}
	return view(rec, host), nil
}

// view 把库里的一行包装成调用方拿到的形状。深链只在这里拼一次。
func view(rec *models.PairCode, host string) *PairCode {
	return &PairCode{
		Code:      rec.Code,
		Display:   idgen.FormatPairCode(rec.Code),
		DeepLink:  fmt.Sprintf("%s://pair?h=%s&c=%s", Scheme, url.QueryEscape(host), rec.Code),
		ExpiresAt: time.Unix(rec.ExpiresAt, 0),
		UserID:    rec.UserId,
	}
}

// Pending 取一张还没用掉、也还没过期的【开放注册】码。
//
// 接入页靠它在刷新时继续用同一张码：每加载一次就签发一张的话，
// pair_code 的行数等于首页被打开的次数，而其中绝大多数永远不会被用到。
// minLeft 是至少还要剩多少有效期——只剩几秒的码交出去，用户还没扫完就废了。
//
// 只认 user_id = 0：这样即便调用方递来一张管理员签发的码，也不会被当成可复用的。
func (p *Pair) Pending(code, host string, minLeft time.Duration) (*PairCode, bool) {
	code = idgen.NormalizePairCode(code)
	if code == "" {
		return nil, false
	}
	var rec models.PairCode
	has, err := p.d.Engine().Where(
		"code = ? AND user_id = 0 AND used_at = 0 AND expires_at > ?",
		code, time.Now().Add(minLeft).Unix()).Get(&rec)
	if err != nil || !has {
		return nil, false
	}
	return view(&rec, host), true
}

// Redeem 用配对码换设备身份。单次使用：先标记已用，再让调用方在同一事务里建设备。
func (p *Pair) Redeem(sess *xorm.Session, code, deviceUUID string) (int64, error) {
	code = idgen.NormalizePairCode(code)
	var rec models.PairCode
	has, err := sess.Where("code = ?", code).Get(&rec)
	if err != nil {
		return 0, err
	}
	if !has {
		return 0, errors.New("配对码不存在")
	}
	if rec.UsedAt != 0 {
		return 0, errors.New("配对码已被使用过")
	}
	if rec.ExpiresAt < time.Now().Unix() {
		return 0, errors.New("配对码已过期，请重新生成")
	}
	if _, err := sess.Exec("UPDATE pair_code SET used_at = ?, used_by = ? WHERE id = ? AND used_at = 0",
		time.Now().Unix(), deviceUUID, rec.Id); err != nil {
		return 0, err
	}
	// user_id = 0 是「开放注册」的码：兑换的那一刻才建用户。
	// 这样公共实例和自建实例走的是【同一条配对流程】，app 端一行都不用改。
	if rec.UserId == 0 {
		uid, err := p.createUser(sess, deviceUUID)
		if err != nil {
			return 0, err
		}
		// **把建出来的用户回写到这张码上。**
		// 不写的话 user_id 永远是 0：接入页事后查不到「这个码对应谁」，
		// 审计上也看不出这张码建出了哪个用户。
		if _, err := sess.Exec("UPDATE pair_code SET user_id = ? WHERE id = ?", uid, rec.Id); err != nil {
			return 0, err
		}
		return uid, nil
	}
	return rec.UserId, nil
}

// createUser 给开放注册建一个没有登录凭据的用户。
//
// **没有用户名密码是刻意的**：扫码注册的人不该被要求想一个密码，
// 身份就是设备上那把 token。代价是设备全丢等于身份没了——这一点必须在页面上说清楚，
// 不能让用户以为能找回。
func (p *Pair) createUser(sess *xorm.Session, deviceUUID string) (int64, error) {
	now := time.Now().Unix()
	u := &models.User{
		Name:   "u_" + idgen.ULID()[:8],
		Role:   models.RoleMember,
		Status: models.StatusActive,
		Ctime:  now, Utime: now,
	}
	if _, err := sess.Insert(u); err != nil {
		return 0, err
	}
	return u.Id, nil
}
