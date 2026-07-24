package cli

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/ganjar/ecorouter/internal/config"
	"github.com/ganjar/ecorouter/internal/health"
	"github.com/ganjar/ecorouter/internal/output"
	"github.com/ganjar/ecorouter/internal/router"
	"github.com/ganjar/ecorouter/internal/tui"
	"github.com/spf13/cobra"
)

func newRouteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "route",
		Short: "Manage routes (single / fallback / round)",
		Long:  `Manage routing rules. A route picks a model for a request — single, fallback (try in order), or round-robin.`,
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
		Use:   "add [name]",
		Short: "Add a route (--single | --fallback | --round)",
		Long: `Add a route. A route tells EcoRouter which model to use for incoming requests.

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

			// Resolve mode from whichever flag was passed
			modeIn := single + fallback + round // any non-empty count
			mode := ""
			if single != "" {
				mode = "single"
			} else if fallback != "" {
				mode = "fallback"
			} else if round != "" {
				mode = "round"
			}

			// 1. name
			name, err = askString(name, "name",
				"Route name",
				"Short. This is how you'll refer to it in tokens and clients.",
				"e.g. default, coding, cheap-chat", force,
				func(s string) error {
					s = strings.TrimSpace(s)
					if s == "" {
						return fmt.Errorf("name required")
					}
					if _, exists := cfg.Routes[s]; exists {
						return fmt.Errorf("route %q already exists", s)
					}
					return nil
				})
			if err != nil {
				return err
			}
			name = strings.TrimSpace(name)

			// 2. mode
			mode, err = askChoice(mode, "mode",
				"How should this route pick a model?",
				"Choose the selection strategy.",
				[]huh.Option[string]{
					huh.NewOption("🎯  Single       — always use one model", "single"),
					huh.NewOption("🔁  Fallback    — try 1, then 2, then 3 on failure", "fallback"),
					huh.NewOption("🔄  Round-robin — rotate evenly across models", "round"),
				}, force)
			if err != nil {
				return err
			}

			// 3. models — derive from flags if present, otherwise prompt
			var models []string
			switch mode {
			case "single":
				models = []string{single}
			case "fallback":
				models = splitModels(fallback)
			case "round":
				models = splitModels(round)
			}
			if !force && modeIn == "" && len(models) == 0 && !tui.IsInteractive() {
				return exitErr(2, fmt.Errorf("missing --%s (required in non-interactive mode)", mode))
			}
			if len(models) == 0 {
				opts := modelOptions(cfg)
				if len(opts) == 0 {
					return exitErr(2, fmt.Errorf("no models available — add a provider first"))
				}
				if mode == "single" {
					m, err := askPick("", "model", "Pick the model", "", opts, force)
					if err != nil {
						return err
					}
					models = []string{m}
				} else {
					var chosen []string
					if err := tui.MultiSelect(
						fmt.Sprintf("Pick models for %s route", mode),
						"Space to toggle. Order = fallback/rotation order.",
						opts, &chosen); err != nil {
						return err
					}
					if len(chosen) < 1 {
						return exitErr(2, fmt.Errorf("at least one model required"))
					}
					models = chosen
				}
			}

			// 4. optional saver hop
			saverOpts := []huh.Option[string]{huh.NewOption("🚫  No saver — straight to provider", "")}
			for _, s := range saverOptions(cfg) {
				saverOpts = append(saverOpts, s)
			}
			if via == "" && !noVia && tui.IsInteractive() && (force || modeIn == "") {
				var picked string
				if err := tui.SelectString(
					"Route through a token saver?",
					"Savers compress requests before they hit the provider.",
					saverOpts, &picked); err == nil {
					via = picked
				}
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
				_ = output.PrintJSON(map[string]any{"name": name, "mode": mode, "models": models, "via": via})
				flag := "--" + mode + " " + strings.Join(models, ",")
				equiv := []string{name, flag}
				if via != "" {
					equiv = append(equiv, "--via "+via)
				}
				tui.PrintEquivalent("eco route add", equiv)
				return nil
			}
			output.Success(fmt.Sprintf("Route %q created (%s: %s).", name, mode, strings.Join(models, ", ")))
			if cfg.Defaults.ActiveRoute == name {
				output.Info("  Set as active route.")
			}
			flag := "--" + mode + " " + strings.Join(models, ",")
			equiv := []string{name, flag}
			if via != "" {
				equiv = append(equiv, "--via "+via)
			}
			tui.PrintEquivalent("eco route add", equiv)
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
	cmd := &cobra.Command{
		Use:   "show [name]",
		Short: "Show full route detail",
		Long: `Show full route detail.

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
			name, err = askPick(name, "name", "Show which route?", "", routeOptions(cfg), force)
			if err != nil {
				return err
			}
			if name == "" {
				return exitErr(1, fmt.Errorf("no route selected"))
			}
			r, ok := cfg.Routes[name]
			if !ok {
				return exitErr(1, fmt.Errorf("route %q not found", name))
			}
			detail := map[string]any{
				"name":         name,
				"mode":         r.Mode,
				"models":       r.Models,
				"via":          r.Via,
				"no_via":       r.NoVia,
				"via_required": r.ViaRequired,
				"active":       name == cfg.Defaults.ActiveRoute,
			}
			if output.JSON {
				return output.PrintJSON(detail)
			}
			output.Info(fmt.Sprintf("Route: %s", name))
			output.Info(fmt.Sprintf("  Mode:         %s", r.Mode))
			output.Info(fmt.Sprintf("  Models:       %s", strings.Join(r.Models, ", ")))
			output.Info(fmt.Sprintf("  Via:          %s", emptyDash(r.Via)))
			output.Info(fmt.Sprintf("  No-via:       %v", r.NoVia))
			output.Info(fmt.Sprintf("  Via-required: %v", r.ViaRequired))
			output.Info(fmt.Sprintf("  Active:       %v", name == cfg.Defaults.ActiveRoute))
			return nil
		},
	}
	return cmd
}

func newRouteRemoveCmd() *cobra.Command {
	var assumeYes bool
	cmd := &cobra.Command{
		Use:   "remove [name]",
		Short: "Delete a route",
		Long: `Delete a route. Tokens scoped to this route will fail until re-scoped.

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
			name, err = askPick(name, "name", "Remove which route?",
				"Tokens scoped to this route will fail until re-scoped.",
				routeOptions(cfg), force)
			if err != nil {
				return err
			}
			if name == "" {
				return exitErr(1, fmt.Errorf("no route selected"))
			}
			if _, ok := cfg.Routes[name]; !ok {
				return exitErr(1, fmt.Errorf("route %q not found", name))
			}
			ok, err := confirmDestructive(assumeYes,
				"Remove route "+name+"?",
				"Tokens scoped to this route will fail until re-scoped.")
			if err != nil {
				return err
			}
			if !ok {
				return nil
			}
			delete(cfg.Routes, name)
			if cfg.Defaults.ActiveRoute == name {
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
				return output.PrintJSON(map[string]string{"removed": name})
			}
			output.Success(fmt.Sprintf("Route %q removed.", name))
			tui.PrintEquivalent("eco route remove", []string{name, "--yes"})
			return nil
		},
	}
	cmd.Flags().BoolVarP(&assumeYes, "yes", "y", false, "skip confirmation prompt")
	return cmd
}

func newRouteTestCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "test [name]",
		Short: "Dry-run: which model would be selected now, and why",
		Long: `Dry-run a route. Shows which model would be selected right now and why.

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
			name, err = askPick(name, "name", "Test which route?",
				"Dry-run selection.", routeOptions(cfg), force)
			if err != nil {
				return err
			}
			if name == "" {
				return exitErr(1, fmt.Errorf("no route selected"))
			}
			if _, ok := cfg.Routes[name]; !ok {
				return exitErr(1, fmt.Errorf("route %q not found", name))
			}
			h := health.New(cfg.Health.Window, cfg.Health.ErrorThreshold, cfg.Health.MinRequests, cfg.Health.CooldownMs)
			eng := router.New(h)
			d, err := eng.Resolve(cfg, name)
			if err != nil {
				return err
			}
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
	return cmd
}

func newUseCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "use [route]",
		Short: "Set the active/default route",
		Long: `Set the active route. The active route is used by default when a token
has no explicit route scope.

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
			name, err = askPick(name, "route", "Set which route as active?", "",
				routeOptions(cfg), force)
			if err != nil {
				return err
			}
			if name == "" {
				return exitErr(1, fmt.Errorf("no route selected"))
			}
			if _, ok := cfg.Routes[name]; !ok {
				return exitErr(1, fmt.Errorf("route %q not found", name))
			}
			cfg.Defaults.ActiveRoute = name
			if err := cfg.Save(); err != nil {
				return err
			}
			if output.JSON {
				return output.PrintJSON(map[string]string{"active_route": name})
			}
			output.Success(fmt.Sprintf("Active route set to %q.", name))
			tui.PrintEquivalent("eco use", []string{name})
			return nil
		},
	}
	return cmd
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
