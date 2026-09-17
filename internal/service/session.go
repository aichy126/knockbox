package service

import (
	"errors"
	"time"

	"github.com/aichy126/knockbox/internal/dao"
	"github.com/aichy126/knockbox/internal/library/idgen"
	"github.com/aichy126/knockbox/internal/models"
	"xorm.io/xorm"
)

// SessionTTL 管理会话有效期。
const SessionTTL = 14 * 24 * time.Hour

// sessionTouchInterval last_seen_at 最多这么久刷新一次。
//
// 每个请求都写的话，SQLite 下等于给每一个后台请求加一次写锁；
// 而这个字段只用来在「活跃会话」列表里显示「最近活动」，精确到分钟绰绰有余。
const sessionTouchInterval = 5 * time.Minute

var ErrSessionInvalid = errors.New("会话无效或已过期")

type Session struct{ d *dao.DAO }

func NewSession(d *dao.DAO) *Session { return &Session{d: d} }

// Login 校验账号密码并签发会话。
//
// 会话存库而不是签 JWT：要能在界面上「踢掉其它设备」，
// 也要能在改密码后立刻让旧 cookie 失效——无状态 token 做不到这两件事。
func (s *Session) Login(username, password, ua, ip string) (string, *models.User, error) {
	u, err := NewAccount(s.d).Verify(username, password)
	if err != nil {
		return "", nil, err
	}
	if u.Role != models.RoleAdmin {
		return "", nil, errors.New("该账号不能登录管理界面")
	}
	raw := idgen.Token("sk_")
	now := time.Now()
	err = s.d.Tx(func(sess *xorm.Session) error {
		if _, err := sess.Insert(&models.Session{
			Id: hashToken(raw), UserId: u.Id, UserAgent: truncateRunes(ua, 200), IP: ip,
			ExpiresAt: now.Add(SessionTTL).Unix(), Ctime: now.Unix(), LastSeenAt: now.Unix(),
		}); err != nil {
			return err
		}
		_, err := sess.Exec("UPDATE user SET last_login_at = ? WHERE id = ?", now.Unix(), u.Id)
		return err
	})
	if err != nil {
		return "", nil, err
	}
	return raw, u, nil
}

// Verify 校验会话 cookie。
func (s *Session) Verify(raw string) (*models.User, error) {
	var sess models.Session
	has, err := s.d.Engine().Where("id = ?", hashToken(raw)).Get(&sess)
	if err != nil {
		return nil, err
	}
	if !has || sess.ExpiresAt < time.Now().Unix() {
		return nil, ErrSessionInvalid
	}
	var u models.User
	has, err = s.d.Engine().ID(sess.UserId).Get(&u)
	if err != nil {
		return nil, err
	}
	if !has || u.Status != models.StatusActive || u.Role != models.RoleAdmin {
		return nil, ErrSessionInvalid
	}
	if now := time.Now().Unix(); now-sess.LastSeenAt >= int64(sessionTouchInterval/time.Second) {
		_, _ = s.d.Engine().Exec("UPDATE session SET last_seen_at = ? WHERE id = ?", now, sess.Id)
	}
	return &u, nil
}

func (s *Session) Logout(raw string) error {
	_, err := s.d.Engine().Exec("DELETE FROM session WHERE id = ?", hashToken(raw))
	return err
}

// List 列出一个账号的活跃会话，界面上用来「踢出其它设备」。
func (s *Session) List(userID int64) ([]models.Session, error) {
	var out []models.Session
	err := s.d.Engine().Where("user_id = ? AND expires_at > ?", userID, time.Now().Unix()).
		OrderBy("last_seen_at DESC").Find(&out)
	return out, err
}

// GC 清掉过期会话与过期配对码。
func (s *Session) GC() error {
	now := time.Now().Unix()
	if _, err := s.d.Engine().Exec("DELETE FROM session WHERE expires_at < ?", now); err != nil {
		return err
	}
	// 已用或已过期超过一天的配对码没有保留价值。
	_, err := s.d.Engine().Exec(
		"DELETE FROM pair_code WHERE expires_at < ? OR (used_at <> 0 AND used_at < ?)",
		now-86400, now-86400)
	return err
}
