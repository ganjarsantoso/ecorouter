package cli

import (
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/ganjar/ecorouter/internal/auth"
	"github.com/ganjar/ecorouter/internal/config"
	"github.com/ganjar/ecorouter/internal/output"
	"github.com/ganjar/ecorouter/internal/secrets"
	"github.com/ganjar/ecorouter/internal/store"
	"github.com/ganjar/ecorouter/internal/tui"
	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	var nonInteractive bool
	var domain, providerType, providerAuth, providerName, providerKey, providerBaseURL string
	var routeName, routeMode, routeModels, tokenLabel, tokenExpires string
	cmd := &cobra.Command{
		Use:   "init",
		Short: "First-run wizard: domain, provider, route, token",
		Long:  `Interactive setup for EcoRouter. Also fully scriptable via flags for automated provisioning.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := config.EnsureDirs(); err != nil {
				return err
			}

			// Interactive path uses the menu-driven wizard.
			if !nonInteractive && tui.IsInteractive() {
				return runInitWizard()
			}

			// Non-interactive / flag-driven path
			if !nonInteractive && !tui.IsInteractive() {
				return exitErr(2, fmt.Errorf("run 'eco init --yes' with flags for non-interactive setup"))
			}

			return runInitNonInteractive(domain, providerType, providerAuth, providerName, providerKey, providerBaseURL,
				routeName, routeMode, routeModels, tokenLabel, tokenExpires, cmd)
		},
	}
	cmd.Flags().BoolVar(&nonInteractive, "yes", false, "non-interactive (use flags/env)")
	cmd.Flags().StringVar(&domain, "domain", "", "public domain")
	cmd.Flags().StringVar(&providerAuth, "provider-auth", "", "bearer|anthropic-key|none  (or legacy: openai|anthropic|ollama)")
	cmd.Flags().StringVar(&providerType, "provider-type", "", "DEPRECATED alias for --provider-auth")
	_ = cmd.Flags().MarkHidden("provider-type")
	cmd.Flags().StringVar(&providerName, "provider-name", "", "provider name (required with --yes)")
	cmd.Flags().StringVar(&providerKey, "provider-key", "", "provider API key")
	cmd.Flags().StringVar(&providerBaseURL, "provider-base-url", "", "provider base URL (required with --yes)")
	cmd.Flags().StringVar(&routeName, "route-name", "default", "first route name")
	cmd.Flags().StringVar(&routeMode, "route-mode", "fallback", "single|fallback|round")
	cmd.Flags().StringVar(&routeModels, "route-models", "", "comma-separated models (required with --yes)")
	cmd.Flags().StringVar(&tokenLabel, "token-label", "default", "first token label")
	cmd.Flags().StringVar(&tokenExpires, "token-expires", "", "token expiry e.g. 90d")
	return cmd
}

func runInitWizard() error {
	if err := config.EnsureDirs(); err != nil {
		return err
	}
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	if !tui.IsInteractive() {
		return fmt.Errorf("run 'eco init --yes' with flags for non-interactive setup")
	}

	// Welcome
	fmt.Println()
	fmt.Println("  🌿  Welcome to EcoRouter. This wizard sets up a secure endpoint.")
	fmt.Println()

	// Step 1: Domain (optional)
	var domain string
	_ = tui.Input(
		"1/5  Public domain (optional)",
		"Leave blank for local-only. For public HTTPS via Caddy, enter your FQDN.",
		"e.g. eco.you.dev",
		&domain, nil,
	)
	domain = strings.TrimSpace(domain)
	cfg.Server.Domain = domain
	cfg.Server.Host = "127.0.0.1"
	if domain != "" {
		if ips, err := net.LookupHost(domain); err == nil && len(ips) > 0 {
			output.Success("DNS resolves to " + strings.Join(ips, ", "))
		} else {
			output.Warn("DNS does not resolve yet — configure A record before TLS.")
		}
	}
	if err := cfg.Save(); err != nil {
		return err
	}

	// Step 2: TLS notes (info-only)
	fmt.Println()
	fmt.Println("  2/5  TLS")
	fmt.Println("       EcoRouter binds loopback only. For public HTTPS use Caddy.")
	fmt.Println("       A Caddyfile snippet will be generated for you.")
	if domain != "" {
		if path, err := writeCaddySnippet(domain, cfg.Server.Port); err == nil {
			output.Info("       Caddyfile snippet: " + path)
		}
	}

	// Step 3: Provider — REUSE the add wizard, no hardcoded URLs
	fmt.Println()
	fmt.Println("  3/5  Add your first provider")
	if err := runProviderAddWizard(); err != nil {
		return err
	}
	// reload cfg after wizard saved
	cfg, err = loadConfig()
	if err != nil {
		return err
	}

	// Step 4: Route
	fmt.Println()
	fmt.Println("  4/5  Create your first route")
	if err := runRouteAddWizard(); err != nil {
		return err
	}
	cfg, err = loadConfig()
	if err != nil {
		return err
	}

	// Step 5: Token
	fmt.Println()
	fmt.Println("  5/5  Issue an access token")
	if err := runTokenNewWizard(); err != nil {
		return err
	}

	// Summary
	base := "http://127.0.0.1:8080"
	if cfg.Server.Domain != "" {
		base = "https://" + cfg.Server.Domain
	}
	fmt.Println()
	output.Success("Your endpoint is ready 🎉")
	fmt.Println()
	fmt.Printf("    Base URL:  %s\n", base)
	fmt.Println()
	fmt.Println("  Next:  eco start -d")
	return nil
}

func runInitNonInteractive(
	domain, providerType, providerAuth, providerName, providerKey, providerBaseURL,
	routeName, routeMode, routeModels, tokenLabel, tokenExpires string,
	cmd *cobra.Command,
) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	// Domain
	cfg.Server.Domain = domain
	cfg.Server.Host = "127.0.0.1"
	if domain != "" {
		if path, err := writeCaddySnippet(domain, cfg.Server.Port); err == nil {
			output.Info("  Caddyfile snippet: " + path)
		}
	}

	// Resolve auth style: --provider-auth preferred; --provider-type is deprecated alias
	authIn := providerAuth
	if authIn == "" {
		authIn = providerType
		if providerType != "" && cmd != nil && cmd.Flags().Changed("provider-type") {
			fmt.Fprintln(os.Stderr, "warning: --provider-type is deprecated; use --provider-auth")
		}
	}
	pType := normalizeAuth(authIn)

	// Provider name is required (no longer defaults to type name)
	name := strings.TrimSpace(providerName)
	if name == "" {
		// backward-compat: if only type given, use it as name (legacy scripts)
		if providerType != "" {
			name = strings.ToLower(providerType)
		} else if authIn != "" {
			name = typeToAuthStyle(pType)
			if name == "bearer" {
				name = "openai"
			} else if name == "anthropic-key" {
				name = "anthropic"
			} else if name == "none" {
				name = "ollama"
			}
		}
	}
	if name == "" {
		return exitErr(2, fmt.Errorf("--provider-name is required in non-interactive mode"))
	}

	// NEVER substitute a default base URL
	baseURL := strings.TrimSpace(providerBaseURL)
	if baseURL == "" {
		return exitErr(2, fmt.Errorf("--provider-base-url is required in non-interactive mode"))
	}
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		return exitErr(2, fmt.Errorf("--provider-base-url must start with http:// or https://"))
	}

	if _, exists := cfg.Providers[name]; !exists {
		key := providerKey
		if key == "" {
			if pType == "anthropic" {
				key = os.Getenv("ANTHROPIC_API_KEY")
			} else if pType != "ollama" {
				key = os.Getenv("OPENAI_API_KEY")
			}
		}
		if key == "" && pType != "ollama" {
			return exitErr(2, fmt.Errorf("API key required (--provider-key or env)"))
		}
		sec, err := secrets.Load("")
		if err != nil {
			return err
		}
		if key != "" {
			_ = sec.Set(name, key)
		}
		models, testErr := fetchModels(pType, baseURL, key)
		cfg.Providers[name] = config.ProviderConfig{Type: pType, BaseURL: baseURL, Models: models}
		if testErr != nil {
			output.Warn(fmt.Sprintf("Provider verify failed: %v (saved anyway)", testErr))
		} else {
			output.Success(fmt.Sprintf("Verified — %d models available.", len(models)))
		}
	} else {
		output.Info("Provider " + name + " already configured.")
	}

	// Route
	if routeName == "" {
		routeName = "default"
	}
	if routeMode == "" {
		routeMode = "fallback"
	}
	if strings.TrimSpace(routeModels) == "" {
		return exitErr(2, fmt.Errorf("--route-models is required in non-interactive mode"))
	}
	models := splitModels(routeModels)
	rc := config.RouteConfig{Mode: strings.ToLower(routeMode), Models: models}
	if rc.Mode == "single" && len(models) > 0 {
		rc.Models = models[:1]
	}
	cfg.Routes[routeName] = rc
	cfg.Defaults.ActiveRoute = routeName
	output.Success(fmt.Sprintf("Route %q created and set active.", routeName))

	if tokenLabel == "" {
		tokenLabel = "default"
	}
	if err := cfg.Save(); err != nil {
		return err
	}

	db, err := store.Open("")
	if err != nil {
		return err
	}
	defer db.Close()
	plain, err := auth.Generate()
	if err != nil {
		return err
	}
	hr, err := auth.Hash(plain)
	if err != nil {
		return err
	}
	id, err := auth.NewTokenID()
	if err != nil {
		return err
	}
	exp, _ := auth.ParseDuration(tokenExpires)
	t := &store.Token{
		ID: id, Label: tokenLabel, Hash: hr.Encoded,
		Rate: "60/min", ExpiresAt: exp, CreatedAt: time.Now().UTC(),
	}
	if err := db.InsertToken(t); err != nil {
		return err
	}

	base := "http://127.0.0.1:8080"
	if domain != "" {
		base = "https://" + domain
	}

	if output.JSON {
		return output.PrintJSON(map[string]any{
			"domain":   domain,
			"base_url": base,
			"route":    routeName,
			"token_id": id,
			"token":    plain,
			"notice":   "store token now; shown once",
		})
	}

	fmt.Println()
	output.Success("Your endpoint is ready 🎉")
	fmt.Println()
	fmt.Printf("    Base URL:  %s\n", base)
	fmt.Printf("    Auth:      Authorization: Bearer %s\n", plain)
	fmt.Println()
	fmt.Println("  Token (copy now, shown once):")
	fmt.Printf("    %s\n", plain)
	fmt.Println()
	fmt.Println("  Next:")
	fmt.Println("    eco start -d")
	fmt.Println("    # point Caddy at 127.0.0.1:8080 (see deploy/Caddyfile)")
	fmt.Println("    # client: OPENAI_BASE_URL=" + base + " OPENAI_API_KEY=<token>")
	fmt.Println()
	return nil
}

func writeCaddySnippet(domain string, port int) (string, error) {
	if port <= 0 {
		port = 8080
	}
	path := config.DataDir() + "/Caddyfile.snippet"
	content := fmt.Sprintf(`# Generated by eco init — copy into /etc/caddy/Caddyfile
%s {
	encode zstd gzip
	reverse_proxy 127.0.0.1:%d
	header {
		Strict-Transport-Security "max-age=31536000; includeSubDomains"
		X-Content-Type-Options "nosniff"
		-Server
	}
	request_body {
		max_size 10MB
	}
}
`, domain, port)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", err
	}
	return path, nil
}
