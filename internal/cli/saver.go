package cli

import (
	"fmt"
	"net"
	"net/url"
	"time"

	"github.com/ganjar/ecorouter/internal/config"
	"github.com/ganjar/ecorouter/internal/output"
	"github.com/spf13/cobra"
)

func newSaverCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "saver",
		Short: "Manage external token-saving proxies",
	}
	cmd.AddCommand(
		newSaverAddCmd(),
		newSaverListCmd(),
		newSaverTestCmd(),
		newSaverDefaultCmd(),
		newSaverRemoveCmd(),
	)
	return cmd
}

func newSaverAddCmd() *cobra.Command {
	var u string
	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Register an external saver proxy",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if u == "" {
				return exitErr(2, fmt.Errorf("--url is required"))
			}
			if _, err := url.ParseRequestURI(u); err != nil {
				return exitErr(2, fmt.Errorf("invalid url: %w", err))
			}
			cfg, err := requireConfig()
			if err != nil {
				return err
			}
			cfg.Savers[args[0]] = config.SaverConfig{URL: u}
			if err := cfg.Save(); err != nil {
				return err
			}
			if output.JSON {
				return output.PrintJSON(map[string]string{"name": args[0], "url": u})
			}
			output.Success(fmt.Sprintf("Saver %q registered at %s.", args[0], u))
			return nil
		},
	}
	cmd.Flags().StringVar(&u, "url", "", "saver base URL (e.g. http://127.0.0.1:8787)")
	return cmd
}

func newSaverListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List savers with reachability",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := requireConfig()
			if err != nil {
				return err
			}
			type row struct {
				Name      string `json:"name"`
				URL       string `json:"url"`
				Reachable bool   `json:"reachable"`
				Default   bool   `json:"default"`
			}
			var rows []row
			var table [][]string
			for name, s := range cfg.Savers {
				ok := probeTCP(s.URL) == nil
				def := name == cfg.Defaults.SaverDefault
				rows = append(rows, row{name, s.URL, ok, def})
				dot := output.HealthDot(ok)
				mark := ""
				if def {
					mark = " (default)"
				}
				table = append(table, []string{dot + " " + name + mark, s.URL})
			}
			if output.JSON {
				return output.PrintJSON(rows)
			}
			if len(table) == 0 {
				output.Info("No savers. Add one: eco saver add headroom --url http://127.0.0.1:8787")
				return nil
			}
			output.Table([]string{"NAME", "URL"}, table)
			return nil
		},
	}
}

func newSaverTestCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "test <name>",
		Short: "Round-trip reachability check",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := requireConfig()
			if err != nil {
				return err
			}
			s, ok := cfg.Savers[args[0]]
			if !ok {
				return exitErr(1, fmt.Errorf("saver %q not found", args[0]))
			}
			if err := probeTCP(s.URL); err != nil {
				if output.JSON {
					output.FailJSON("saver_unreachable", err.Error(), "start the saver process")
					return exitErr(7, err)
				}
				output.Fail(fmt.Sprintf("Saver %q unreachable at %s.", args[0], s.URL),
					"start the saver (e.g. headroom proxy --port 8787)")
				return exitErr(7, err)
			}
			if output.JSON {
				return output.PrintJSON(map[string]any{"ok": true, "url": s.URL})
			}
			output.Success(fmt.Sprintf("Saver %q reachable at %s.", args[0], s.URL))
			return nil
		},
	}
}

func newSaverDefaultCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "default <name>",
		Short: "Set global default saver (routes may --no-via)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := requireConfig()
			if err != nil {
				return err
			}
			if _, ok := cfg.Savers[args[0]]; !ok {
				return exitErr(1, fmt.Errorf("saver %q not found", args[0]))
			}
			cfg.Defaults.SaverDefault = args[0]
			if err := cfg.Save(); err != nil {
				return err
			}
			if output.JSON {
				return output.PrintJSON(map[string]string{"saver_default": args[0]})
			}
			output.Success(fmt.Sprintf("Default saver set to %q.", args[0]))
			return nil
		},
	}
}

func newSaverRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Unregister a saver",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := requireConfig()
			if err != nil {
				return err
			}
			if _, ok := cfg.Savers[args[0]]; !ok {
				return exitErr(1, fmt.Errorf("saver %q not found", args[0]))
			}
			delete(cfg.Savers, args[0])
			if cfg.Defaults.SaverDefault == args[0] {
				cfg.Defaults.SaverDefault = ""
			}
			if err := cfg.Save(); err != nil {
				return err
			}
			if output.JSON {
				return output.PrintJSON(map[string]string{"removed": args[0]})
			}
			output.Success(fmt.Sprintf("Saver %q removed.", args[0]))
			return nil
		},
	}
}

func probeTCP(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	host := u.Host
	if host == "" {
		return fmt.Errorf("empty host")
	}
	if u.Port() == "" {
		port := "80"
		if u.Scheme == "https" {
			port = "443"
		}
		host = net.JoinHostPort(u.Hostname(), port)
	}
	d := net.Dialer{Timeout: 2 * time.Second}
	c, err := d.Dial("tcp", host)
	if err != nil {
		return err
	}
	_ = c.Close()
	return nil
}
