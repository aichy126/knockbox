package service

import (
	"crypto/rand"
	"fmt"
	"strings"

	"github.com/aichy126/knockbox/internal/models"
)

// DefaultAdminUsername 没配用户名时用它。
const DefaultAdminUsername = "admin"

// FirstAdmin 自动建出来的第一个管理员。Password 只在这一次返回，
// 库里存的是 bcrypt 哈希，之后谁也拿不回明文。
type FirstAdmin struct {
	Username string
	Password string
	// Generated 为真表示密码是随机生成的，调用方必须把它显示出来——
	// 不显示的话这台服务器就再也进不去了。
	Generated bool
}

// EnsureFirstAdmin 在一个账号都没有的库上建出第一个管理员，已有账号则什么都不做。
//
// 为什么这件事放在启动流程里，而不是让人自己跑一条 `user add`：
// 自建这个服务的人不都是程序员。「进容器再敲一行命令」这道门槛足以让相当一部分人
// 停在第一步，而这一步没完成，服务是起着的、却谁也登不进去——
// 表现是一个能打开却用不了的登录页，最难自己查出原因的那种失败。
//
// 判据是【库里一个账号都没有】，不是「这是不是第一次启动」：
// 升级上来的实例、或者管理员把自己删光了的实例，前者不会被塞进一个新账号，
// 后者能自己恢复出一个入口。
func (a *Account) EnsureFirstAdmin(username, password string) (*FirstAdmin, error) {
	n, err := a.Count()
	if err != nil {
		return nil, err
	}
	if n > 0 {
		return nil, nil
	}

	username = strings.TrimSpace(username)
	if username == "" {
		username = DefaultAdminUsername
	}
	generated := false
	if password == "" {
		password, err = randomPassword(16)
		if err != nil {
			return nil, err
		}
		generated = true
	}

	u, err := a.Create(username, password, models.RoleAdmin)
	if err != nil {
		return nil, fmt.Errorf("cannot create the first admin account: %w", err)
	}
	return &FirstAdmin{Username: u.Username, Password: password, Generated: generated}, nil
}

// Count 现有账号数。
func (a *Account) Count() (int64, error) {
	return a.d.Engine().Count(new(models.User))
}

// randomPassword 生成一个给人抄一次的密码。
//
// 字母表里没有 0 O o l 1 I：这串多半要从终端日志里手抄进浏览器，
// 而抄错的代价是一次登录失败加一次「是不是坏了」的怀疑。
// 少掉这几个字符对强度的影响可以忽略——16 位仍有约 90 bit。
const passwordAlphabet = "abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func randomPassword(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	// 取模会让字母表前几个字符略微偏多。字母表长 56、模 256 的偏差在 1% 量级，
	// 对一个开机就该被改掉的初始密码不值得引入拒绝采样。
	out := make([]byte, n)
	for i, c := range b {
		out[i] = passwordAlphabet[int(c)%len(passwordAlphabet)]
	}
	return string(out), nil
}
