package cli

import (
	"fmt"

	"github.com/charmbracelet/huh"
	"github.com/ganjar/ecorouter/internal/output"
	"github.com/ganjar/ecorouter/internal/tui"
	"github.com/spf13/cobra"
)

// runMainMenu is invoked when `eco` is run with no subcommand.
// On non-TTY it returns nil so cobra prints help instead.
func runMainMenu(root *cobra.Command) error {
	if !tui.IsInteractive() {
		return root.Help()
	}

	fmt.Println()
	fmt.Println("  🌿  EcoRouter — self-hosted LLM router")
	fmt.Println("      Arrow keys to move · Enter to select · Ctrl+C to quit")
	fmt.Println()

	for {
		var choice string
		err := tui.SelectString(
			"🌿  EcoRouter",
			"What would you like to do?",
			[]huh.Option[string]{
				huh.NewOption("✨  Set up EcoRouter (first-run wizard)", "init"),
				huh.NewOption("🔌  Providers — add, edit, list, test, remove", "providers"),
				huh.NewOption("🛣️   Routes — add, edit, list, test, remove", "routes"),
				huh.NewOption("🎫  Tokens — create, list, rotate, revoke", "tokens"),
				huh.NewOption("💾  Savers — add, list, set default, remove", "savers"),
				huh.NewOption("🛡️   Access & limits — CIDR rules, global caps", "access"),
				huh.NewOption("▶️   Start / stop the router", "server"),
				huh.NewOption("📊  Activity & stats", "activity"),
				huh.NewOption("🩺  Run a health check (doctor)", "doctor"),
				huh.NewOption("🚪  Exit", "exit"),
			},
			&choice,
		)
		if err != nil {
			return err
		}
		switch choice {
		case "init":
			if err := runInitWizard(); err != nil && err != tui.ErrNotInteractive {
				output.Warn(err.Error())
			}
		case "providers":
			if err := runProviderMenu(); err != nil {
				output.Warn(err.Error())
			}
		case "routes":
			if err := runRouteMenu(); err != nil {
				output.Warn(err.Error())
			}
		case "tokens":
			if err := runTokenMenu(); err != nil {
				output.Warn(err.Error())
			}
		case "savers":
			if err := runSaverMenu(); err != nil {
				output.Warn(err.Error())
			}
		case "access":
			if err := runAccessMenu(); err != nil {
				output.Warn(err.Error())
			}
		case "server":
			if err := runServerMenu(); err != nil {
				output.Warn(err.Error())
			}
		case "activity":
			if err := runActivityMenu(); err != nil {
				output.Warn(err.Error())
			}
		case "doctor":
			doctor := newDoctorCmd()
			if err := doctor.RunE(doctor, nil); err != nil {
				output.Warn(err.Error())
			}
		case "exit", "":
			fmt.Println()
			output.Info("Goodbye 👋")
			return nil
		}
		fmt.Println()
	}
}
