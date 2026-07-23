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

// runProviderMenu is the top-level "Providers" screen.
func runProviderMenu() error {
	for {
		var choice string
		err := tui.SelectString(
			"🔌  Providers",
			"Add and manage LLM providers.",
			[]huh.Option[string]{
				huh.NewOption("➕  Add a new provider", "add"),
				huh.NewOption("📋  List providers", "list"),
				huh.NewOption("🧪  Test a provider", "test"),
				huh.NewOption("✏️   Edit a provider (rename, change URL, refresh models)", "edit"),
				huh.NewOption("💰  Set / edit model pricing", "pricing"),
				huh.NewOption("🗑️   Remove a provider", "remove"),
				huh.NewOption("↩️   Back", "back"),
			},
			&choice,
		)
		if err != nil {
			return err
		}
		switch choice {
		case "add":
			_ = runProviderAddWizard()
		case "list":
			cfg, err := requireConfig()
			if err == nil {
				providerListPrint(cfg)
			}
		case "test":
			_ = runProviderPickAndTest()
		case "edit":
			_ = runProviderEditWizard()
		case "pricing":
			_ = runPricingWizard()
		case "remove":
			_ = runProviderRemoveWizard()
		case "back", "":
			return nil
		}
	}
}

// runProviderAddWizard walks through a full "add provider" flow.
// Everything is user-entered — no vendor presets, no hardcoded URLs.
func runProviderAddWizard() error {
	if err := tui.RequireTTY(); err != nil {
		return err
	}
	cfg, err := requireConfig()
	if err != nil {
		return err
	}

	// Step 1 — starting shape (auth style)
	var authStyle string
	if err := tui.SelectString(
		"How does this provider authenticate?",
		"If you're unsure, pick the first — most modern APIs use Bearer tokens.",
		[]huh.Option[string]{
			huh.NewOption("🔑  Bearer token in Authorization header  (OpenAI, DeepSeek, Groq, OpenRouter, Together, xAI, Mistral, …)", "bearer"),
			huh.NewOption("🗝️   x-api-key header  (Anthropic and Anthropic-compatible gateways)", "anthropic-key"),
			huh.NewOption("🚫  No authentication  (local models: Ollama, LM Studio, vLLM, llama.cpp)", "none"),
		},
		&authStyle,
	); err != nil {
		return err
	}

	// Step 2 — name, base URL
	var name, baseURL, key string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Give this provider a name").
				Description("This is how you'll refer to it in routes. Any short name you like.").
				Placeholder("e.g. openrouter, deepseek, local-llama").
				Value(&name).
				Validate(func(s string) error {
					s = strings.TrimSpace(s)
					if s == "" {
						return fmt.Errorf("name is required")
					}
					if strings.ContainsAny(s, " /\\") {
						return fmt.Errorf("name cannot contain spaces or slashes")
					}
					if _, exists := cfg.Providers[s]; exists {
						return fmt.Errorf("provider %q already exists", s)
					}
					return nil
				}),
			huh.NewInput().
				Title("Base URL").
				Description("Copy from the provider's docs. Include the version prefix (e.g. /v1).").
				Placeholder("https://api.example.com/v1").
				Value(&baseURL).
				Validate(func(s string) error {
					s = strings.TrimSpace(s)
					if s == "" {
						return fmt.Errorf("URL is required")
					}
					if !strings.HasPrefix(s, "http://") && !strings.HasPrefix(s, "https://") {
						return fmt.Errorf("must start with http:// or https://")
					}
					return nil
				}),
		),
	)
	if err := tui.RunForm(form); err != nil {
		return err
	}

	if authStyle != "none" {
		if err := tui.Password(
			"API key",
			"Pasted from the provider's dashboard. Hidden as you type.",
			&key,
		); err != nil {
			return err
		}
	}

	// Save provider (without models yet)
	name = strings.TrimSpace(name)
	baseURL = strings.TrimSpace(baseURL)
	pType := authStyleToType(authStyle)
	cfg.Providers[name] = config.ProviderConfig{
		Type:    pType,
		BaseURL: baseURL,
	}
	sec, err := secrets.Load("")
	if err != nil {
		return err
	}
	if key != "" {
		if err := sec.Set(name, key); err != nil {
			return err
		}
	}
	if err := cfg.Save(); err != nil {
		return err
	}

	// Step 3 — try to fetch model catalog
	output.Info("  Testing connection and fetching model catalog…")
	models, testErr := fetchModels(pType, baseURL, key)
	if testErr != nil {
		output.Warn(fmt.Sprintf("Could not fetch models automatically: %v", testErr))
		output.Info("You can add models manually next.")
		manual, _ := tui.Confirm(
			"Add models manually?",
			"Enter comma-separated model IDs you want to use.",
			true,
		)
		if manual {
			var manualModels string
			_ = tui.Input(
				"Model IDs",
				"Comma-separated. Example: gpt-4o, gpt-4o-mini",
				"model-a, model-b, ...",
				&manualModels,
				nil,
			)
			p := cfg.Providers[name]
			p.Models = splitModels(manualModels)
			cfg.Providers[name] = p
			_ = cfg.Save()
		}
	} else if len(models) == 0 {
		output.Warn("Connected, but the catalog is empty. You can add models manually later via Edit.")
	} else {
		// Step 4 — multi-select which models to enable
		var chosen []string
		opts := make([]huh.Option[string], 0, len(models))
		for _, m := range models {
			opts = append(opts, huh.NewOption(m, m))
		}
		_ = huh.NewMultiSelect[string]().
			Title(fmt.Sprintf("Found %d models. Which ones do you want available?", len(models))).
			Description("Space to toggle. Enter to confirm. Leave empty to enable ALL.").
			Options(opts...).
			Value(&chosen).
			WithTheme(tui.Theme()).
			Run()

		p := cfg.Providers[name]
		if len(chosen) == 0 {
			p.Models = models
		} else {
			p.Models = chosen
		}
		cfg.Providers[name] = p
		_ = cfg.Save()

		output.Success(fmt.Sprintf("Provider %q added — %d model(s) enabled.", name, len(p.Models)))
	}

	// Step 5 — offer pricing
	if len(cfg.Providers[name].Models) > 0 {
		want, _ := tui.Confirm(
			"Set prices for these models now?",
			"You can also do this later via Providers → Set / edit model pricing.",
			false,
		)
		if want {
			_ = runPricingWizardFor(name)
		}
	}

	// Equivalent flag command
	argsList := []string{
		name,
		"--auth " + authStyle,
		"--base-url " + baseURL,
	}
	if key != "" {
		argsList = append(argsList, "--key $KEY")
	}
	if len(cfg.Providers[name].Models) > 0 {
		argsList = append(argsList, fmt.Sprintf("--models %q", strings.Join(cfg.Providers[name].Models, ",")))
	}
	tui.PrintEquivalent("eco provider add", argsList)
	return nil
}

func runProviderPickAndTest() error {
	if err := tui.RequireTTY(); err != nil {
		return err
	}
	cfg, err := requireConfig()
	if err != nil {
		return err
	}
	name, err := pickProvider(cfg, "Test which provider?")
	if err != nil || name == "" {
		return err
	}
	return newProviderTestCmd().RunE(newProviderTestCmd(), []string{name})
}

func pickProvider(cfg *config.Config, title string) (string, error) {
	if len(cfg.Providers) == 0 {
		output.Info("No providers yet. Add one first.")
		return "", nil
	}
	var name string
	opts := make([]huh.Option[string], 0, len(cfg.Providers))
	for n, p := range cfg.Providers {
		opts = append(opts, huh.NewOption(fmt.Sprintf("%s  (%s)", n, p.BaseURL), n))
	}
	err := tui.SelectString(title, "", opts, &name)
	return name, err
}

func providerListPrint(cfg *config.Config) {
	sec, _ := secrets.Load("")
	var table [][]string
	for name, p := range cfg.Providers {
		_, has := sec.Get(name)
		if p.Type == "ollama" {
			has = true
		}
		dot := output.HealthDot(has)
		table = append(table, []string{dot + " " + name, authStyleLabel(p.Type), p.BaseURL, fmt.Sprintf("%d", len(p.Models))})
	}
	if len(table) == 0 {
		output.Info("No providers. Add one from the menu or: eco provider add <name> --auth bearer --base-url https://…")
		return
	}
	output.Table([]string{"NAME", "AUTH", "BASE URL", "MODELS"}, table)
}
