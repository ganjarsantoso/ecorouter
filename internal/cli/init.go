package cli

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/ganjar/ecorouter/internal/auth"
	"github.com/ganjar/ecorouter/internal/config"
	"github.com/ganjar/ecorouter/internal/output"
	"github.com/ganjar/ecorouter/internal/secrets"
	"github.com/ganjar/ecorouter/internal/store"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func newInitCmd() *cobra.Command {
	var nonInteractive bool
	var domain, providerType, providerKey, routeName, routeMode, routeModels, tokenLabel, tokenExpires string
	cmd := &cobra.Command{
		Use:   "init",
		Short: "First-run wizard: domain, provider, route, token",
		Long:  `Interactive setup for EcoRouter. Also fully scriptable via flags for automated provisioning.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := config.EnsureDirs(); err != nil {
				return err
			}
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			reader := bufio.NewReader(os.Stdin)
			ask := func(prompt, def string) string {
				if nonInteractive {
					return def
				}
				if def != "" {
					fmt.Printf("  %s [%s]: ", prompt, def)
				} else {
					fmt.Printf("  %s: ", prompt)
				}
				line, _ := reader.ReadString('\n')
				line = strings.TrimSpace(line)
				if line == "" {
					return def
				}
				return line
			}
			yesNo := func(prompt string, defYes bool) bool {
				if nonInteractive {
					return defYes
				}
				d := "Y/n"
				if !defYes {
					d = "y/N"
				}
				fmt.Printf("  %s [%s]: ", prompt, d)
				line, _ := reader.ReadString('\n')
				line = strings.TrimSpace(strings.ToLower(line))
				if line == "" {
					return defYes
				}
				return line == "y" || line == "yes"
			}

			if !output.Quiet && !nonInteractive {
				fmt.Println()
				fmt.Println("  Welcome to EcoRouter 👋  Let's put a secure endpoint online.")
				fmt.Println()
			}

			// 1 domain
			if !output.Quiet && !nonInteractive {
				fmt.Println("  1/6  Public domain")
			}
			if domain == "" {
				domain = ask("Domain that points to this host (A record)", cfg.Server.Domain)
			}
			cfg.Server.Domain = domain
			cfg.Server.Host = "127.0.0.1"
			if domain != "" {
				if ips, err := net.LookupHost(domain); err == nil && len(ips) > 0 {
					output.Success("DNS resolves to " + strings.Join(ips, ", "))
				} else {
					output.Warn("DNS does not resolve yet — configure A record before TLS.")
				}
			}

			// 2 TLS note + write local Caddyfile snippet into data dir
			if !output.Quiet && !nonInteractive {
				fmt.Println()
				fmt.Println("  2/6  TLS")
				fmt.Println("       EcoRouter binds loopback only. Install Caddy for HTTPS:")
				fmt.Println("         see deploy/Caddyfile and deploy/ecorouter.service")
				_ = yesNo("Continue (TLS via Caddy is documented, not auto-installed here)?", true)
				output.Success("Firewall reminder: open only 443 and 22. 8080 stays loopback.")
			}
			if domain != "" {
				if path, err := writeCaddySnippet(domain, cfg.Server.Port); err == nil {
					output.Info("  Caddyfile snippet: " + path)
				}
			}

			// 3 provider
			if !output.Quiet && !nonInteractive {
				fmt.Println()
				fmt.Println("  3/6  Add your first provider")
			}
			if providerType == "" {
				providerType = ask("Type (openai / anthropic / ollama)", "openai")
			}
			providerType = strings.ToLower(providerType)
			name := providerType
			if _, exists := cfg.Providers[name]; !exists {
				baseURL := ""
				switch providerType {
				case "openai":
					baseURL = "https://api.openai.com/v1"
				case "anthropic":
					baseURL = "https://api.anthropic.com/v1"
				case "ollama":
					baseURL = "http://127.0.0.1:11434/v1"
				default:
					return exitErr(2, fmt.Errorf("unknown provider type %q", providerType))
				}
				key := providerKey
				if key == "" && providerType != "ollama" && !nonInteractive {
					fmt.Print("  API key (hidden): ")
					key = readSecret()
					fmt.Println()
				}
				if key == "" {
					if providerType == "anthropic" {
						key = os.Getenv("ANTHROPIC_API_KEY")
					} else {
						key = os.Getenv("OPENAI_API_KEY")
					}
				}
				sec, err := secrets.Load("")
				if err != nil {
					return err
				}
				if key != "" {
					_ = sec.Set(name, key)
				}
				models, testErr := fetchModels(providerType, baseURL, key)
				cfg.Providers[name] = config.ProviderConfig{Type: providerType, BaseURL: baseURL, Models: models}
				if testErr != nil {
					output.Warn(fmt.Sprintf("Provider verify failed: %v (saved anyway)", testErr))
				} else {
					output.Success(fmt.Sprintf("Verified — %d models available.", len(models)))
				}
			} else {
				output.Info("Provider " + name + " already configured.")
			}

			// 4 profiles skip
			if !output.Quiet && !nonInteractive {
				fmt.Println()
				fmt.Println("  4/6  Model profiles")
				if yesNo("Import evidence-based model profiles? (stub in v0.1)", false) {
					output.Info("  Profile import deferred — use eco models --refresh for catalogs.")
				}
			}

			// 5 route
			if !output.Quiet && !nonInteractive {
				fmt.Println()
				fmt.Println("  5/6  Create your first route")
			}
			if routeName == "" {
				routeName = ask("Name", "default")
			}
			if routeMode == "" {
				routeMode = ask("Mode (single / fallback / round)", "fallback")
			}
			if routeModels == "" {
				defModels := "gpt-4o-mini"
				if p, ok := cfg.Providers[name]; ok && len(p.Models) > 0 {
					if len(p.Models) >= 2 {
						defModels = p.Models[0] + "," + p.Models[1]
					} else {
						defModels = p.Models[0]
					}
				}
				routeModels = ask("Models (comma-separated)", defModels)
			}
			models := splitModels(routeModels)
			rc := config.RouteConfig{Mode: strings.ToLower(routeMode), Models: models}
			if rc.Mode == "single" && len(models) > 0 {
				rc.Models = models[:1]
			}
			cfg.Routes[routeName] = rc
			cfg.Defaults.ActiveRoute = routeName
			output.Success(fmt.Sprintf("Route %q created and set active.", routeName))

			// 6 token
			if !output.Quiet && !nonInteractive {
				fmt.Println()
				fmt.Println("  6/6  Issue an access token")
			}
			if tokenLabel == "" {
				tokenLabel = ask("Label", "my-laptop")
			}
			if tokenExpires == "" && !nonInteractive {
				tokenExpires = ask("Expiry (blank = never)", "90d")
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
		},
	}
	cmd.Flags().BoolVar(&nonInteractive, "yes", false, "non-interactive (use flags/env)")
	cmd.Flags().StringVar(&domain, "domain", "", "public domain")
	cmd.Flags().StringVar(&providerType, "provider-type", "", "openai|anthropic|ollama")
	cmd.Flags().StringVar(&providerKey, "provider-key", "", "provider API key")
	cmd.Flags().StringVar(&routeName, "route-name", "default", "first route name")
	cmd.Flags().StringVar(&routeMode, "route-mode", "fallback", "single|fallback|round")
	cmd.Flags().StringVar(&routeModels, "route-models", "gpt-4o-mini", "comma-separated models")
	cmd.Flags().StringVar(&tokenLabel, "token-label", "default", "first token label")
	cmd.Flags().StringVar(&tokenExpires, "token-expires", "", "token expiry e.g. 90d")
	return cmd
}

func readSecret() string {
	if term.IsTerminal(int(syscall.Stdin)) {
		b, err := term.ReadPassword(int(syscall.Stdin))
		if err == nil {
			return strings.TrimSpace(string(b))
		}
	}
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	return strings.TrimSpace(line)
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
