// Package service 业务编排。
package service

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/aichy126/knockbox/internal/dao"
	"github.com/aichy126/knockbox/internal/models"
	"golang.org/x/crypto/bcrypt"
	"xorm.io/xorm"
)

var (
	ErrUserNotFound = errors.New("用户不存在")
	ErrUserExists   = errors.New("用户名已被占用")
)

type Account struct{ d *dao.DAO }

func NewAccount(d *dao.DAO) *Account { return &Account{d: d} }

// ValidateUsername 只允许字母数字和 . _ -，因为它会出现在 URL 和命令行里。
func ValidateUsername(name string) error {
	if len(name) < 2 || len(name) > 32 {
		return fmt.Errorf("用户名长度要在 2-32 之间，得到 %d", len(name))
	}
	for _, r := range name {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '.' && r != '_' && r != '-' {
			return fmt.Errorf("用户名只能用字母、数字和 . _ -，不能有 %q", r)
		}
	}
	return nil
}

// ValidatePassword 只卡长度下限。
// 不强制「大小写数字符号各一个」那套——它会把人逼向 Passw0rd! 这种可预测的模式，
// 而长度才是真正有用的那个维度。
func ValidatePassword(pw string) error {
	if len([]rune(pw)) < 8 {
		return fmt.Errorf("密码至少 8 位，得到 %d 位", len([]rune(pw)))
	}
	if strings.TrimSpace(pw) == "" {
		return errors.New("密码不能全是空白字符")
	}
	return nil
}

func (a *Account) GetByUsername(name string) (*models.User, error) {
	var u models.User
	has, err := a.d.Engine().Where("username = ?", name).Get(&u)
	if err != nil {
		return nil, err
	}
	if !has {
		return nil, ErrUserNotFound
	}
	return &u, nil
}

// Create 建一个能登录管理界面的账号。
func (a *Account) Create(username, password, role string) (*models.User, error) {
	if err := ValidateUsername(username); err != nil {
		return nil, err
	}
	if err := ValidatePassword(password); err != nil {
		return nil, err
	}
	if role != models.RoleAdmin && role != models.RoleMember {
		return nil, fmt.Errorf("role 只能是 %s 或 %s", models.RoleAdmin, models.RoleMember)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	now := time.Now().Unix()
	u := &models.User{
		Name: username, Username: username, PasswordHash: string(hash),
		Role: role, Status: models.StatusActive, Ctime: now, Utime: now,
	}
	err = a.d.Tx(func(sess *xorm.Session) error {
		var exist models.User
		has, err := sess.Where("username = ?", username).Get(&exist)
		if err != nil {
			return err
		}
		if has {
			return ErrUserExists
		}
		_, err = sess.Insert(u)
		return err
	})
	if err != nil {
		return nil, err
	}
	return u, nil
}

// SetPassword 重设密码。
//
// 有了它，重设密码就不需要使用者自己生成 bcrypt 哈希再去改库。
// 改完清掉该用户的全部会话，否则旧 cookie 仍然有效。
func (a *Account) SetPassword(username, password string) error {
	if err := ValidatePassword(password); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return a.d.Tx(func(sess *xorm.Session) error {
		var u models.User
		has, err := sess.Where("username = ?", username).Get(&u)
		if err != nil {
			return err
		}
		if !has {
			return ErrUserNotFound
		}
		if _, err := sess.Exec(
			"UPDATE user SET password_hash = ?, updated_at = ? WHERE id = ?",
			string(hash), time.Now().Unix(), u.Id); err != nil {
			return err
		}
		_, err = sess.Exec("DELETE FROM session WHERE user_id = ?", u.Id)
		return err
	})
}

// dummyPasswordHash 是一个固定的 bcrypt 哈希，只用于在用户不存在时消耗与真实
// 校验相当的时间。它不对应任何可用的密码。
const dummyPasswordHash = "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"

// Verify 校验用户名密码。
// 用户不存在时也跑一次 bcrypt，让两种失败的耗时一致，不给用户名枚举留时间侧信道。
func (a *Account) Verify(username, password string) (*models.User, error) {
	u, err := a.GetByUsername(username)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			// 故意丢弃结果：这次比对只为把耗时抹平到和真实校验一致。
			_ = bcrypt.CompareHashAndPassword(
				[]byte(dummyPasswordHash), []byte(password))
		}
		return nil, ErrUserNotFound
	}
	if u.Status != models.StatusActive {
		return nil, errors.New("账号已停用")
	}
	if u.PasswordHash == "" {
		return nil, errors.New("该账号没有设置密码，不能登录管理界面")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return nil, errors.New("密码不正确")
	}
	return u, nil
}

func (a *Account) List() ([]models.User, error) {
	var us []models.User
	err := a.d.Engine().OrderBy("id").Find(&us)
	return us, err
}

func (a *Account) SetStatus(username string, status int) error {
	return a.d.Tx(func(sess *xorm.Session) error {
		res, err := sess.Exec("UPDATE user SET status = ?, updated_at = ? WHERE username = ?",
			status, time.Now().Unix(), username)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrUserNotFound
		}
		if status != models.StatusActive {
			_, err = sess.Exec(
				"DELETE FROM session WHERE user_id = (SELECT id FROM user WHERE username = ?)", username)
		}
		return err
	})
}

// hashToken 与 middleware.Hash 必须一致：设备 token 用 sha256 比对。
// 它是 256 位高熵随机串，不需要 bcrypt 那种抗字典的开销。
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
