package cli

import (
	"fmt"
	"strings"

	"github.com/aichy126/knockbox/internal/service"
)

// printFirstAdmin 把自动建出来的管理员告诉运行它的人。
//
// 走 stdout 而不是日志：这几行必须在一屏日志里一眼认出来，
// 而随机密码只有这一次机会被看到——库里存的是哈希，错过了就只能重设。
// 文字用英文，和 README、config.toml.example 一致。
func printFirstAdmin(a *service.FirstAdmin, externalURL string) {
	loginURL := strings.TrimSuffix(strings.TrimSpace(externalURL), "/") + "/login"
	if loginURL == "/login" {
		loginURL = "http://localhost:8080/login"
	}

	line := strings.Repeat("─", 64)
	fmt.Printf("\n%s\n", line)
	fmt.Printf("  Knockbox created the first admin account.\n\n")
	fmt.Printf("    Sign in    %s\n", loginURL)
	fmt.Printf("    Username   %s\n", a.Username)
	if a.Generated {
		fmt.Printf("    Password   %s\n\n", a.Password)
		fmt.Printf("  This password is printed once and nowhere else: the database\n")
		fmt.Printf("  keeps only a hash of it. Sign in and change it on the settings\n")
		fmt.Printf("  page of the admin interface.\n")
	} else {
		fmt.Printf("    Password   the one from bootstrap.password\n\n")
		fmt.Printf("  You can change it later on the settings page of the admin\n")
		fmt.Printf("  interface, and clear bootstrap.password from the config.\n")
	}
	fmt.Printf("%s\n\n", line)
}
