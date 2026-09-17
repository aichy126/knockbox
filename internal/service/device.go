package service

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aichy126/knockbox/internal/dao"
	"github.com/aichy126/knockbox/internal/library/idgen"
	"github.com/aichy126/knockbox/internal/models"
	"xorm.io/xorm"
)

type Device struct{ d *dao.DAO }

func NewDevice(d *dao.DAO) *Device { return &Device{d: d} }

// RegisterInput 配对时 app 提交的设备信息。
type RegisterInput struct {
	PairCode   string `json:"pair_code"`
	UUID       string `json:"uuid"` // 客户端生成、存钥匙串，重装 app 不变
	Name       string `json:"name"`
	Platform   string `json:"platform"`
	Model      string `json:"model"`
	OSVersion  string `json:"os_version"`
	AppVersion string `json:"app_version"`
	APNsToken  string `json:"apns_token"` // 可以为空：用户还没授权通知
	APNsEnv    string `json:"apns_env"`
}

type RegisterResult struct {
	DeviceToken string         `json:"device_token"` // 只在这一次返回，服务端只存哈希
	DeviceUUID  string         `json:"device_uuid"`
	UserID      int64          `json:"user_id"`
	Server      map[string]any `json:"server"`
}

// normEnv 兜住客户端上报的各种写法。
// 判定本身必须由 app 读 embedded provisioning profile 的 aps-environment 得出，
// 不能用 #if DEBUG —— TestFlight 是 Release 构建但走 production。
func normEnv(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "sandbox", "development", "dev":
		return models.EnvSandbox
	default:
		return models.EnvProduction
	}
}

// Pair 用配对码换一台设备的长期身份。
//
// 整个过程在一个事务里：配对码作废、设备落库、旧设备让出 APNs token，
// 三件事要么一起成、要么一起回滚。否则会出现「码用掉了但设备没建成」这种谁都救不回来的状态。
func (s *Device) Pair(in RegisterInput, serverName, version string) (*RegisterResult, error) {
	if strings.TrimSpace(in.UUID) == "" {
		return nil, errors.New("缺少设备 uuid")
	}
	token := idgen.Token("dv_")
	now := time.Now().Unix()
	var userID int64

	err := s.d.Tx(func(sess *xorm.Session) error {
		uid, err := NewPair(s.d).Redeem(sess, in.PairCode, in.UUID)
		if err != nil {
			return err
		}
		userID = uid

		dev := &models.Device{
			UUID: in.UUID, UserId: uid, Name: in.Name, Platform: in.Platform,
			Model: in.Model, OSVersion: in.OSVersion, AppVersion: in.AppVersion,
			APNsToken: strings.TrimSpace(in.APNsToken), APNsEnv: normEnv(in.APNsEnv),
			AuthHash: hashToken(token), AuthPrefix: token[:11],
			Status: models.DeviceLive, Ctime: now, Utime: now, LastSeenAt: now,
		}
		if err := releaseAPNsToken(sess, dev.APNsToken, in.UUID, now); err != nil {
			return err
		}

		// 同一台设备重新配对（app 删了重装、或换了账号）走更新而不是插入，
		// 否则会因为 uuid 唯一索引失败。
		var old models.Device
		has, err := sess.Where("uuid = ?", in.UUID).Get(&old)
		if err != nil {
			return err
		}
		if has {
			dev.Id = old.Id
			dev.Ctime = old.Ctime
			_, err = sess.ID(old.Id).AllCols().Omit("id", "created_at").Update(dev)
			return err
		}
		_, err = sess.Insert(dev)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &RegisterResult{
		DeviceToken: token, DeviceUUID: in.UUID, UserID: userID,
		Server: map[string]any{"name": serverName, "version": version},
	}, nil
}

// UpdatePushToken 客户端每次拿到 APNs token 都无条件调这里，服务端比对不同才写库。
func (s *Device) UpdatePushToken(dev *models.Device, apnsToken, env, appVersion, osVersion string) error {
	apnsToken = strings.TrimSpace(apnsToken)
	env = normEnv(env)
	if apnsToken == dev.APNsToken && env == dev.APNsEnv &&
		appVersion == dev.AppVersion && osVersion == dev.OSVersion {
		return s.touch(dev)
	}
	now := time.Now().Unix()
	return s.d.Tx(func(sess *xorm.Session) error {
		if err := releaseAPNsToken(sess, apnsToken, dev.UUID, now); err != nil {
			return err
		}
		_, err := sess.Exec(`UPDATE device SET apns_token = ?, apns_env = ?, app_version = ?,
			os_version = ?, status = ?, fail_count = 0, last_seen_at = ?, updated_at = ? WHERE id = ?`,
			apnsToken, env, appVersion, osVersion, models.DeviceLive, now, now, dev.Id)
		return err
	})
}

func (s *Device) touch(dev *models.Device) error {
	_, err := s.d.Engine().Exec("UPDATE device SET last_seen_at = ? WHERE id = ?", time.Now().Unix(), dev.Id)
	return err
}

// releaseAPNsToken 让出这个 APNs token 的旧归属。
//
// 同一个 token 绝不能挂在两台设备上，否则一条消息推两遍。换机、从备份恢复
// 都会把 token 带到新设备行上，所以冲突时「新行接管、老行清空并标记失效」。
func releaseAPNsToken(sess *xorm.Session, apnsToken, keepUUID string, now int64) error {
	if apnsToken == "" {
		return nil
	}
	_, err := sess.Exec(
		`UPDATE device SET apns_token = '', status = ?, updated_at = ? WHERE apns_token = ? AND uuid <> ?`,
		models.DeviceUnregistered, now, apnsToken, keepUUID)
	return err
}

// ListByUser 列出一个成员的设备。
func (s *Device) ListByUser(userID int64) ([]models.Device, error) {
	var out []models.Device
	err := s.d.Engine().Where("user_id = ?", userID).OrderBy("id").Find(&out)
	return out, err
}

// PushTargets 一个成员当前能收推送的设备。
// 只有既在用、又有 APNs token 的才算——没授权通知的设备照常同步历史，只是推不了。
func (s *Device) PushTargets(userID int64) ([]models.Device, error) {
	var out []models.Device
	err := s.d.Engine().
		Where("user_id = ? AND status = ? AND apns_token <> ''", userID, models.DeviceLive).
		OrderBy("id").Find(&out)
	return out, err
}

// Logout 主动登出一台设备。
func (s *Device) Logout(userID int64, uuid string) error {
	res, err := s.d.Engine().Exec(
		`UPDATE device SET status = ?, apns_token = '', updated_at = ? WHERE user_id = ? AND uuid = ?`,
		models.DeviceLoggedOut, time.Now().Unix(), userID, uuid)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("设备 %s 不存在", uuid)
	}
	return nil
}
