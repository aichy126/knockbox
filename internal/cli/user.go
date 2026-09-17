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
		Short: "Manage accounts",
		Long: "Manage the accounts that can sign in to the admin interface.\n\n" +
			"Everything about accounts happens here; there is never a reason to edit the database by hand.",
	}
	c.AddCommand(userAddCmd(), userPasswdCmd(), userListCmd(), userEnableCmd(), userDisableCmd())
	return c
}

func userAddCmd() *cobra.Command {
	var password, role string
	c := &cobra.Command{
		Use:   "add <name>",
		Short: "Create an account",
		Args:  cobra.ExactArgs(1),
		Example: "  knockbox user add alice                 # type the password when asked (recommended)\n" +
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
				return fmt.Errorf("an account named %q already exists. To change its password: knockbox user passwd %s", args[0], args[0])
			}
			if err != nil {
				return err
			}
			fmt.Printf("Created %s (%s, id=%d)\n", u.Username, u.Role, u.Id)
			return nil
		},
	}
	c.Flags().StringVar(&password, "password", "", "give the password directly (for scripts; it lands in shell history and in ps)")
	c.Flags().StringVar(&role, "role", models.RoleAdmin, "role: admin can sign in to the admin interface, member only receives messages")
	return c
}

// userPasswdCmd 是这个 CLI 存在的首要理由。
// 缺了它，「本地库进不去后台」就只能靠写程序生成 bcrypt 哈希再 UPDATE 数据库。
func userPasswdCmd() *cobra.Command {
	var password string
	c := &cobra.Command{
		Use:   "passwd <name>",
		Short: "Reset a password",
		Long:  "Reset a password. Every session of that account is invalidated immediately.",
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
					return fmt.Errorf("no account named %q. See what exists: knockbox user list", args[0])
				}
				return err
			}
			fmt.Printf("Password for %s has been reset; all of its sessions are now invalid\n", args[0])
			return nil
		},
	}
	c.Flags().StringVar(&password, "password", "", "give the password directly (for scripts)")
	return c
}

func userListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List accounts",
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
				fmt.Println("No accounts yet. Starting the server once (knockbox serve) creates an admin, or create one now: knockbox user add <name>")
				return nil
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(w, "ID\tNAME\tROLE\tSTATUS\tCAN SIGN IN\tLAST SIGN-IN")
			for _, u := range us {
				name := u.Username
				if name == "" {
					name = "(none, receives only)"
				}
				status := "active"
				if u.Status != models.StatusActive {
					status = "disabled"
				}
				login := "no"
				if u.PasswordHash != "" && u.Role == models.RoleAdmin {
					login = "yes"
				}
				_, _ = fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\n",
					u.Id, name, u.Role, status, login, humanTime(u.LastLoginAt))
			}
			return w.Flush()
		},
	}
}

func userEnableCmd() *cobra.Command {
	return userStatusCmd("enable", "Enable an account", models.StatusActive)
}
func userDisableCmd() *cobra.Command {
	return userStatusCmd("disable", "Disable an account and invalidate its sessions", models.StatusDisabled)
}

func userStatusCmd(verb, short string, status int) *cobra.Command {
	return &cobra.Command{
		Use:   verb + " <name>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, d, err := openDAO()
			if err != nil {
				return err
			}
			if err := service.NewAccount(d).SetStatus(args[0], status); err != nil {
				if errors.Is(err, service.ErrUserNotFound) {
					return fmt.Errorf("no account named %q. See what exists: knockbox user list", args[0])
				}
				return err
			}
			fmt.Printf("%s is now %sd\n", args[0], verb)
			return nil
		},
	}
}

func humanTime(ts int64) string {
	if ts == 0 {
		return "never"
	}
	return time.Unix(ts, 0).Format("2006-01-02 15:04")
}
