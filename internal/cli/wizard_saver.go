package cli

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/ganjar/ecorouter/internal/config"
	"github.com/ganjar/ecorouter/internal/output"
	"github.com/ganjar/ecorouter/internal/tui"
)

func runSaverMenu() error {
	for {
		var choice string
		if err := tui.SelectString("💾  Savers", "External token-saving proxies.",
			[]huh.Option[string]{
				huh.NewOption("➕  Add a saver", "add"),
				huh.NewOption("📋  List savers", "list"),
				huh.NewOption("🧪  Test a saver", "test"),
				huh.NewOption("⭐  Set default saver", "default"),
				huh.NewOption("🗑️   Remove a saver", "remove"),
				huh.NewOption("↩️   Back", "back"),
			}, &choice); err != nil {
			return err
		}
		switch choice {
		case "add":
			_ = runSaverAddWizard()
		case "list":
			_ = newSaverListCmd().RunE(newSaverListCmd(), nil)
		case "test":
			_ = runSaverTestWizard()
		case "default":
			_ = runSaverDefaultWizard()
		case "remove":
			_ = runSaverRemoveWizard()
		case "back", "":
			return nil
		}
	}
}

func runSaverAddWizard() error {
	if err := tui.RequireTTY(); err != nil {
		return err
	}
	cfg, err := requireConfig()
	if err != nil {
		return err
	}

	var name, u string
	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("Saver name").
			Description("Any short name you like. No hardcoded presets.").
			Placeholder("e.g. headroom, caveman, local-saver").
			Value(&name).
			Validate(func(s string) error {
				s = strings.TrimSpace(s)
				if s == "" {
					return fmt.Errorf("name is required")
				}
				if _, exists := cfg.Savers[s]; exists {
					return fmt.Errorf("saver %q already exists", s)
				}
				return nil
			}),
		huh.NewInput().
			Title("Saver base URL").
			Description("Where the saver proxy listens. Copy from its docs.").
			Placeholder("http://127.0.0.1:8787").
			Value(&u).
			Validate(func(s string) error {
				s = strings.TrimSpace(s)
				if s == "" {
					return fmt.Errorf("URL is required")
				}
				if _, err := url.ParseRequestURI(s); err != nil {
					return fmt.Errorf("invalid url: %w", err)
				}
				return nil
			}),
	))
	if err := tui.RunForm(form); err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	u = strings.TrimSpace(u)
	cfg.Savers[name] = config.SaverConfig{URL: u}
	if err := cfg.Save(); err != nil {
		return err
	}
	output.Success(fmt.Sprintf("Saver %q registered at %s.", name, u))
	tui.PrintEquivalent("eco saver add", []string{name, "--url " + u})
	return nil
}

func pickSaver(cfg *config.Config, title string) (string, error) {
	if len(cfg.Savers) == 0 {
		output.Info("No savers yet. Add one first.")
		return "", nil
	}
	var name string
	opts := make([]huh.Option[string], 0, len(cfg.Savers))
	for n, s := range cfg.Savers {
		label := fmt.Sprintf("%s  (%s)", n, s.URL)
		if n == cfg.Defaults.SaverDefault {
			label += "  ★"
		}
		opts = append(opts, huh.NewOption(label, n))
	}
	err := tui.SelectString(title, "", opts, &name)
	return name, err
}

func runSaverTestWizard() error {
	if err := tui.RequireTTY(); err != nil {
		return err
	}
	cfg, err := requireConfig()
	if err != nil {
		return err
	}
	name, err := pickSaver(cfg, "Test which saver?")
	if err != nil || name == "" {
		return err
	}
	return newSaverTestCmd().RunE(newSaverTestCmd(), []string{name})
}

func runSaverDefaultWizard() error {
	if err := tui.RequireTTY(); err != nil {
		return err
	}
	cfg, err := requireConfig()
	if err != nil {
		return err
	}
	name, err := pickSaver(cfg, "Set which saver as default?")
	if err != nil || name == "" {
		return err
	}
	cfg.Defaults.SaverDefault = name
	if err := cfg.Save(); err != nil {
		return err
	}
	output.Success(fmt.Sprintf("Default saver set to %q.", name))
	tui.PrintEquivalent("eco saver default", []string{name})
	return nil
}

func runSaverRemoveWizard() error {
	if err := tui.RequireTTY(); err != nil {
		return err
	}
	cfg, err := requireConfig()
	if err != nil {
		return err
	}
	name, err := pickSaver(cfg, "Remove which saver?")
	if err != nil || name == "" {
		return err
	}
	ok, _ := tui.Confirm("Remove saver "+name+"?", "Routes using --via "+name+" will need updating.", false)
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
	output.Success(fmt.Sprintf("Saver %q removed.", name))
	tui.PrintEquivalent("eco saver remove", []string{name})
	return nil
}
