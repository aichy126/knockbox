package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/aichy126/knockbox/internal/service"
	"github.com/mdp/qrterminal/v3"
	"github.com/spf13/cobra"
)

func newPairCmd() *cobra.Command {
	var username string
	var ttl time.Duration
	c := &cobra.Command{
		Use:   "pair",
		Short: "Issue a pairing code and print it as a QR code",
		Long: "Issue a single-use pairing code and print it as a QR code for the app to scan.\n\n" +
			"No server to start and no browser needed — this is the path for the first device, and for the day a phone is lost.",
		Example: "  knockbox pair\n" +
			"  knockbox pair --user alice --ttl 1h",
		RunE: func(cmd *cobra.Command, args []string) error {
			app, d, err := openDAO()
			if err != nil {
				return err
			}
			acc := service.NewAccount(d)

			var userID int64
			if username != "" {
				u, err := acc.GetByUsername(username)
				if err != nil {
					return fmt.Errorf("no account named %q. See what exists: knockbox user list", username)
				}
				userID = u.Id
			} else {
				us, err := acc.List()
				if err != nil {
					return err
				}
				if len(us) == 0 {
					return fmt.Errorf("no accounts yet. Starting the server once (knockbox serve) creates an admin, or create one now: knockbox user add <name>")
				}
				if len(us) > 1 {
					return fmt.Errorf("there are %d accounts; use --user to say which one this device pairs to (knockbox user list shows them)", len(us))
				}
				userID = us[0].Id
			}

			host := app.Conf.GetString("server.external_url")
			if ttl <= 0 {
				ttl = time.Duration(app.Conf.GetInt("server.pair_ttl")) * time.Second
			}
			p, err := service.NewPair(d).Issue(userID, host, "cli", ttl)
			if err != nil {
				return err
			}

			fmt.Println()
			qrterminal.GenerateHalfBlock(p.DeepLink, qrterminal.L, os.Stdout)
			fmt.Printf("\n  Server     %s\n", host)
			fmt.Printf("  Code       %s\n", p.Display)
			fmt.Printf("  Valid to   %s (%s)\n",
				p.ExpiresAt.Format("15:04:05"), time.Until(p.ExpiresAt).Round(time.Second))
			fmt.Printf("\n  Scan the code above with Knockbox. If scanning is not possible, enter the\n  server address and the code in the app by hand.\n")
			fmt.Printf("  The code works once.\n\n")
			return nil
		},
	}
	c.Flags().StringVar(&username, "user", "", "which account this device pairs to (optional when there is only one)")
	c.Flags().DurationVar(&ttl, "ttl", 0, "how long the code stays valid; empty uses server.pair_ttl from the config")
	return c
}
