package cli

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ganjar/ecorouter/internal/config"
	"github.com/ganjar/ecorouter/internal/output"
	"github.com/ganjar/ecorouter/internal/secrets"
	"github.com/ganjar/ecorouter/internal/store"
	"github.com/spf13/cobra"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Daemon status, domain, active route, saver default",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := requireConfig()
			if err != nil {
				return err
			}
			pid, running := readPID()
			alive := running && processAlive(pid)
			if running && !alive {
				alive = false
				pid = 0
			}
			// healthz probe
			healthOK := false
			if alive {
				url := fmt.Sprintf("http://%s:%d/healthz", cfg.Server.Host, cfg.Server.Port)
				client := &http.Client{Timeout: 2 * time.Second}
				if resp, err := client.Get(url); err == nil {
					healthOK = resp.StatusCode == 200
					_ = resp.Body.Close()
				}
			}
			info := map[string]any{
				"running":       alive,
				"pid":           pid,
				"healthz":       healthOK,
				"listen":        fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
				"domain":        cfg.Server.Domain,
				"active_route":  cfg.Defaults.ActiveRoute,
				"saver_default": cfg.Defaults.SaverDefault,
				"providers":     len(cfg.Providers),
				"routes":        len(cfg.Routes),
				"config":        cfg.Path(),
				"data_dir":      config.DataDir(),
			}
			if output.JSON {
				return output.PrintJSON(info)
			}
			if alive {
				output.Success(fmt.Sprintf("Daemon running (pid %d) on %s:%d", pid, cfg.Server.Host, cfg.Server.Port))
			} else {
				output.Fail("Daemon is not running.", "eco start -d")
			}
			if cfg.Server.Domain != "" {
				output.Info("  Domain:       https://" + cfg.Server.Domain)
			} else {
				output.Info("  Domain:       (not set)")
			}
			output.Info("  Active route: " + emptyDash(cfg.Defaults.ActiveRoute))
			output.Info("  Saver default:" + " " + emptyDash(cfg.Defaults.SaverDefault))
			output.Info(fmt.Sprintf("  Providers:    %d", len(cfg.Providers)))
			output.Info(fmt.Sprintf("  Routes:       %d", len(cfg.Routes)))
			if healthOK {
				output.Info("  Health:       " + output.Green("ok"))
			} else if alive {
				output.Info("  Health:       " + output.Red("unreachable"))
			}
			return nil
		},
	}
}

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose config, connectivity, and security posture",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := requireConfig()
			if err != nil {
				return err
			}
			type check struct {
				Name   string `json:"name"`
				OK     bool   `json:"ok"`
				Detail string `json:"detail"`
				Fix    string `json:"fix,omitempty"`
			}
			var checks []check

			// config writable / data dir
			if err := config.EnsureDirs(); err != nil {
				checks = append(checks, check{"data_dir", false, err.Error(), "mkdir " + config.DataDir()})
			} else {
				checks = append(checks, check{"data_dir", true, config.DataDir(), ""})
			}

			// loopback binding
			if cfg.Server.Host != "127.0.0.1" && cfg.Server.Host != "localhost" && cfg.Server.Host != "::1" {
				checks = append(checks, check{
					"loopback_bind", false,
					fmt.Sprintf("host is %s — daemon must be loopback-only", cfg.Server.Host),
					"set server.host = \"127.0.0.1\" in config",
				})
			} else {
				checks = append(checks, check{"loopback_bind", true, cfg.Server.Host, ""})
			}

			// port open on loopback?
			addr := net.JoinHostPort(cfg.Server.Host, strconv.Itoa(cfg.Server.Port))
			ln, err := net.Listen("tcp", addr)
			if err != nil {
				// in use — good if our daemon
				pid, ok := readPID()
				if ok && processAlive(pid) {
					checks = append(checks, check{"port", true, fmt.Sprintf("%s in use by eco (pid %d)", addr, pid), ""})
				} else {
					checks = append(checks, check{"port", false, fmt.Sprintf("%s in use by another process", addr), "eco stop or eco start --port 8090"})
				}
			} else {
				_ = ln.Close()
				checks = append(checks, check{"port", true, addr + " free (daemon not running)", "eco start -d"})
			}

			// domain / DNS hint
			if cfg.Server.Domain == "" {
				checks = append(checks, check{"domain", false, "no public domain set", "eco start --domain eco.you.dev  or eco init"})
			} else {
				ips, err := net.LookupHost(cfg.Server.Domain)
				if err != nil {
					checks = append(checks, check{"domain", false, "DNS lookup failed for " + cfg.Server.Domain, "point A record to this host"})
				} else {
					checks = append(checks, check{"domain", true, fmt.Sprintf("%s → %s", cfg.Server.Domain, strings.Join(ips, ", ")), ""})
				}
			}

			// providers
			sec, _ := secrets.Load("")
			if len(cfg.Providers) == 0 {
				checks = append(checks, check{"providers", false, "no providers configured", "eco provider add openai --key $KEY"})
			} else {
				for name, p := range cfg.Providers {
					key, has := sec.Get(name)
					if p.Type != "ollama" && !has {
						checks = append(checks, check{"provider:" + name, false, "missing API key", "eco provider add " + name + " --key $KEY"})
						continue
					}
					_, err := fetchModels(p.Type, p.BaseURL, key)
					if err != nil {
						checks = append(checks, check{"provider:" + name, false, err.Error(), "eco provider test " + name})
					} else {
						checks = append(checks, check{"provider:" + name, true, p.BaseURL, ""})
					}
				}
			}

			// routes
			if len(cfg.Routes) == 0 {
				checks = append(checks, check{"routes", false, "no routes", "eco route add default --single gpt-4o-mini"})
			} else if cfg.Defaults.ActiveRoute == "" {
				checks = append(checks, check{"routes", false, "no active route", "eco use <route>"})
			} else {
				checks = append(checks, check{"routes", true, "active=" + cfg.Defaults.ActiveRoute, ""})
			}

			// tokens
			db, err := store.Open("")
			if err != nil {
				checks = append(checks, check{"tokens_db", false, err.Error(), ""})
			} else {
				toks, _ := db.ListTokens()
				active := 0
				for _, t := range toks {
					if !t.Revoked {
						active++
					}
				}
				if active == 0 {
					checks = append(checks, check{"tokens", false, "no active tokens", "eco token new \"my-laptop\""})
				} else {
					checks = append(checks, check{"tokens", true, fmt.Sprintf("%d active", active), ""})
				}
				_ = db.Close()
			}

			// savers
			for name, s := range cfg.Savers {
				if err := probeTCP(s.URL); err != nil {
					checks = append(checks, check{"saver:" + name, false, "unreachable " + s.URL, "start saver or eco saver test " + name})
				} else {
					checks = append(checks, check{"saver:" + name, true, s.URL, ""})
				}
			}

			// caddy presence (optional)
			if _, err := os.Stat("/etc/caddy/Caddyfile"); err == nil {
				checks = append(checks, check{"caddy", true, "/etc/caddy/Caddyfile present", ""})
			} else {
				checks = append(checks, check{"caddy", true, "Caddyfile not found (ok for local dev)", "see deploy/Caddyfile for production"})
			}

			if output.JSON {
				return output.PrintJSON(checks)
			}
			fail := 0
			for _, c := range checks {
				if c.OK {
					output.Success(fmt.Sprintf("%s — %s", c.Name, c.Detail))
				} else {
					fail++
					output.Fail(fmt.Sprintf("%s — %s", c.Name, c.Detail), c.Fix)
				}
			}
			fmt.Println()
			if fail == 0 {
				output.Success("All checks passed.")
			} else {
				output.Warn(fmt.Sprintf("%d check(s) need attention.", fail))
			}
			return nil
		},
	}
}
