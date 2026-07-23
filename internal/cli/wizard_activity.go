package cli

import (
	"fmt"
	"strconv"

	"github.com/charmbracelet/huh"
	"github.com/ganjar/ecorouter/internal/store"
	"github.com/ganjar/ecorouter/internal/tui"
)

func runActivityMenu() error {
	for {
		var choice string
		if err := tui.SelectString("📊  Activity & stats", "Inspect request history and rollups.",
			[]huh.Option[string]{
				huh.NewOption("📋  Recent activity", "activity"),
				huh.NewOption("📈  Stats by group", "stats"),
				huh.NewOption("🔐  Audit log", "audit"),
				huh.NewOption("↩️   Back", "back"),
			}, &choice); err != nil {
			return err
		}
		switch choice {
		case "activity":
			_ = runActivityWizard()
		case "stats":
			_ = runStatsWizard()
		case "audit":
			_ = runAuditWizard()
		case "back", "":
			return nil
		}
	}
}

func runActivityWizard() error {
	if err := tui.RequireTTY(); err != nil {
		return err
	}
	var since string
	_ = tui.SelectString("Time window", "",
		[]huh.Option[string]{
			huh.NewOption("Last hour", "1h"),
			huh.NewOption("Last 24 hours", "24h"),
			huh.NewOption("Last 7 days", "7d"),
			huh.NewOption("All / custom…", "__custom__"),
		}, &since)
	if since == "__custom__" {
		_ = tui.Input("Custom window", "e.g. 2h, 3d, 48h", "24h", &since, nil)
	}

	tokenID := ""
	db, err := store.Open("")
	if err == nil {
		tokens, _ := db.ListTokens()
		_ = db.Close()
		if len(tokens) > 0 {
			opts := []huh.Option[string]{huh.NewOption("All tokens", "")}
			for _, t := range tokens {
				if t.Revoked {
					continue
				}
				opts = append(opts, huh.NewOption(fmt.Sprintf("%s (%s)", t.Label, t.ID), t.ID))
			}
			_ = tui.SelectString("Filter by token?", "", opts, &tokenID)
		}
	}

	cmd := newActivityCmd()
	if since != "" {
		_ = cmd.Flags().Set("since", since)
	}
	if tokenID != "" {
		_ = cmd.Flags().Set("token", tokenID)
	}
	return cmd.RunE(cmd, nil)
}

func runStatsWizard() error {
	if err := tui.RequireTTY(); err != nil {
		return err
	}
	var group, since string
	_ = tui.SelectString("Group by", "",
		[]huh.Option[string]{
			huh.NewOption("Route", "route"),
			huh.NewOption("Model", "model"),
			huh.NewOption("Token", "token"),
			huh.NewOption("Day", "day"),
		}, &group)
	_ = tui.SelectString("Time window", "",
		[]huh.Option[string]{
			huh.NewOption("Last hour", "1h"),
			huh.NewOption("Last 24 hours", "24h"),
			huh.NewOption("Last 7 days", "7d"),
			huh.NewOption("Custom…", "__custom__"),
		}, &since)
	if since == "__custom__" {
		_ = tui.Input("Custom window", "e.g. 2h, 3d", "24h", &since, nil)
	}
	cmd := newStatsCmd()
	_ = cmd.Flags().Set("by", group)
	_ = cmd.Flags().Set("since", since)
	return cmd.RunE(cmd, nil)
}

func runAuditWizard() error {
	if err := tui.RequireTTY(); err != nil {
		return err
	}
	var lim string
	_ = tui.SelectString("How many events?", "",
		[]huh.Option[string]{
			huh.NewOption("25", "25"),
			huh.NewOption("50", "50"),
			huh.NewOption("100", "100"),
			huh.NewOption("Custom…", "__custom__"),
		}, &lim)
	if lim == "__custom__" {
		_ = tui.Input("Limit", "", "50", &lim, nil)
	}
	if _, err := strconv.Atoi(lim); err != nil {
		lim = "50"
	}
	cmd := newAuditCmd()
	_ = cmd.Flags().Set("limit", lim)
	return cmd.RunE(cmd, nil)
}
