package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/ganjar/ecorouter/internal/config"
	"github.com/ganjar/ecorouter/internal/output"
	"github.com/ganjar/ecorouter/internal/secrets"
	"github.com/ganjar/ecorouter/internal/tui"
	"github.com/spf13/cobra"
)

func newProviderCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "provider",
		Short: "Manage LLM providers",
		Long:  `Manage LLM providers. Each provider holds a base URL and an API key (stored separately, never in config.toml).`,
	}
	cmd.AddCommand(newProviderAddCmd(), newProviderListCmd(), newProviderTestCmd(), newProviderRemoveCmd())
	return cmd
}

func newProviderAddCmd() *cobra.Command {
	var key, baseURL, pAuth, pTypeLegacy, modelsFlag string
	cmd := &cobra.Command{
		Use:   "add [name]",
		Short: "Add a provider (API key stored in secrets, not config)",
		Long: `Add an LLM provider.

💡 Run with no arguments (or --wizard) to be guided step-by-step.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := requireConfig()
			if err != nil {
				return err
			}
			force := WizardRequested()

			// name may come from positional arg
			name := ""
			if len(args) == 1 {
				name = args[0]
			}

			// 1. auth style (choice) — pTypeLegacy is a deprecated --type alias
			authIn := pAuth
			if authIn == "" {
				authIn = pTypeLegacy
			}
			if !force && pTypeLegacy != "" && pAuth == "" {
				fmt.Fprintln(cmd.ErrOrStderr(), "warning: --type is deprecated; use --auth")
			}
			authVal, err := askChoice(authIn, "auth",
				"How does this provider authenticate?",
				"If unsure, pick the first — most APIs use Bearer tokens.",
				[]huh.Option[string]{
					huh.NewOption("🔑  Bearer token in Authorization header", "bearer"),
					huh.NewOption("🗝️   x-api-key header (Anthropic-style)", "anthropic-key"),
					huh.NewOption("🚫  No authentication (local models)", "none"),
				}, force)
			if err != nil {
				return err
			}
			pType := normalizeAuth(authVal)
			isNoAuth := pType == "ollama"

			// 2. name (text)
			name, err = askString(name, "name",
				"Give this provider a name",
				"How you'll refer to it in routes.",
				"e.g. openrouter, deepseek, local-llama", force,
				func(s string) error {
					s = strings.TrimSpace(s)
					if s == "" {
						return fmt.Errorf("name required")
					}
					if strings.ContainsAny(s, " /\\") {
						return fmt.Errorf("name cannot contain spaces or slashes")
					}
					if _, ok := cfg.Providers[s]; ok {
						return fmt.Errorf("provider %q already exists", s)
					}
					return nil
				})
			if err != nil {
				return err
			}
			name = strings.TrimSpace(name)

			// 3. base URL (text) — NO hardcoded default
			baseURL, err = askString(baseURL, "base-url",
				"Base URL",
				"Copy from the provider's docs, include the version prefix.",
				"https://api.example.com/v1", force,
				func(s string) error {
					s = strings.TrimSpace(s)
					if s == "" {
						return fmt.Errorf("URL required")
					}
					if !strings.HasPrefix(s, "http://") && !strings.HasPrefix(s, "https://") {
						return fmt.Errorf("must start with http:// or https://")
					}
					return nil
				})
			if err != nil {
				return err
			}
			baseURL = strings.TrimSpace(baseURL)

			// 4. key (secret, unless none)
			if !isNoAuth {
				key, err = askSecret(key, "key",
					"API key", "Pasted from the provider dashboard. Hidden as you type.", force)
				if err != nil {
					return err
				}
			}

			// 5. Persist + fetch + model multi-select
			sec, err := secrets.Load("")
			if err != nil {
				return err
			}
			if key != "" {
				if err := sec.Set(name, key); err != nil {
					return err
				}
			}

			var models []string
			var testErr error
			if modelsFlag != "" {
				models = splitModels(modelsFlag)
			} else {
				if tui.IsInteractive() {
					fmt.Println("  ⏳ Testing connection and fetching model catalog…")
				}
				models, testErr = fetchModels(pType, baseURL, key)
				// If interactive (or wizard) AND no --models, offer a multi-select.
				if testErr == nil && len(models) > 0 && tui.IsInteractive() {
					var chosen []string
					mErr := tui.MultiSelect(
						fmt.Sprintf("Found %d models. Which ones do you want available?", len(models)),
						"Space to toggle. Enter to confirm. Leave empty to enable ALL.",
						modelOptionList(models), &chosen)
					if mErr == nil && len(chosen) > 0 {
						models = chosen
					}
				}
			}
			cfg.Providers[name] = config.ProviderConfig{
				Type:    pType,
				BaseURL: baseURL,
				Models:  models,
			}
			if err := cfg.Save(); err != nil {
				return err
			}

			if output.JSON {
				_ = output.PrintJSON(map[string]any{
					"name": name, "type": pType, "auth": typeToAuthStyle(pType),
					"base_url": baseURL, "models": len(models),
					"verified": testErr == nil && modelsFlag == "",
				})
				printEquivalentIfInteractive("eco provider add", []string{
					name,
					"--auth " + authVal,
					"--base-url " + baseURL,
				}, key, models)
				return nil
			}
			if modelsFlag == "" && testErr != nil {
				output.Success(fmt.Sprintf("Provider %q added (%s).", name, typeToAuthStyle(pType)))
				output.Warn(fmt.Sprintf("Could not verify connectivity: %v", testErr))
				output.Info("  Fix: eco provider test " + name)
			} else {
				output.Success(fmt.Sprintf("Provider %q added — %d model(s) enabled.", name, len(models)))
			}
			printEquivalentIfInteractive("eco provider add", []string{
				name,
				"--auth " + authVal,
				"--base-url " + baseURL,
			}, key, models)
			return nil
		},
	}
	cmd.Flags().StringVar(&key, "key", "", "API key (prefer env: --key $OPENAI_API_KEY)")
	cmd.Flags().StringVar(&baseURL, "base-url", "", "provider base URL (required; no defaults)")
	cmd.Flags().StringVar(&pAuth, "auth", "", "bearer|anthropic-key|none  (or legacy: openai|anthropic|ollama)")
	cmd.Flags().StringVar(&pTypeLegacy, "type", "", "DEPRECATED alias for --auth")
	_ = cmd.Flags().MarkHidden("type")
	cmd.Flags().StringVar(&modelsFlag, "models", "", "comma-separated model IDs (skip catalog fetch)")
	return cmd
}

// modelOptionList converts a list of model IDs into huh.Options.
func modelOptionList(models []string) []huh.Option[string] {
	o := make([]huh.Option[string], 0, len(models))
	for _, m := range models {
		o = append(o, huh.NewOption(m, m))
	}
	return o
}

// printEquivalentIfInteractive shows the flag-form of what the command did,
// but only when running interactively (so scripts stay quiet).
// It also redacts the key into $KEY for teachability.
func printEquivalentIfInteractive(cmd string, args []string, key string, models []string) {
	if !tui.IsInteractive() {
		return
	}
	all := append([]string{}, args...)
	if key != "" {
		all = append(all, "--key $KEY")
	}
	if len(models) > 0 {
		all = append(all, fmt.Sprintf("--models %q", strings.Join(models, ",")))
	}
	tui.PrintEquivalent(cmd, all)
}

func newProviderListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List providers with health",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := requireConfig()
			if err != nil {
				return err
			}
			sec, _ := secrets.Load("")
			type row struct {
				Name    string `json:"name"`
				Type    string `json:"type"`
				Auth    string `json:"auth"`
				BaseURL string `json:"base_url"`
				Models  int    `json:"models"`
				HasKey  bool   `json:"has_key"`
			}
			var rows []row
			var table [][]string
			for name, p := range cfg.Providers {
				_, has := sec.Get(name)
				if p.Type == "ollama" {
					has = true
				}
				rows = append(rows, row{name, p.Type, typeToAuthStyle(p.Type), p.BaseURL, len(p.Models), has})
				dot := output.HealthDot(has)
				table = append(table, []string{dot + " " + name, typeToAuthStyle(p.Type), p.BaseURL, fmt.Sprintf("%d", len(p.Models))})
			}
			if output.JSON {
				return output.PrintJSON(rows)
			}
			if len(table) == 0 {
				output.Info("No providers. Add one: eco provider add myapi --auth bearer --base-url https://… --key $KEY")
				return nil
			}
			output.Table([]string{"NAME", "AUTH", "BASE URL", "MODELS"}, table)
			return nil
		},
	}
}

func newProviderTestCmd() *cobra.Command {
	testCmd := &cobra.Command{
		Use:   "test [name]",
		Short: "Live connectivity + auth check",
		Long: `Live connectivity + auth check for an existing provider.

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
			name, err = askPick(name, "name", "Test which provider?",
				"Live connectivity check.", providerOptions(cfg), force)
			if err != nil {
				return err
			}
			if name == "" {
				return exitErr(1, fmt.Errorf("no provider selected"))
			}
			p, ok := cfg.Providers[name]
			if !ok {
				return exitErr(1, fmt.Errorf("provider %q not found", name))
			}
			sec, err := secrets.Load("")
			if err != nil {
				return err
			}
			key, _ := sec.Get(name)
			models, err := fetchModels(p.Type, p.BaseURL, key)
			if err != nil {
				if output.JSON {
					output.FailJSON("provider_error", err.Error(), "check API key and base URL")
					return exitErr(4, err)
				}
				output.Fail(fmt.Sprintf("Provider %q unreachable or unauthorized.", name),
					"check key with eco provider add, or base URL")
				return exitErr(4, err)
			}
			// refresh catalog
			p.Models = models
			cfg.Providers[name] = p
			_ = cfg.Save()
			if output.JSON {
				return output.PrintJSON(map[string]any{"ok": true, "models": len(models)})
			}
			output.Success(fmt.Sprintf("Provider %q OK — %d models.", name, len(models)))
			tui.PrintEquivalent("eco provider test", []string{name})
			return nil
		},
	}
	return testCmd
}

func newProviderRemoveCmd() *cobra.Command {
	var assumeYes bool
	cmd := &cobra.Command{
		Use:   "remove [name]",
		Short: "Remove provider and purge its secret",
		Long: `Remove a provider and purge its API key from secrets.

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
			name, err = askPick(name, "name", "Remove which provider?",
				"Provider to delete (also purges its secret).", providerOptions(cfg), force)
			if err != nil {
				return err
			}
			if name == "" {
				return exitErr(1, fmt.Errorf("no provider selected"))
			}
			if _, ok := cfg.Providers[name]; !ok {
				return exitErr(1, fmt.Errorf("provider %q not found", name))
			}
			ok, err := confirmDestructive(assumeYes,
				fmt.Sprintf("Remove provider %q?", name),
				"Deletes the provider from config and purges its API key. Routes that reference its models will break.")
			if err != nil {
				return err
			}
			if !ok {
				return nil
			}
			delete(cfg.Providers, name)
			if err := cfg.Save(); err != nil {
				return err
			}
			if sec, err := secrets.Load(""); err == nil {
				_ = sec.Delete(name)
			}
			if output.JSON {
				return output.PrintJSON(map[string]string{"removed": name})
			}
			output.Success(fmt.Sprintf("Provider %q removed; secret purged.", name))
			tui.PrintEquivalent("eco provider remove", []string{name, "--yes"})
			return nil
		},
	}
	cmd.Flags().BoolVarP(&assumeYes, "yes", "y", false, "skip confirmation prompt")
	return cmd
}

func fetchModels(pType, baseURL, key string) ([]string, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	url := strings.TrimRight(baseURL, "/") + "/models"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	switch strings.ToLower(pType) {
	case "anthropic":
		if key != "" {
			req.Header.Set("x-api-key", key)
			req.Header.Set("anthropic-version", "2023-06-01")
		}
	default:
		if key != "" {
			req.Header.Set("Authorization", "Bearer "+key)
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	var parsed struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
		// ollama native
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parse models: %w", err)
	}
	var out []string
	for _, m := range parsed.Data {
		if m.ID != "" {
			out = append(out, m.ID)
		}
	}
	for _, m := range parsed.Models {
		if m.Name != "" {
			out = append(out, m.Name)
		}
	}
	return out, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
