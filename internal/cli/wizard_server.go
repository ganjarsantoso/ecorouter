package cli

import (
	"github.com/charmbracelet/huh"
	"github.com/ganjar/ecorouter/internal/tui"
)

func runServerMenu() error {
	for {
		var choice string
		if err := tui.SelectString("▶️  Server", "Start, stop, and inspect the daemon.",
			[]huh.Option[string]{
				huh.NewOption("▶️   Start (background)", "start"),
				huh.NewOption("⏹   Stop", "stop"),
				huh.NewOption("🔄  Restart", "restart"),
				huh.NewOption("📜  View logs (last 100 lines)", "logs"),
				huh.NewOption("📡  Follow logs (Ctrl+C to stop)", "follow"),
				huh.NewOption("📊  Status", "status"),
				huh.NewOption("↩️   Back", "back"),
			}, &choice); err != nil {
			return err
		}
		switch choice {
		case "start":
			cmd := newStartCmd()
			_ = cmd.Flags().Set("detach", "true")
			_ = cmd.RunE(cmd, nil)
		case "stop":
			_ = newStopCmd().RunE(newStopCmd(), nil)
		case "restart":
			_ = newRestartCmd().RunE(newRestartCmd(), nil)
		case "logs":
			_ = newLogsCmd().RunE(newLogsCmd(), nil)
		case "follow":
			cmd := newLogsCmd()
			_ = cmd.Flags().Set("follow", "true")
			_ = cmd.RunE(cmd, nil)
		case "status":
			_ = newStatusCmd().RunE(newStatusCmd(), nil)
		case "back", "":
			return nil
		}
	}
}
