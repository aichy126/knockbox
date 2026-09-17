package cli

import (
	"errors"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/aichy126/knockbox/internal/models"
	"github.com/aichy126/knockbox/internal/service"
	"github.com/spf13/cobra"
)

func newUserCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "user",
		Short: "管理账号",
		Long: "管理能登录管理界面的账号。\n\n" +
			"所有账号操作都在这里，任何情况下都不需要直接改数据库。",
	}
	c.AddCommand(userAddCmd(), userPasswdCmd(), userListCmd(), userEnableCmd(), userDisableCmd())
	return c
}

func userAddCmd() *cobra.Command {
	var password, role string
	c := &cobra.Command{
		Use:   "add <用户名>",
		Short: "新建账号",
		Args:  cobra.ExactArgs(1),
		Example: "  knockbox user add alice                 # 交互式输入密码（推荐）\n" +
			"  echo 'hunter2hunter2' | knockbox user add bob\n" +
			"  knockbox user add ci --password=xxx --role=member",
		RunE: func(cmd *cobra.Command, args []string) error {
			pw, err := readPassword(password, true)
			if err != nil {
				return err
			}
			_, d, err := openDAO()
			if err != nil {
				return err
			}
			u, err := service.NewAccount(d).Create(args[0], pw, role)
			if errors.Is(err, service.ErrUserExists) {
				return fmt.Errorf("用户名 %q 已存在。改密码用：knockbox user passwd %s", args[0], args[0])
			}
			if err != nil {
				return err
			}
			fmt.Printf("已创建账号 %s（%s，id=%d）\n", u.Username, u.Role, u.Id)
			return nil
		},
	}
	c.Flags().StringVar(&password, "password", "", "直接给密码（非交互场景用；会进 shell history 和 ps）")
	c.Flags().StringVar(&role, "role", models.RoleAdmin, "角色：admin 能进管理界面，member 只收消息")
	return c
}

// userPasswdCmd 是这个 CLI 存在的首要理由。
// 缺了它，「本地库进不去后台」就只能靠写程序生成 bcrypt 哈希再 UPDATE 数据库。
func userPasswdCmd() *cobra.Command {
	var password string
	c := &cobra.Command{
		Use:   "passwd <用户名>",
		Short: "重设密码",
		Long:  "重设密码。改完会立刻踢掉该账号的全部登录会话。",
		Args:  cobra.ExactArgs(1),
		Example: "  knockbox user passwd alice\n" +
			"  echo 'newpassword' | knockbox user passwd alice",
		RunE: func(cmd *cobra.Command, args []string) error {
			pw, err := readPassword(password, true)
			if err != nil {
				return err
			}
			_, d, err := openDAO()
			if err != nil {
				return err
			}
			if err := service.NewAccount(d).SetPassword(args[0], pw); err != nil {
				if errors.Is(err, service.ErrUserNotFound) {
					return fmt.Errorf("账号 %q 不存在。看看有哪些：knockbox user list", args[0])
				}
				return err
			}
			fmt.Printf("已重设 %s 的密码，该账号的登录会话已全部失效\n", args[0])
			return nil
		},
	}
	c.Flags().StringVar(&password, "password", "", "直接给密码（非交互场景用）")
	return c
}

func userListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "列出账号",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, d, err := openDAO()
			if err != nil {
				return err
			}
			us, err := service.NewAccount(d).List()
			if err != nil {
				return err
			}
			if len(us) == 0 {
				fmt.Println("还没有任何账号。启动一次服务（knockbox serve）会自动建出管理员，或者手动建：knockbox user add <用户名>")
				return nil
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(w, "ID\t用户名\t角色\t状态\t能登录\t最后登录")
			for _, u := range us {
				name := u.Username
				if name == "" {
					name = "(无，仅收消息)"
				}
				status := "正常"
				if u.Status != models.StatusActive {
					status = "已停用"
				}
				login := "否"
				if u.PasswordHash != "" && u.Role == models.RoleAdmin {
					login = "是"
				}
				_, _ = fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\n",
					u.Id, name, u.Role, status, login, humanTime(u.LastLoginAt))
			}
			return w.Flush()
		},
	}
}

func userEnableCmd() *cobra.Command {
	return userStatusCmd("enable", "启用账号", models.StatusActive)
}
func userDisableCmd() *cobra.Command {
	return userStatusCmd("disable", "停用账号（并踢掉其全部会话）", models.StatusDisabled)
}

func userStatusCmd(verb, short string, status int) *cobra.Command {
	return &cobra.Command{
		Use:   verb + " <用户名>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, d, err := openDAO()
			if err != nil {
				return err
			}
			if err := service.NewAccount(d).SetStatus(args[0], status); err != nil {
				if errors.Is(err, service.ErrUserNotFound) {
					return fmt.Errorf("账号 %q 不存在。看看有哪些：knockbox user list", args[0])
				}
				return err
			}
			fmt.Printf("已%s %s\n", short[:2], args[0])
			return nil
		},
	}
}

func humanTime(ts int64) string {
	if ts == 0 {
		return "从未"
	}
	return time.Unix(ts, 0).Format("2006-01-02 15:04")
}
