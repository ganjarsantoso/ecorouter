package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/ganjar/ecorouter/internal/config"
	"github.com/ganjar/ecorouter/internal/output"
	"github.com/ganjar/ecorouter/internal/secrets"
	"github.com/ganjar/ecorouter/internal/server"
	"github.com/ganjar/ecorouter/internal/store"
	"github.com/ganjar/ecorouter/internal/tui"
	"github.com/spf13/cobra"
)

func newStartCmd() *cobra.Command {
	var detached bool
	var port int
	var domain string
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the eco daemon (loopback only)",
		Long: `Start the eco daemon on loopback. Use --detach to run in the background.

💡 Use --wizard to be prompted for port and public domain.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := config.EnsureDirs(); err != nil {
				return err
			}
			// already running? (ignore our own pid if re-exec)
			if pid, ok := readPID(); ok && pid != os.Getpid() && processAlive(pid) {
				return exitErr(5, fmt.Errorf("daemon already running (pid %d); eco stop first", pid))
			}

			cfg, err := requireConfig()
			if err != nil {
				return err
			}
			force := WizardRequested()

			// Optional interactive domain prompt when unset (or in wizard)
			if (cfg.Server.Domain == "" && tui.IsInteractive() && !force) || force {
				want, _ := tui.Confirm("Set a public domain now?",
					"Required for public HTTPS via Caddy. Skip for local-only.",
					false)
				if want {
					d, err := askString(domain, "domain",
						"Public domain", "FQDN that points at this host.",
						"e.g. eco.you.dev", force,
						func(s string) error {
							s = strings.TrimSpace(s)
							if s == "" {
								return fmt.Errorf("domain required (or leave empty and skip)")
							}
							return nil
						})
					if err == nil {
						domain = strings.TrimSpace(d)
					}
				}
			}

			// Optional port prompt in wizard
			if force && port == 0 {
				portStr := fmt.Sprintf("%d", cfg.Server.Port)
				if cfg.Server.Port == 0 {
					portStr = "8080"
				}
				ps, err := askString(portStr, "port",
					"Loopback port", "Default 8080.", "8080", force,
					func(s string) error {
						p, err := strconv.Atoi(strings.TrimSpace(s))
						if err != nil || p <= 0 {
							return fmt.Errorf("invalid port %q", s)
						}
						return nil
					})
				if err == nil {
					if p, err := strconv.Atoi(strings.TrimSpace(ps)); err == nil && p > 0 {
						port = p
					}
				}
			}

			changed := false
			if port > 0 && cfg.Server.Port != port {
				cfg.Server.Port = port
				changed = true
			}
			if domain != "" && cfg.Server.Domain != domain {
				cfg.Server.Domain = domain
				changed = true
			}
			// force loopback always
			if cfg.Server.Host == "" || cfg.Server.Host == "0.0.0.0" {
				cfg.Server.Host = "127.0.0.1"
				changed = true
			}
			if changed {
				_ = cfg.Save()
			}

			if detached {
				return startDetached(cfg)
			}

			db, err := store.Open("")
			if err != nil {
				return err
			}
			defer db.Close()
			sec, err := secrets.Load("")
			if err != nil {
				return err
			}
			srv, err := server.New(cfg, db, sec)
			if err != nil {
				return err
			}
			if !output.Quiet {
				output.Info(fmt.Sprintf("Starting eco on %s:%d (loopback)…", cfg.Server.Host, cfg.Server.Port))
				if cfg.Server.Domain != "" {
					output.Info("Public domain: https://" + cfg.Server.Domain + " (via Caddy)")
				}
				output.Info("Press Ctrl+C to stop. Live activity is logged.")
			}
			if err := srv.Run(false); err != nil {
				if strings.Contains(err.Error(), "port in use") {
					return exitErr(6, err)
				}
				return err
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&detached, "detach", "d", false, "run in background")
	cmd.Flags().IntVar(&port, "port", 0, "override loopback port (default 8080)")
	cmd.Flags().StringVar(&domain, "domain", "", "public domain for logging/TLS coordination")
	return cmd
}

func startDetached(cfg *config.Config) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	args := []string{"start", "--port", strconv.Itoa(cfg.Server.Port)}
	if cfgPath != "" {
		args = append([]string{"--config", cfgPath}, args...)
	}
	if cfg.Server.Domain != "" {
		args = append(args, "--domain", cfg.Server.Domain)
	}
	// open log file
	logPath := config.LogPath()
	logF, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	c := exec.Command(exe, args...)
	c.Stdout = logF
	c.Stderr = logF
	setSysProcAttr(c)
	if err := c.Start(); err != nil {
		_ = logF.Close()
		return err
	}
	// Child writes the PID file in server.Run; wait briefly for it.
	childPID := c.Process.Pid
	for i := 0; i < 30; i++ {
		if pid, ok := readPID(); ok && processAlive(pid) {
			childPID = pid
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if output.JSON {
		return output.PrintJSON(map[string]any{"pid": childPID, "log": logPath, "addr": fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)})
	}
	output.Success(fmt.Sprintf("Daemon started in background (pid %d).", childPID))
	output.Info("  Log: " + logPath)
	output.Info(fmt.Sprintf("  Addr: %s:%d", cfg.Server.Host, cfg.Server.Port))
	_ = c.Process.Release()
	_ = logF.Close()
	return nil
}

func newStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the eco daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			pid, ok := readPID()
			if !ok || !processAlive(pid) {
				_ = os.Remove(config.PIDPath())
				return exitErr(5, fmt.Errorf("daemon is not running"))
			}
			if err := sendTermSignal(pid); err != nil {
				return err
			}
			// wait up to 5s
			for i := 0; i < 50; i++ {
				if !processAlive(pid) {
					_ = os.Remove(config.PIDPath())
					if output.JSON {
						return output.PrintJSON(map[string]string{"status": "stopped"})
					}
					output.Success("Daemon stopped.")
					return nil
				}
				time.Sleep(100 * time.Millisecond)
			}
			_ = sendKillSignal(pid)
			_ = os.Remove(config.PIDPath())
			if output.JSON {
				return output.PrintJSON(map[string]string{"status": "killed"})
			}
			output.Warn("Daemon did not exit cleanly; sent SIGKILL.")
			return nil
		},
	}
}

func newRestartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restart",
		Short: "Restart the eco daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = newStopCmd().RunE(cmd, nil)
			time.Sleep(300 * time.Millisecond)
			// start detached
			cfg, err := requireConfig()
			if err != nil {
				return err
			}
			return startDetached(cfg)
		},
	}
}

func readPID() (int, bool) {
	b, err := os.ReadFile(config.PIDPath())
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || pid <= 0 {
		return 0, false
	}
	return pid, true
}

func processAlive(pid int) bool {
	return processAlivePlatform(pid)
}

func newLogsCmd() *cobra.Command {
	var follow bool
	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Show server logs",
		Long: `Show server logs.

💡 Run on a TTY to be prompted: view last 100 lines or follow live.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			path := config.LogPath()
			// If neither --follow set nor flag explicitly disabled, prompt on TTY.
			if !follow && !cmd.Flags().Changed("follow") && tui.IsInteractive() {
				choice := "view"
				if err := tui.SelectString("Logs", "",
					[]huh.Option[string]{
						huh.NewOption("📜  View last 100 lines", "view"),
						huh.NewOption("📡  Follow live (Ctrl+C to stop)", "follow"),
					}, &choice); err == nil {
					follow = choice == "follow"
				}
			}
			if follow {
				// simple tail -f via exec if available
				c := exec.Command("tail", "-n", "50", "-f", path)
				c.Stdout = os.Stdout
				c.Stderr = os.Stderr
				return c.Run()
			}
			b, err := os.ReadFile(path)
			if err != nil {
				if os.IsNotExist(err) {
					output.Info("No log file yet. Start with: eco start -d")
					return nil
				}
				return err
			}
			// last 100 lines
			lines := strings.Split(string(b), "\n")
			start := 0
			if len(lines) > 100 {
				start = len(lines) - 100
			}
			fmt.Print(strings.Join(lines[start:], "\n"))
			return nil
		},
	}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "follow log output")
	return cmd
}

// ensure filepath used
var _ = filepath.Join
