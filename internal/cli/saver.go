package cli

import (
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/ganjar/ecorouter/internal/config"
	"github.com/ganjar/ecorouter/internal/output"
	"github.com/ganjar/ecorouter/internal/tui"
	"github.com/spf13/cobra"
)

func newSaverCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "saver",
		Short: "Manage external token-saving proxies",
		Long:  `Manage external token-saving proxies (savers compress prompts before they hit the provider).`,
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
		Use:   "add [name]",
		Short: "Register an external saver proxy",
		Long: `Register an external saver proxy.

💡 Run with no arguments (or --wizard) to be guided step-by-step.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := requireConfig()
			if err != nil {
				return err
			}
			force := WizardRequested()
			name := ""
			if len(args) == 1 {
				name = args[0]
			}

			name, err = askString(name, "name",
				"Saver name",
				"Any short name. No hardcoded presets.",
				"e.g. headroom, caveman, local-saver", force,
				func(s string) error {
					s = strings.TrimSpace(s)
					if s == "" {
						return fmt.Errorf("name required")
					}
					if _, exists := cfg.Savers[s]; exists {
						return fmt.Errorf("saver %q already exists", s)
					}
					return nil
				})
			if err != nil {
				return err
			}
			name = strings.TrimSpace(name)

			u, err = askString(u, "url",
				"Saver base URL",
				"Where the saver proxy listens. Copy from its docs.",
				"http://127.0.0.1:8787", force,
				func(s string) error {
					s = strings.TrimSpace(s)
					if s == "" {
						return fmt.Errorf("URL required")
					}
					if _, err := url.ParseRequestURI(s); err != nil {
						return fmt.Errorf("invalid url: %w", err)
					}
					return nil
				})
			if err != nil {
				return err
			}
			u = strings.TrimSpace(u)

			cfg.Savers[name] = config.SaverConfig{URL: u}
			if err := cfg.Save(); err != nil {
				return err
			}
			if output.JSON {
				_ = output.PrintJSON(map[string]string{"name": name, "url": u})
				tui.PrintEquivalent("eco saver add", []string{name, "--url " + u})
				return nil
			}
			output.Success(fmt.Sprintf("Saver %q registered at %s.", name, u))
			tui.PrintEquivalent("eco saver add", []string{name, "--url " + u})
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
	cmd := &cobra.Command{
		Use:   "test [name]",
		Short: "Round-trip reachability check",
		Long: `Round-trip reachability check for a saver.

💡 Run with no arguments to pick from a list.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := requireConfig()
			if err != nil {
				return err
			}
			force := WizardRequested()
			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			name, err = askPick(name, "name", "Test which saver?",
				"Round-trip reachability check.", saverOptions(cfg), force)
			if err != nil {
				return err
			}
			if name == "" {
				return exitErr(1, fmt.Errorf("no saver selected"))
			}
			s, ok := cfg.Savers[name]
			if !ok {
				return exitErr(1, fmt.Errorf("saver %q not found", name))
			}
			if err := probeTCP(s.URL); err != nil {
				if output.JSON {
					output.FailJSON("saver_unreachable", err.Error(), "start the saver process")
					return exitErr(7, err)
				}
				output.Fail(fmt.Sprintf("Saver %q unreachable at %s.", name, s.URL),
					"start the saver (e.g. headroom proxy --port 8787)")
				return exitErr(7, err)
			}
			if output.JSON {
				return output.PrintJSON(map[string]any{"ok": true, "url": s.URL})
			}
			output.Success(fmt.Sprintf("Saver %q reachable at %s.", name, s.URL))
			return nil
		},
	}
	return cmd
}

func newSaverDefaultCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "default [name]",
		Short: "Set global default saver (routes may --no-via)",
		Long: `Set the global default saver. Routes without their own --via will use it.

💡 Run with no arguments to pick from a list.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := requireConfig()
			if err != nil {
				return err
			}
			force := WizardRequested()
			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			name, err = askPick(name, "name", "Set which saver as default?",
				"Routes without --via will use it.", saverOptions(cfg), force)
			if err != nil {
				return err
			}
			if name == "" {
				return exitErr(1, fmt.Errorf("no saver selected"))
			}
			if _, ok := cfg.Savers[name]; !ok {
				return exitErr(1, fmt.Errorf("saver %q not found", name))
			}
			cfg.Defaults.SaverDefault = name
			if err := cfg.Save(); err != nil {
				return err
			}
			if output.JSON {
				return output.PrintJSON(map[string]string{"saver_default": name})
			}
			output.Success(fmt.Sprintf("Default saver set to %q.", name))
			tui.PrintEquivalent("eco saver default", []string{name})
			return nil
		},
	}
	return cmd
}

func newSaverRemoveCmd() *cobra.Command {
	var assumeYes bool
	cmd := &cobra.Command{
		Use:   "remove [name]",
		Short: "Unregister a saver",
		Long: `Unregister a saver. Routes using --via <name> will need updating.

💡 Run with no arguments to pick from a list.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := requireConfig()
			if err != nil {
				return err
			}
			force := WizardRequested()
			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			name, err = askPick(name, "name", "Remove which saver?",
				"Routes using --via <name> will need updating.",
				saverOptions(cfg), force)
			if err != nil {
				return err
			}
			if name == "" {
				return exitErr(1, fmt.Errorf("no saver selected"))
			}
			if _, ok := cfg.Savers[name]; !ok {
				return exitErr(1, fmt.Errorf("saver %q not found", name))
			}
			ok, err := confirmDestructive(assumeYes,
				"Remove saver "+name+"?",
				"Routes using --via "+name+" will need updating.")
			if err != nil {
				return err
			}
			if !ok {
				return nil
			}
			delete(cfg.Savers, name)
			if cfg.Defaults.SaverDefault == name {
				cfg.Defaults.SaverDefault = ""
			}
			if err := cfg.Save(); err != nil {
				return err
			}
			if output.JSON {
				return output.PrintJSON(map[string]string{"removed": name})
			}
			output.Success(fmt.Sprintf("Saver %q removed.", name))
			tui.PrintEquivalent("eco saver remove", []string{name, "--yes"})
			return nil
		},
	}
	cmd.Flags().BoolVarP(&assumeYes, "yes", "y", false, "skip confirmation prompt")
	return cmd
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
