package service

import (
	"github.com/aichy126/knockbox/internal/uierr"
	"strings"
	"time"

	"github.com/aichy126/knockbox/internal/dao"
	"github.com/aichy126/knockbox/internal/library/idgen"
	"github.com/aichy126/knockbox/internal/models"
	"xorm.io/xorm"
)

// ErrChannelNotFound app 里翻到一个已经被删掉的频道时也会撞上，所以带 code。
var ErrChannelNotFound = uierr.New(uierr.ChannelNotFound)

type Channel struct{ d *dao.DAO }

func NewChannel(d *dao.DAO) *Channel { return &Channel{d: d} }

// ChannelInput app 注册频道时提交的东西。
//
// 注意没有名字、图标、颜色——那些在 Meta 里，服务端只存不读。
// 服务端要读的只有 Muted / Sound / Level 三样，因为它们要写进 APNs payload。
type ChannelInput struct {
	ID string `json:"id"` // app 生成的 ULID；留空由服务端生成
	// Meta 用指针是为了区分「没传这个字段」和「传了空串」：
	// 后者是把备注清空，和其余几个可选字段一个写法。
	Meta      *string `json:"meta"`
	Muted     *bool   `json:"muted"`
	MuteUntil *int64  `json:"mute_until"` // 秒级时间戳；0 = 取消定时静音
	Sound     *string `json:"sound"`
	Level     *string `json:"level"`
}

func validLevel(l string) bool {
	switch l {
	case models.LevelPassive, models.LevelActive, models.LevelTimeSensitive, models.LevelCritical:
		return true
	}
	return false
}

// Create 注册一个频道并签发它的发送 token。
func (s *Channel) Create(userID int64, in ChannelInput) (*models.Channel, error) {
	id := in.ID
	if id == "" {
		id = idgen.ULID()
	}
	now := time.Now().Unix()
	ch := &models.Channel{
		Id: id, UserId: userID, Token: idgen.Token("ch_"),
		Sound: "default", Level: models.LevelActive,
		Status: models.StatusActive, Ctime: now, Utime: now,
	}
	if in.Meta != nil {
		ch.Meta = *in.Meta
	}
	if in.Muted != nil && *in.Muted {
		ch.Muted = 1
	}
	if in.Sound != nil {
		ch.Sound = *in.Sound
	}
	if in.Level != nil {
		if !validLevel(*in.Level) {
			return nil, uierr.New(uierr.ChannelBadLevel, *in.Level)
		}
		ch.Level = *in.Level
	}

	err := s.d.Tx(func(sess *xorm.Session) error {
		var exist models.Channel
		has, err := sess.Where("id = ?", id).Get(&exist)
		if err != nil {
			return err
		}
		if has {
			return uierr.New(uierr.ChannelExists, id)
		}
		_, err = sess.Insert(ch)
		return err
	})
	if err != nil {
		return nil, err
	}
	return ch, nil
}

// Update 只允许改四样。
// Meta 是不透明的，服务端不解析它的内容——名字改成什么是 app 的事。
func (s *Channel) Update(userID int64, id string, in ChannelInput) (*models.Channel, error) {
	if in.Level != nil && !validLevel(*in.Level) {
		return nil, uierr.New(uierr.ChannelBadLevel, *in.Level)
	}
	err := s.d.Tx(func(sess *xorm.Session) error {
		var ch models.Channel
		has, err := sess.Where("id = ? AND user_id = ?", id, userID).Get(&ch)
		if err != nil {
			return err
		}
		if !has {
			return ErrChannelNotFound
		}
		sets, args := []string{"updated_at = ?"}, []any{time.Now().Unix()}
		if in.Meta != nil {
			sets = append(sets, "meta = ?")
			args = append(args, *in.Meta)
		}
		if in.Muted != nil {
			v := 0
			if *in.Muted {
				v = 1
			}
			sets = append(sets, "muted = ?")
			args = append(args, v)
		}
		if in.MuteUntil != nil {
			sets = append(sets, "mute_until = ?")
			args = append(args, *in.MuteUntil)
		}
		if in.Sound != nil {
			sets = append(sets, "sound = ?")
			args = append(args, *in.Sound)
		}
		if in.Level != nil {
			sets = append(sets, "level = ?")
			args = append(args, *in.Level)
		}
		args = append(args, ch.Id)
		// xorm 的 Exec 签名是 (...any)，SQL 也是其中一个元素，所以要整体拼进去。
		_, err = sess.Exec(append([]any{"UPDATE channel SET " + strings.Join(sets, ", ") + " WHERE id = ?"}, args...)...)
		return err
	})
	if err != nil {
		return nil, err
	}
	return s.Get(userID, id)
}

func (s *Channel) Get(userID int64, id string) (*models.Channel, error) {
	var ch models.Channel
	has, err := s.d.Engine().Where("id = ? AND user_id = ?", id, userID).Get(&ch)
	if err != nil {
		return nil, err
	}
	if !has {
		return nil, ErrChannelNotFound
	}
	return &ch, nil
}

func (s *Channel) List(userID int64) ([]models.Channel, error) {
	var out []models.Channel
	err := s.d.Engine().Where("user_id = ? AND status = ?", userID, models.StatusActive).
		OrderBy("id").Find(&out)
	return out, err
}

// Delete 删掉一个频道本身。
//
// 消息不在这里处理：调用方决定是先 PurgeChannel 再删（连消息一起没），
// 还是只删频道把消息留下。分开是因为这两件事的可逆性完全不同。
func (s *Channel) Delete(userID int64, id string) error {
	return s.d.Tx(func(sess *xorm.Session) error {
		res, err := sess.Exec("DELETE FROM channel WHERE id = ? AND user_id = ?", id, userID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrChannelNotFound
		}
		return nil
	})
}

// RotateToken 换发送凭据，频道身份和历史不受影响。
func (s *Channel) RotateToken(userID int64, id string) (*models.Channel, error) {
	tok := idgen.Token("ch_")
	err := s.d.Tx(func(sess *xorm.Session) error {
		res, err := sess.Exec("UPDATE channel SET token = ?, updated_at = ? WHERE id = ? AND user_id = ?",
			tok, time.Now().Unix(), id, userID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrChannelNotFound
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.Get(userID, id)
}
