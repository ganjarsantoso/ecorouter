package tui

import (
	"fmt"
	"strings"

	"github.com/ganjar/ecorouter/internal/output"
)

// PrintEquivalent shows the flag-form of what the wizard just did.
// This teaches scriptability by example.
func PrintEquivalent(cmd string, args []string) {
	if !IsInteractive() {
		return
	}
	fmt.Println()
	output.Info(output.Dim("  Equivalent command:"))
	joined := "  " + cmd
	if len(args) > 0 {
		joined += " " + strings.Join(args, " \\\n    ")
	}
	fmt.Println(output.Dim(joined))
	fmt.Println()
}
