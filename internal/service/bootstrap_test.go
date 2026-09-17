package service

import (
	"testing"
)

// 空库上要建出一个能立刻登录的管理员，密码随机且当场返回。
func TestEnsureFirstAdminOnEmptyDatabase(t *testing.T) {
	d, _ := newTestDAO(t)
	a := NewAccount(d)

	first, err := a.EnsureFirstAdmin("", "")
	if err != nil {
		t.Fatal(err)
	}
	if first == nil {
		t.Fatal("空库上应当建出管理员")
	}
	if first.Username != DefaultAdminUsername {
		t.Errorf("用户名应当是 %q，得到 %q", DefaultAdminUsername, first.Username)
	}
	if !first.Generated {
		t.Error("没给密码时应当是生成的")
	}
	// 返回的密码必须真的能登录——生成密码和写进库里的哈希对不上时，
	// 表现是一台谁也进不去的服务器，而日志里那串密码看起来完全正常。
	if _, err := a.Verify(first.Username, first.Password); err != nil {
		t.Fatalf("打印出来的密码登不进去: %v", err)
	}
}

// 已经有账号的库一个字都不该动：升级上来的实例不能被塞进一个新管理员。
func TestEnsureFirstAdminSkipsNonEmptyDatabase(t *testing.T) {
	d, _ := newTestDAO(t)
	a := NewAccount(d)
	mustUser(t, d)

	first, err := a.EnsureFirstAdmin("", "")
	if err != nil {
		t.Fatal(err)
	}
	if first != nil {
		t.Fatalf("已有账号时不该再建，却建出了 %q", first.Username)
	}
	n, err := a.Count()
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("账号数应当还是 1，得到 %d", n)
	}
}

// 配置里给了用户名和密码就用它们，并且不标成「生成的」——
// 调用方据此决定要不要把密码打在屏幕上。
func TestEnsureFirstAdminUsesConfiguredCredentials(t *testing.T) {
	d, _ := newTestDAO(t)
	a := NewAccount(d)

	first, err := a.EnsureFirstAdmin("alice", "hunter2hunter2")
	if err != nil {
		t.Fatal(err)
	}
	if first == nil {
		t.Fatal("空库上应当建出管理员")
	}
	if first.Username != "alice" || first.Generated {
		t.Errorf("应当用配置里的账号且不标成生成的，得到 %q generated=%v", first.Username, first.Generated)
	}
	if _, err := a.Verify("alice", "hunter2hunter2"); err != nil {
		t.Fatalf("配置里的密码登不进去: %v", err)
	}
}

// 生成的密码要足够长，且不含手抄时会认错的字符。
func TestRandomPasswordShape(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		pw, err := randomPassword(16)
		if err != nil {
			t.Fatal(err)
		}
		if len(pw) != 16 {
			t.Fatalf("长度应当是 16，得到 %d", len(pw))
		}
		for _, r := range pw {
			switch r {
			case '0', 'O', 'o', 'l', '1', 'I':
				t.Fatalf("生成的密码里出现了易认错的字符 %q: %s", r, pw)
			}
		}
		if seen[pw] {
			t.Fatalf("50 次里生成了重复的密码: %s", pw)
		}
		seen[pw] = true
	}
}
