package cli

import (
	"fmt"
	"strconv"

	"github.com/charmbracelet/huh"
	"github.com/ganjar/ecorouter/internal/config"
	"github.com/ganjar/ecorouter/internal/output"
	"github.com/ganjar/ecorouter/internal/tui"
	"github.com/spf13/cobra"
)

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show or set configuration values",
		Long:  `Show or set EcoRouter configuration values.`,
	}
	cmd.AddCommand(newConfigShowCmd(), newConfigSetCmd())
	return cmd
}

func newConfigShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Print effective config (secrets redacted)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := requireConfig()
			if err != nil {
				return err
			}
			view := map[string]any{
				"path":     cfg.Path(),
				"data_dir": config.DataDir(),
				"server":   cfg.Server,
				"security": cfg.Security,
				"access":   cfg.Access,
				"defaults": cfg.Defaults,
				"health":   cfg.Health,
				"providers": func() map[string]any {
					m := map[string]any{}
					for k, v := range cfg.Providers {
						m[k] = map[string]any{"type": v.Type, "base_url": v.BaseURL, "models": len(v.Models)}
					}
					return m
				}(),
				"routes": cfg.Routes,
				"savers": cfg.Savers,
			}
			if output.JSON {
				return output.PrintJSON(view)
			}
			output.Info("Config:  " + cfg.Path())
			output.Info("Data:    " + config.DataDir())
			output.Info(fmt.Sprintf("Listen:  %s:%d", cfg.Server.Host, cfg.Server.Port))
			output.Info("Domain:  " + emptyDash(cfg.Server.Domain))
			output.Info(fmt.Sprintf("Global rate: %s  daily cap: %v", cfg.Security.GlobalRate, cfg.Security.GlobalDailyCap))
			output.Info("Active route: " + emptyDash(cfg.Defaults.ActiveRoute))
			output.Info(fmt.Sprintf("Providers: %d  Routes: %d  Savers: %d", len(cfg.Providers), len(cfg.Routes), len(cfg.Savers)))
			return nil
		},
	}
}

// setableKeys lists the config keys exposed to `eco config set` along with
// their types and how to validate/parse user-entered values.
type setableKey struct {
	name    string
	label   string
	desc    string
	current func(*config.Config) string
	parse   func(string) (any, error)
	apply   func(*config.Config, any)
}

func setableKeys() []setableKey {
	return []setableKey{
		{
			name:  "domain",
			label: "🌐  domain  (public hostname; leave empty for local-only)",
			desc:  "FQDN that points at this host. Caddy will terminate TLS for it.",
			current: func(c *config.Config) string { return c.Server.Domain },
			parse: func(s string) (any, error) { return s, nil },
			apply:  func(c *config.Config, v any) { c.Server.Domain = v.(string) },
		},
		{
			name:  "port",
			label: "🔌  port  (loopback port; default 8080)",
			desc:  "Daemon binds this port on 127.0.0.1.",
			current: func(c *config.Config) string { return strconv.Itoa(c.Server.Port) },
			parse: func(s string) (any, error) {
				p, err := strconv.Atoi(s)
				if err != nil || p <= 0 {
					return nil, fmt.Errorf("invalid port %q", s)
				}
				return p, nil
			},
			apply: func(c *config.Config, v any) { c.Server.Port = v.(int) },
		},
		{
			name:  "global_rate",
			label: "🚦  global_rate  (e.g. 120/min)",
			desc:  "System-wide request rate cap.",
			current: func(c *config.Config) string { return c.Security.GlobalRate },
			parse: func(s string) (any, error) {
				if _, _, err := config.ParseRate(s); err != nil {
					return nil, err
				}
				return s, nil
			},
			apply: func(c *config.Config, v any) { c.Security.GlobalRate = v.(string) },
		},
		{
			name:  "global_daily_cap",
			label: "💰  global_daily_cap  (USD)",
			desc:  "System-wide daily spend cap in USD.",
			current: func(c *config.Config) string { return fmt.Sprintf("%v", c.Security.GlobalDailyCap) },
			parse: func(s string) (any, error) {
				f, err := strconv.ParseFloat(s, 64)
				if err != nil {
					return nil, fmt.Errorf("invalid cap %q", s)
				}
				return f, nil
			},
			apply: func(c *config.Config, v any) { c.Security.GlobalDailyCap = v.(float64) },
		},
		{
			name:  "timeout_ms",
			label: "⏱   timeout_ms  (HTTP request timeout)",
			desc:  "Upstream request timeout in milliseconds.",
			current: func(c *config.Config) string { return strconv.Itoa(c.Server.TimeoutMs) },
			parse: func(s string) (any, error) {
				n, err := strconv.Atoi(s)
				if err != nil || n <= 0 {
					return nil, fmt.Errorf("invalid timeout %q", s)
				}
				return n, nil
			},
			apply: func(c *config.Config, v any) { c.Server.TimeoutMs = v.(int) },
		},
	}
}

func newConfigSetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set [key] [value]",
		Short: "Set a config key (domain|port|global_rate|global_daily_cap|timeout_ms)",
		Long: `Set a configuration value.

💡 Run with no arguments to pick a key and be prompted for the value.`,
		Args: cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := requireConfig()
			if err != nil {
				return err
			}
			force := WizardRequested()
			keyName := ""
			val := ""
			if len(args) >= 1 {
				keyName = args[0]
			}
			if len(args) >= 2 {
				val = args[1]
			}

			// Build key choices
			keys := setableKeys()
			keyOpts := make([]huh.Option[string], 0, len(keys))
			for _, k := range keys {
				keyOpts = append(keyOpts, huh.NewOption(k.label, k.name))
			}

			keyName, err = askChoice(keyName, "key",
				"Which key to set?", "", keyOpts, force)
			if err != nil {
				return err
			}

			// Find key
			var k *setableKey
			for i := range keys {
				if keys[i].name == keyName {
					k = &keys[i]
					break
				}
			}
			if k == nil {
				return exitErr(2, fmt.Errorf("unknown key %q (domain|port|global_rate|global_daily_cap|timeout_ms)", keyName))
			}

			// prompt for value with current as default
			if val == "" {
				val = k.current(cfg)
			}
			val, err = askString(val, "value",
				fmt.Sprintf("New value for %s", k.name),
				k.desc, k.current(cfg), force,
				func(s string) error {
					_, err := k.parse(s)
					return err
				})
			if err != nil {
				return err
			}

			parsed, err := k.parse(val)
			if err != nil {
				return exitErr(2, err)
			}
			k.apply(cfg, parsed)
			if err := cfg.Save(); err != nil {
				return err
			}
			if output.JSON {
				_ = output.PrintJSON(map[string]string{"set": keyName, "value": val})
				tui.PrintEquivalent("eco config set", []string{keyName, val})
				return nil
			}
			output.Success(fmt.Sprintf("Set %s = %s (restart daemon if running: eco restart)", keyName, val))
			tui.PrintEquivalent("eco config set", []string{keyName, val})
			return nil
		},
	}
	return cmd
}
