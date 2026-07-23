package cli

import (
	"fmt"

	"github.com/ganjar/ecorouter/internal/config"
	"github.com/ganjar/ecorouter/internal/output"
	"github.com/spf13/cobra"
)

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show or set configuration values",
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

func newConfigSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a config key (domain|port|global_rate|global_daily_cap|timeout_ms)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := requireConfig()
			if err != nil {
				return err
			}
			key, val := args[0], args[1]
			switch key {
			case "domain":
				cfg.Server.Domain = val
			case "port":
				var p int
				if _, err := fmt.Sscanf(val, "%d", &p); err != nil || p <= 0 {
					return exitErr(2, fmt.Errorf("invalid port %q", val))
				}
				cfg.Server.Port = p
			case "global_rate":
				if _, _, err := config.ParseRate(val); err != nil {
					return exitErr(2, err)
				}
				cfg.Security.GlobalRate = val
			case "global_daily_cap":
				var f float64
				if _, err := fmt.Sscanf(val, "%f", &f); err != nil {
					return exitErr(2, fmt.Errorf("invalid cap %q", val))
				}
				cfg.Security.GlobalDailyCap = f
			case "timeout_ms":
				var n int
				if _, err := fmt.Sscanf(val, "%d", &n); err != nil || n <= 0 {
					return exitErr(2, fmt.Errorf("invalid timeout %q", val))
				}
				cfg.Server.TimeoutMs = n
			default:
				return exitErr(2, fmt.Errorf("unknown key %q (domain|port|global_rate|global_daily_cap|timeout_ms)", key))
			}
			if err := cfg.Save(); err != nil {
				return err
			}
			if output.JSON {
				return output.PrintJSON(map[string]string{"set": key, "value": val})
			}
			output.Success(fmt.Sprintf("Set %s = %s (restart daemon if running: eco restart)", key, val))
			return nil
		},
	}
}
