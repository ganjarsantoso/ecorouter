package cli

import (
	"fmt"
	"strings"

	"github.com/ganjar/ecorouter/internal/config"
	"github.com/ganjar/ecorouter/internal/health"
	"github.com/ganjar/ecorouter/internal/output"
	"github.com/ganjar/ecorouter/internal/router"
	"github.com/spf13/cobra"
)

func newRouteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "route",
		Short: "Manage routes (single / fallback / round)",
	}
	cmd.AddCommand(
		newRouteAddCmd(),
		newRouteListCmd(),
		newRouteShowCmd(),
		newRouteRemoveCmd(),
		newRouteTestCmd(),
	)
	return cmd
}

func newRouteAddCmd() *cobra.Command {
	var single, fallback, round, via string
	var noVia, viaRequired bool
	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Add a route (--single | --fallback | --round)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			cfg, err := requireConfig()
			if err != nil {
				return err
			}
			modes := 0
			mode := ""
			var models []string
			if single != "" {
				modes++
				mode = "single"
				models = []string{single}
			}
			if fallback != "" {
				modes++
				mode = "fallback"
				models = splitModels(fallback)
			}
			if round != "" {
				modes++
				mode = "round"
				models = splitModels(round)
			}
			if modes != 1 {
				return exitErr(2, fmt.Errorf("exactly one of --single, --fallback, --round is required"))
			}
			if len(models) == 0 {
				return exitErr(2, fmt.Errorf("at least one model required"))
			}
			if via != "" && noVia {
				return exitErr(2, fmt.Errorf("--via and --no-via are mutually exclusive"))
			}
			if _, exists := cfg.Routes[name]; exists {
				return exitErr(1, fmt.Errorf("route %q already exists", name))
			}
			rc := config.RouteConfig{
				Mode:        mode,
				Models:      models,
				Via:         via,
				NoVia:       noVia,
				ViaRequired: viaRequired,
			}
			cfg.Routes[name] = rc
			if cfg.Defaults.ActiveRoute == "" {
				cfg.Defaults.ActiveRoute = name
			}
			if err := cfg.Save(); err != nil {
				return err
			}
			if output.JSON {
				return output.PrintJSON(map[string]any{"name": name, "mode": mode, "models": models, "via": via})
			}
			output.Success(fmt.Sprintf("Route %q created (%s: %s).", name, mode, strings.Join(models, ", ")))
			if cfg.Defaults.ActiveRoute == name {
				output.Info("  Set as active route.")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&single, "single", "", "single model")
	cmd.Flags().StringVar(&fallback, "fallback", "", "comma-separated fallback models (ordered)")
	cmd.Flags().StringVar(&round, "round", "", "comma-separated round-robin models")
	cmd.Flags().StringVar(&via, "via", "", "route through named saver")
	cmd.Flags().BoolVar(&noVia, "no-via", false, "bypass global default saver")
	cmd.Flags().BoolVar(&viaRequired, "via-required", false, "fail if saver unreachable (default: bypass)")
	return cmd
}

func newRouteListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List routes",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := requireConfig()
			if err != nil {
				return err
			}
			type row struct {
				Name   string   `json:"name"`
				Mode   string   `json:"mode"`
				Models []string `json:"models"`
				Via    string   `json:"via,omitempty"`
				Active bool     `json:"active"`
			}
			var rows []row
			var table [][]string
			for name, r := range cfg.Routes {
				via := r.Via
				if via == "" && !r.NoVia {
					via = cfg.Defaults.SaverDefault
				}
				if r.NoVia {
					via = "(none)"
				}
				active := name == cfg.Defaults.ActiveRoute
				rows = append(rows, row{name, r.Mode, r.Models, via, active})
				mark := " "
				if active {
					mark = "*"
				}
				table = append(table, []string{mark, name, r.Mode, strings.Join(r.Models, ","), via})
			}
			if output.JSON {
				return output.PrintJSON(rows)
			}
			if len(table) == 0 {
				output.Info("No routes. Create one: eco route add default --fallback gpt-4o,gpt-4o-mini")
				return nil
			}
			output.Table([]string{"", "NAME", "MODE", "MODELS", "VIA"}, table)
			return nil
		},
	}
}

func newRouteShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <name>",
		Short: "Show full route detail",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := requireConfig()
			if err != nil {
				return err
			}
			r, ok := cfg.Routes[args[0]]
			if !ok {
				return exitErr(1, fmt.Errorf("route %q not found", args[0]))
			}
			detail := map[string]any{
				"name":         args[0],
				"mode":         r.Mode,
				"models":       r.Models,
				"via":          r.Via,
				"no_via":       r.NoVia,
				"via_required": r.ViaRequired,
				"active":       args[0] == cfg.Defaults.ActiveRoute,
			}
			if output.JSON {
				return output.PrintJSON(detail)
			}
			output.Info(fmt.Sprintf("Route: %s", args[0]))
			output.Info(fmt.Sprintf("  Mode:         %s", r.Mode))
			output.Info(fmt.Sprintf("  Models:       %s", strings.Join(r.Models, ", ")))
			output.Info(fmt.Sprintf("  Via:          %s", emptyDash(r.Via)))
			output.Info(fmt.Sprintf("  No-via:       %v", r.NoVia))
			output.Info(fmt.Sprintf("  Via-required: %v", r.ViaRequired))
			output.Info(fmt.Sprintf("  Active:       %v", args[0] == cfg.Defaults.ActiveRoute))
			return nil
		},
	}
}

func newRouteRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Delete a route",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := requireConfig()
			if err != nil {
				return err
			}
			if _, ok := cfg.Routes[args[0]]; !ok {
				return exitErr(1, fmt.Errorf("route %q not found", args[0]))
			}
			delete(cfg.Routes, args[0])
			if cfg.Defaults.ActiveRoute == args[0] {
				cfg.Defaults.ActiveRoute = ""
				for n := range cfg.Routes {
					cfg.Defaults.ActiveRoute = n
					break
				}
			}
			if err := cfg.Save(); err != nil {
				return err
			}
			if output.JSON {
				return output.PrintJSON(map[string]string{"removed": args[0]})
			}
			output.Success(fmt.Sprintf("Route %q removed.", args[0]))
			return nil
		},
	}
}

func newRouteTestCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "test <name>",
		Short: "Dry-run: which model would be selected now, and why",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := requireConfig()
			if err != nil {
				return err
			}
			if _, ok := cfg.Routes[args[0]]; !ok {
				return exitErr(1, fmt.Errorf("route %q not found", args[0]))
			}
			h := health.New(cfg.Health.Window, cfg.Health.ErrorThreshold, cfg.Health.MinRequests, cfg.Health.CooldownMs)
			eng := router.New(h)
			// for round, peek without advancing permanently — Resolve advances; for test we document next
			d, err := eng.Resolve(cfg, args[0])
			if err != nil {
				return err
			}
			// note: Resolve advances round counter in process memory only (not daemon)
			if output.JSON {
				return output.PrintJSON(d)
			}
			output.Info(fmt.Sprintf("Route:    %s (%s)", d.Route, d.Mode))
			output.Info(fmt.Sprintf("Selected: %s", output.Cyan(d.Selected)))
			output.Info(fmt.Sprintf("Order:    %s", strings.Join(d.Models, " → ")))
			if d.Via != "" {
				output.Info(fmt.Sprintf("Via:      %s (required=%v)", d.Via, d.ViaReq))
			}
			output.Info(fmt.Sprintf("Reason:   %s", d.Reason))
			return nil
		},
	}
}

func newUseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "use <route>",
		Short: "Set the active/default route",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := requireConfig()
			if err != nil {
				return err
			}
			if _, ok := cfg.Routes[args[0]]; !ok {
				return exitErr(1, fmt.Errorf("route %q not found", args[0]))
			}
			cfg.Defaults.ActiveRoute = args[0]
			if err := cfg.Save(); err != nil {
				return err
			}
			if output.JSON {
				return output.PrintJSON(map[string]string{"active_route": args[0]})
			}
			output.Success(fmt.Sprintf("Active route set to %q.", args[0]))
			return nil
		},
	}
}

func splitModels(s string) []string {
	parts := strings.Split(s, ",")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func emptyDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
