package cli

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/ganjar/ecorouter/internal/config"
	"github.com/ganjar/ecorouter/internal/output"
	"github.com/ganjar/ecorouter/internal/tui"
)

func runRouteMenu() error {
	for {
		var choice string
		if err := tui.SelectString("🛣️  Routes", "Add and manage routing rules.",
			[]huh.Option[string]{
				huh.NewOption("➕  Add a route", "add"),
				huh.NewOption("📋  List routes", "list"),
				huh.NewOption("🧪  Test a route (dry-run selection)", "test"),
				huh.NewOption("⭐  Set the active route", "use"),
				huh.NewOption("✏️   Edit a route", "edit"),
				huh.NewOption("🗑️   Remove a route", "remove"),
				huh.NewOption("↩️   Back", "back"),
			}, &choice); err != nil {
			return err
		}

		switch choice {
		case "add":
			_ = runRouteAddWizard()
		case "list":
			if cfg, err := requireConfig(); err == nil {
				routeListPrint(cfg)
			}
		case "test":
			_ = runRouteTestWizard()
		case "use":
			_ = runRouteUseWizard()
		case "edit":
			_ = runRouteEditWizard()
		case "remove":
			_ = runRouteRemoveWizard()
		case "back", "":
			return nil
		}
	}
}

func runRouteAddWizard() error {
	if err := tui.RequireTTY(); err != nil {
		return err
	}
	cfg, err := requireConfig()
	if err != nil {
		return err
	}

	if len(cfg.Providers) == 0 {
		output.Warn("No providers configured yet. Add one first.")
		return nil
	}

	// Step 1 — name + mode
	var name, mode string
	err = huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("Route name").
			Description("Short. This is how you'll refer to it in tokens and clients.").
			Placeholder("default, coding, cheap-chat…").
			Value(&name).
			Validate(func(s string) error {
				s = strings.TrimSpace(s)
				if s == "" {
					return fmt.Errorf("name is required")
				}
				if _, exists := cfg.Routes[s]; exists {
					return fmt.Errorf("route %q already exists", s)
				}
				return nil
			}),
		huh.NewSelect[string]().
			Title("How should this route pick a model?").
			Options(
				huh.NewOption("🎯  Single       — always use one model", "single"),
				huh.NewOption("🔁  Fallback    — try model 1, then 2, then 3 on failure", "fallback"),
				huh.NewOption("🔄  Round-robin — rotate evenly across models", "round"),
			).
			Value(&mode),
	)).WithTheme(tui.Theme()).Run()
	if err != nil {
		return err
	}
	name = strings.TrimSpace(name)

	// Step 2 — pick models
	all := allProviderModels(cfg)
	if len(all) == 0 {
		output.Warn("No models available. Add models to your provider first.")
		return nil
	}
	var chosen []string
	opts := make([]huh.Option[string], 0, len(all))
	for _, m := range all {
		opts = append(opts, huh.NewOption(m, m))
	}
	if mode == "single" {
		var one string
		if err := tui.SelectString("Pick the model", "", opts, &one); err != nil {
			return err
		}
		chosen = []string{one}
	} else {
		desc := "Space to toggle. Order = fallback/rotation order."
		if err := tui.MultiSelect(
			fmt.Sprintf("Pick models for %s route", mode),
			desc, opts, &chosen); err != nil {
			return err
		}
		if len(chosen) < 1 {
			output.Warn("Need at least one model. Aborted.")
			return nil
		}
	}

	// Step 3 — optional saver hop
	via := ""
	if len(cfg.Savers) > 0 {
		var choice string
		saverOpts := []huh.Option[string]{
			huh.NewOption("🚫  No saver — straight to provider", ""),
		}
		for sName := range cfg.Savers {
			saverOpts = append(saverOpts, huh.NewOption("💾  "+sName, sName))
		}
		if err := tui.SelectString(
			"Route through a token saver?",
			"Savers compress requests before they hit the provider.",
			saverOpts, &choice); err != nil {
			return err
		}
		via = choice
	}

	rc := config.RouteConfig{
		Mode:   mode,
		Models: chosen,
		Via:    via,
	}
	cfg.Routes[name] = rc
	if cfg.Defaults.ActiveRoute == "" {
		cfg.Defaults.ActiveRoute = name
	}
	if err := cfg.Save(); err != nil {
		return err
	}

	output.Success(fmt.Sprintf("Route %q created (%s: %s).", name, mode, strings.Join(chosen, ", ")))

	flag := "--" + mode + " " + strings.Join(chosen, ",")
	argsList := []string{name, flag}
	if via != "" {
		argsList = append(argsList, "--via "+via)
	}
	tui.PrintEquivalent("eco route add", argsList)
	return nil
}

// allProviderModels flattens every provider's catalog into "provider/model" IDs.
func allProviderModels(cfg *config.Config) []string {
	var out []string
	for pName, p := range cfg.Providers {
		for _, m := range p.Models {
			out = append(out, pName+"/"+m)
		}
	}
	return out
}

func pickRoute(cfg *config.Config, title string) (string, error) {
	if len(cfg.Routes) == 0 {
		output.Info("No routes yet. Add one first.")
		return "", nil
	}
	var name string
	opts := make([]huh.Option[string], 0, len(cfg.Routes))
	for n, r := range cfg.Routes {
		label := fmt.Sprintf("%s  (%s: %s)", n, r.Mode, strings.Join(r.Models, ", "))
		if n == cfg.Defaults.ActiveRoute {
			label += "  ★"
		}
		opts = append(opts, huh.NewOption(label, n))
	}
	err := tui.SelectString(title, "", opts, &name)
	return name, err
}

func runRouteTestWizard() error {
	if err := tui.RequireTTY(); err != nil {
		return err
	}
	cfg, err := requireConfig()
	if err != nil {
		return err
	}
	name, err := pickRoute(cfg, "Test which route?")
	if err != nil || name == "" {
		return err
	}
	return newRouteTestCmd().RunE(newRouteTestCmd(), []string{name})
}

func runRouteUseWizard() error {
	if err := tui.RequireTTY(); err != nil {
		return err
	}
	cfg, err := requireConfig()
	if err != nil {
		return err
	}
	name, err := pickRoute(cfg, "Set which route as active?")
	if err != nil || name == "" {
		return err
	}
	cfg.Defaults.ActiveRoute = name
	if err := cfg.Save(); err != nil {
		return err
	}
	output.Success(fmt.Sprintf("Active route set to %q.", name))
	tui.PrintEquivalent("eco use", []string{name})
	return nil
}

func runRouteEditWizard() error {
	if err := tui.RequireTTY(); err != nil {
		return err
	}
	cfg, err := requireConfig()
	if err != nil {
		return err
	}
	name, err := pickRoute(cfg, "Edit which route?")
	if err != nil || name == "" {
		return err
	}
	r := cfg.Routes[name]

	var action string
	if err := tui.SelectString("Edit route "+name, "", []huh.Option[string]{
		huh.NewOption("🎯  Change models", "models"),
		huh.NewOption("🔁  Change mode", "mode"),
		huh.NewOption("💾  Change saver hop", "via"),
		huh.NewOption("↩️   Back", "back"),
	}, &action); err != nil {
		return err
	}

	switch action {
	case "models":
		all := allProviderModels(cfg)
		if len(all) == 0 {
			output.Warn("No models available.")
			return nil
		}
		opts := make([]huh.Option[string], 0, len(all))
		for _, m := range all {
			opts = append(opts, huh.NewOption(m, m))
		}
		chosen := append([]string{}, r.Models...)
		if r.Mode == "single" {
			var one string
			if len(chosen) > 0 {
				one = chosen[0]
			}
			if err := tui.SelectString("Pick the model", "", opts, &one); err != nil {
				return err
			}
			r.Models = []string{one}
		} else {
			if err := tui.MultiSelect("Pick models", "Space to toggle.", opts, &chosen); err != nil {
				return err
			}
			if len(chosen) < 1 {
				output.Warn("Need at least one model.")
				return nil
			}
			r.Models = chosen
		}
	case "mode":
		var mode string
		if err := tui.SelectString("Mode", "", []huh.Option[string]{
			huh.NewOption("Single", "single"),
			huh.NewOption("Fallback", "fallback"),
			huh.NewOption("Round-robin", "round"),
		}, &mode); err != nil {
			return err
		}
		r.Mode = mode
		if mode == "single" && len(r.Models) > 1 {
			r.Models = r.Models[:1]
		}
	case "via":
		saverOpts := []huh.Option[string]{
			huh.NewOption("🚫  No saver", ""),
		}
		for sName := range cfg.Savers {
			saverOpts = append(saverOpts, huh.NewOption("💾  "+sName, sName))
		}
		var via string
		if err := tui.SelectString("Saver hop", "", saverOpts, &via); err != nil {
			return err
		}
		r.Via = via
		r.NoVia = via == ""
	case "back", "":
		return nil
	}
	cfg.Routes[name] = r
	if err := cfg.Save(); err != nil {
		return err
	}
	output.Success(fmt.Sprintf("Route %q updated.", name))
	return nil
}

func runRouteRemoveWizard() error {
	if err := tui.RequireTTY(); err != nil {
		return err
	}
	cfg, err := requireConfig()
	if err != nil {
		return err
	}
	name, err := pickRoute(cfg, "Remove which route?")
	if err != nil || name == "" {
		return err
	}
	ok, _ := tui.Confirm("Remove route "+name+"?", "Tokens scoped to this route will fail until re-scoped.", false)
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
	output.Success(fmt.Sprintf("Route %q removed.", name))
	tui.PrintEquivalent("eco route remove", []string{name})
	return nil
}

func routeListPrint(cfg *config.Config) {
	var table [][]string
	for name, r := range cfg.Routes {
		via := r.Via
		if via == "" && !r.NoVia {
			via = cfg.Defaults.SaverDefault
		}
		if r.NoVia {
			via = "(none)"
		}
		mark := " "
		if name == cfg.Defaults.ActiveRoute {
			mark = "*"
		}
		table = append(table, []string{mark, name, r.Mode, strings.Join(r.Models, ","), via})
	}
	if len(table) == 0 {
		output.Info("No routes. Create one from the menu or: eco route add default --single provider/model")
		return
	}
	output.Table([]string{"", "NAME", "MODE", "MODELS", "VIA"}, table)
}
