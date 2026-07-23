package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/ganjar/ecorouter/internal/config"
	"github.com/ganjar/ecorouter/internal/output"
	"github.com/ganjar/ecorouter/internal/secrets"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func newProviderCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "provider",
		Short: "Manage LLM providers",
	}
	cmd.AddCommand(newProviderAddCmd(), newProviderListCmd(), newProviderTestCmd(), newProviderRemoveCmd())
	return cmd
}

func newProviderAddCmd() *cobra.Command {
	var key, baseURL, pType string
	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Add a provider (API key stored in secrets, not config)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			cfg, err := requireConfig()
			if err != nil {
				return err
			}
			if _, exists := cfg.Providers[name]; exists {
				return exitErr(1, fmt.Errorf("provider %q already exists", name))
			}
			if pType == "" {
				pType = "openai"
			}
			pType = strings.ToLower(pType)
			if pType != "openai" && pType != "anthropic" && pType != "ollama" {
				// Custom provider: require base URL
				if baseURL == "" {
					return exitErr(2, fmt.Errorf("custom provider type %q requires --base-url", pType))
				}
			}
			if baseURL == "" {
				switch pType {
				case "openai":
					baseURL = "https://api.openai.com/v1"
				case "anthropic":
					baseURL = "https://api.anthropic.com/v1"
				case "ollama":
					baseURL = "http://127.0.0.1:11434/v1"
				}
			}
			if key == "" && pType != "ollama" {
				fmt.Fprint(os.Stderr, "API key (hidden): ")
				b, err := term.ReadPassword(int(syscall.Stdin))
				fmt.Fprintln(os.Stderr)
				if err != nil {
					// non-tty fallback
					reader := bufio.NewReader(os.Stdin)
					line, _ := reader.ReadString('\n')
					key = strings.TrimSpace(line)
				} else {
					key = strings.TrimSpace(string(b))
				}
			}
			if key == "" && pType != "ollama" {
				return exitErr(2, fmt.Errorf("API key required (use --key or enter interactively)"))
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

			models, testErr := fetchModels(pType, baseURL, key)
			cfg.Providers[name] = config.ProviderConfig{
				Type:    pType,
				BaseURL: baseURL,
				Models:  models,
			}
			if err := cfg.Save(); err != nil {
				return err
			}

			if output.JSON {
				return output.PrintJSON(map[string]any{
					"name": name, "type": pType, "base_url": baseURL, "models": len(models),
					"verified": testErr == nil,
				})
			}
			if testErr != nil {
				output.Success(fmt.Sprintf("Provider %q added (%s).", name, pType))
				output.Warn(fmt.Sprintf("Could not verify connectivity: %v", testErr))
				output.Info("  Fix: eco provider test " + name)
			} else {
				output.Success(fmt.Sprintf("Provider %q added — %d models available.", name, len(models)))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&key, "key", "", "API key (prefer env: --key $OPENAI_API_KEY)")
	cmd.Flags().StringVar(&baseURL, "base-url", "", "provider base URL")
	cmd.Flags().StringVar(&pType, "type", "openai", "auth type: openai|anthropic|ollama (or any with --base-url)")
	return cmd
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
				rows = append(rows, row{name, p.Type, p.BaseURL, len(p.Models), has})
				dot := output.HealthDot(has)
				table = append(table, []string{dot + " " + name, p.Type, p.BaseURL, fmt.Sprintf("%d", len(p.Models))})
			}
			if output.JSON {
				return output.PrintJSON(rows)
			}
			if len(table) == 0 {
				output.Info("No providers. Add one: eco provider add openai --key $OPENAI_API_KEY")
				return nil
			}
			output.Table([]string{"NAME", "TYPE", "BASE URL", "MODELS"}, table)
			return nil
		},
	}
}

func newProviderTestCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "test <name>",
		Short: "Live connectivity + auth check",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			cfg, err := requireConfig()
			if err != nil {
				return err
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
			return nil
		},
	}
}

func newProviderRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove provider and purge its secret",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			cfg, err := requireConfig()
			if err != nil {
				return err
			}
			if _, ok := cfg.Providers[name]; !ok {
				return exitErr(1, fmt.Errorf("provider %q not found", name))
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
			return nil
		},
	}
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
