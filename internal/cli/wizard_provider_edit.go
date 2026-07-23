package cli

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/ganjar/ecorouter/internal/config"
	"github.com/ganjar/ecorouter/internal/output"
	"github.com/ganjar/ecorouter/internal/secrets"
	"github.com/ganjar/ecorouter/internal/tui"
)

func runProviderEditWizard() error {
	if err := tui.RequireTTY(); err != nil {
		return err
	}
	cfg, err := requireConfig()
	if err != nil {
		return err
	}
	name, err := pickProvider(cfg, "Which provider to edit?")
	if err != nil || name == "" {
		return err
	}

	var action string
	if err := tui.SelectString("Edit "+name, "", []huh.Option[string]{
		huh.NewOption("✏️   Rename", "rename"),
		huh.NewOption("🌐  Change base URL", "url"),
		huh.NewOption("🔄  Refresh model catalog", "refresh"),
		huh.NewOption("🎯  Choose enabled models", "models"),
		huh.NewOption("🔑  Rotate API key", "key"),
		huh.NewOption("↩️   Back", "back"),
	}, &action); err != nil {
		return err
	}

	switch action {
	case "rename":
		return providerRename(cfg, name)
	case "url":
		return providerChangeURL(cfg, name)
	case "refresh":
		return providerRefreshModels(cfg, name)
	case "models":
		return providerChooseModels(cfg, name)
	case "key":
		return providerRotateKey(cfg, name)
	default:
		return nil
	}
}

func providerRename(cfg *config.Config, oldName string) error {
	var newName string
	if err := tui.Input(
		"New name for "+oldName,
		"Routes that reference this provider's models will be updated.",
		oldName,
		&newName,
		func(s string) error {
			s = strings.TrimSpace(s)
			if s == "" {
				return fmt.Errorf("name is required")
			}
			if strings.ContainsAny(s, " /\\") {
				return fmt.Errorf("name cannot contain spaces or slashes")
			}
			if s != oldName {
				if _, exists := cfg.Providers[s]; exists {
					return fmt.Errorf("provider %q already exists", s)
				}
			}
			return nil
		},
	); err != nil {
		return err
	}
	newName = strings.TrimSpace(newName)
	if newName == oldName {
		return nil
	}

	p := cfg.Providers[oldName]
	delete(cfg.Providers, oldName)
	cfg.Providers[newName] = p

	// Update routes that use "oldName/model" prefixes
	prefix := oldName + "/"
	newPrefix := newName + "/"
	for rName, r := range cfg.Routes {
		changed := false
		for i, m := range r.Models {
			if strings.HasPrefix(m, prefix) {
				r.Models[i] = newPrefix + strings.TrimPrefix(m, prefix)
				changed = true
			} else if !strings.Contains(m, "/") && providerOwnsBareModel(p, m) {
				// bare model IDs stay as-is
			}
			_ = i
		}
		if changed {
			cfg.Routes[rName] = r
		}
	}

	// Move secret
	if sec, err := secrets.Load(""); err == nil {
		if key, ok := sec.Get(oldName); ok {
			_ = sec.Set(newName, key)
			_ = sec.Delete(oldName)
		}
	}

	if err := cfg.Save(); err != nil {
		return err
	}
	output.Success(fmt.Sprintf("Provider renamed %q → %q.", oldName, newName))
	tui.PrintEquivalent("eco provider …", []string{"# rename is interactive-only; edit config.toml if scripting"})
	return nil
}

func providerOwnsBareModel(p config.ProviderConfig, model string) bool {
	for _, m := range p.Models {
		if m == model {
			return true
		}
	}
	return false
}

func providerChangeURL(cfg *config.Config, name string) error {
	p := cfg.Providers[name]
	var baseURL string
	if err := tui.Input(
		"New base URL for "+name,
		"Copy from the provider's docs. Include the version prefix (e.g. /v1).",
		p.BaseURL,
		&baseURL,
		func(s string) error {
			s = strings.TrimSpace(s)
			if s == "" {
				return fmt.Errorf("URL is required")
			}
			if !strings.HasPrefix(s, "http://") && !strings.HasPrefix(s, "https://") {
				return fmt.Errorf("must start with http:// or https://")
			}
			return nil
		},
	); err != nil {
		return err
	}
	p.BaseURL = strings.TrimSpace(baseURL)
	cfg.Providers[name] = p
	if err := cfg.Save(); err != nil {
		return err
	}
	output.Success("Base URL updated.")
	// offer refresh
	if ok, _ := tui.Confirm("Refresh model catalog from the new URL?", "", true); ok {
		return providerRefreshModels(cfg, name)
	}
	tui.PrintEquivalent("eco provider test", []string{name})
	return nil
}

func providerRefreshModels(cfg *config.Config, name string) error {
	p := cfg.Providers[name]
	sec, err := secrets.Load("")
	if err != nil {
		return err
	}
	key, _ := sec.Get(name)
	output.Info("  Fetching model catalog…")
	models, err := fetchModels(p.Type, p.BaseURL, key)
	if err != nil {
		output.Warn(fmt.Sprintf("Could not fetch models: %v", err))
		return nil
	}
	if len(models) == 0 {
		output.Warn("Catalog is empty.")
		return nil
	}

	// multi-select preloaded with currently enabled ones
	enabled := map[string]bool{}
	for _, m := range p.Models {
		enabled[m] = true
	}
	var chosen []string
	opts := make([]huh.Option[string], 0, len(models))
	for _, m := range models {
		opts = append(opts, huh.NewOption(m, m))
		if enabled[m] {
			chosen = append(chosen, m)
		}
	}
	if err := tui.MultiSelect(
		fmt.Sprintf("Found %d models. Which ones to enable?", len(models)),
		"Space to toggle. Currently enabled models are pre-selected.",
		opts,
		&chosen,
	); err != nil {
		return err
	}
	if len(chosen) == 0 {
		chosen = models
	}
	p.Models = chosen
	cfg.Providers[name] = p
	if err := cfg.Save(); err != nil {
		return err
	}
	output.Success(fmt.Sprintf("Enabled %d model(s) for %q.", len(chosen), name))
	tui.PrintEquivalent("eco provider test", []string{name})
	return nil
}

func providerChooseModels(cfg *config.Config, name string) error {
	p := cfg.Providers[name]
	if len(p.Models) == 0 {
		// try refresh first
		return providerRefreshModels(cfg, name)
	}
	var manual string
	if err := tui.Input(
		"Enabled model IDs (comma-separated)",
		"Edit the list of models available from this provider.",
		strings.Join(p.Models, ", "),
		&manual,
		func(s string) error {
			if len(splitModels(s)) == 0 {
				return fmt.Errorf("at least one model required")
			}
			return nil
		},
	); err != nil {
		return err
	}
	p.Models = splitModels(manual)
	cfg.Providers[name] = p
	if err := cfg.Save(); err != nil {
		return err
	}
	output.Success(fmt.Sprintf("Updated models for %q (%d).", name, len(p.Models)))
	return nil
}

func providerRotateKey(cfg *config.Config, name string) error {
	p := cfg.Providers[name]
	if p.Type == "ollama" {
		output.Info("This provider uses no authentication.")
		return nil
	}
	var key string
	if err := tui.Password(
		"New API key for "+name,
		"Pasted from the provider's dashboard. Hidden as you type.",
		&key,
	); err != nil {
		return err
	}
	key = strings.TrimSpace(key)
	if key == "" {
		output.Warn("Empty key — aborted.")
		return nil
	}
	sec, err := secrets.Load("")
	if err != nil {
		return err
	}
	if err := sec.Set(name, key); err != nil {
		return err
	}
	output.Success("API key updated.")
	// verify
	output.Info("  Verifying…")
	if _, err := fetchModels(p.Type, p.BaseURL, key); err != nil {
		output.Warn(fmt.Sprintf("Key saved but verification failed: %v", err))
	} else {
		output.Success("Verified.")
	}
	tui.PrintEquivalent("eco provider add", []string{name, "--key $KEY  # re-add / rotate via secrets"})
	return nil
}
