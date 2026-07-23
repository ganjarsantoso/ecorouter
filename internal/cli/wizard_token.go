package cli

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/ganjar/ecorouter/internal/auth"
	"github.com/ganjar/ecorouter/internal/config"
	"github.com/ganjar/ecorouter/internal/output"
	"github.com/ganjar/ecorouter/internal/store"
	"github.com/ganjar/ecorouter/internal/tui"
)

func runTokenMenu() error {
	for {
		var choice string
		if err := tui.SelectString("🎫  Tokens", "Manage client Bearer tokens.",
			[]huh.Option[string]{
				huh.NewOption("➕  Create a new token", "new"),
				huh.NewOption("📋  List tokens", "list"),
				huh.NewOption("🔄  Rotate a token", "rotate"),
				huh.NewOption("🚫  Revoke a token", "revoke"),
				huh.NewOption("↩️   Back", "back"),
			}, &choice); err != nil {
			return err
		}
		switch choice {
		case "new":
			_ = runTokenNewWizard()
		case "list":
			_ = newTokenListCmd().RunE(newTokenListCmd(), nil)
		case "rotate":
			_ = runTokenRotateWizard()
		case "revoke":
			_ = runTokenRevokeWizard()
		case "back", "":
			return nil
		}
	}
}

func runTokenNewWizard() error {
	if err := tui.RequireTTY(); err != nil {
		return err
	}
	cfg, err := requireConfig()
	if err != nil {
		return err
	}

	var label string
	if err := tui.Input("Who is this token for?",
		"A label helps you identify it later. Not shared with the client.",
		"e.g. alice-laptop, ci, staging",
		&label, func(s string) error {
			if strings.TrimSpace(s) == "" {
				return fmt.Errorf("label required")
			}
			return nil
		}); err != nil {
		return err
	}
	label = strings.TrimSpace(label)

	// Route scope
	route := ""
	if len(cfg.Routes) > 0 {
		opts := []huh.Option[string]{huh.NewOption("🌐  Any route (unrestricted)", "")}
		for r := range cfg.Routes {
			opts = append(opts, huh.NewOption(r, r))
		}
		_ = tui.SelectString("Which route can this token use?", "", opts, &route)
	}

	// Rate limit
	rate := "60/min"
	rateOpts := []huh.Option[string]{
		huh.NewOption("Light      — 30/min", "30/min"),
		huh.NewOption("Normal     — 60/min", "60/min"),
		huh.NewOption("Heavy      — 120/min", "120/min"),
		huh.NewOption("Very heavy — 600/min", "600/min"),
		huh.NewOption("Custom…", "__custom__"),
	}
	_ = tui.SelectString("Rate limit", "How many requests per minute?", rateOpts, &rate)
	if rate == "__custom__" {
		var custom string
		_ = tui.Input("Custom rate", "Format: N/min or N/s or N/h", "e.g. 200/min", &custom,
			func(s string) error {
				if _, _, err := config.ParseRate(s); err != nil {
					return err
				}
				return nil
			})
		rate = custom
	}

	// Daily cap
	var capChoice string
	capOpts := []huh.Option[string]{
		huh.NewOption("💰  No cap", "0"),
		huh.NewOption("💵  $1 / day", "1"),
		huh.NewOption("💵  $5 / day", "5"),
		huh.NewOption("💵  $10 / day", "10"),
		huh.NewOption("💵  $25 / day", "25"),
		huh.NewOption("Custom…", "__custom__"),
	}
	_ = tui.SelectString("Daily spend cap", "", capOpts, &capChoice)
	dailyCap := parseFloat(capChoice)
	if capChoice == "__custom__" {
		var s string
		_ = tui.Input("Custom cap (USD/day)", "", "e.g. 12.50", &s, nil)
		dailyCap = parseFloat(s)
	}

	// Concurrency
	var conc string
	_ = tui.SelectString("Max concurrent requests", "",
		[]huh.Option[string]{
			huh.NewOption("No cap", "0"),
			huh.NewOption("1", "1"),
			huh.NewOption("2", "2"),
			huh.NewOption("4", "4"),
			huh.NewOption("8", "8"),
			huh.NewOption("Custom…", "__custom__"),
		}, &conc)
	if conc == "__custom__" {
		_ = tui.Input("Custom concurrency", "", "", &conc, nil)
	}
	maxConcurrent, _ := strconv.Atoi(conc)

	// Expiry
	var expiry string
	_ = tui.SelectString("Expiry", "",
		[]huh.Option[string]{
			huh.NewOption("⏳  Never", ""),
			huh.NewOption("📅  30 days", "30d"),
			huh.NewOption("📅  90 days", "90d"),
			huh.NewOption("📅  1 year", "365d"),
			huh.NewOption("Custom…", "__custom__"),
		}, &expiry)
	if expiry == "__custom__" {
		_ = tui.Input("Custom expiry", "e.g. 45d, 12h, 6m", "", &expiry, nil)
	}

	// Model scope (optional)
	var models string
	if route != "" {
		if r, ok := cfg.Routes[route]; ok && len(r.Models) > 0 {
			scopeChoice := "all"
			_ = tui.SelectString("Model scope", "",
				[]huh.Option[string]{
					huh.NewOption("All models on that route", "all"),
					huh.NewOption("Restrict to a subset", "subset"),
				}, &scopeChoice)
			if scopeChoice == "subset" {
				opts := make([]huh.Option[string], 0, len(r.Models))
				for _, m := range r.Models {
					opts = append(opts, huh.NewOption(m, m))
				}
				var chosen []string
				_ = tui.MultiSelect("Allowed models", "Space to toggle.", opts, &chosen)
				models = strings.Join(chosen, ",")
			}
		}
	}

	return createTokenAndPrint(label, route, rate, dailyCap, maxConcurrent, expiry, models)
}

func createTokenAndPrint(label, route, rate string, dailyCap float64, maxConcurrent int, expires, models string) error {
	if err := config.EnsureDirs(); err != nil {
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
	exp, err := auth.ParseDuration(expires)
	if err != nil {
		return exitErr(2, err)
	}
	t := &store.Token{
		ID:            id,
		Label:         label,
		Hash:          hr.Encoded,
		ScopeRoute:    route,
		ScopeModels:   splitModels(models),
		Rate:          rate,
		DailyCap:      dailyCap,
		MaxConcurrent: maxConcurrent,
		ExpiresAt:     exp,
		CreatedAt:     time.Now().UTC(),
	}
	if err := db.InsertToken(t); err != nil {
		return err
	}
	_ = db.InsertAudit("token_create", "label="+label, "", id)

	output.Success(fmt.Sprintf("Token created for %q (id %s).", label, id))
	fmt.Println()
	fmt.Println("  ┌────────────────────────────────────────────────────────────┐")
	fmt.Printf("  │  %s\n", plain)
	fmt.Println("  │  ← copy now, shown once")
	fmt.Println("  └────────────────────────────────────────────────────────────┘")
	fmt.Println()
	fmt.Println("  Auth:  Authorization: Bearer <token>")
	if dailyCap > 0 {
		fmt.Printf("  Daily spend cap: $%.2f\n", dailyCap)
	}
	if maxConcurrent > 0 {
		fmt.Printf("  Max concurrent:  %d\n", maxConcurrent)
	}

	argsList := []string{fmt.Sprintf("%q", label)}
	if route != "" {
		argsList = append(argsList, "--route "+route)
	}
	if models != "" {
		argsList = append(argsList, "--models "+models)
	}
	if rate != "" {
		argsList = append(argsList, "--rate "+rate)
	}
	if dailyCap > 0 {
		argsList = append(argsList, fmt.Sprintf("--daily-cap %.2f", dailyCap))
	}
	if maxConcurrent > 0 {
		argsList = append(argsList, fmt.Sprintf("--concurrency %d", maxConcurrent))
	}
	if expires != "" {
		argsList = append(argsList, "--expires "+expires)
	}
	tui.PrintEquivalent("eco token new", argsList)
	return nil
}

func parseFloat(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" || s == "__custom__" {
		return 0
	}
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
}

func pickToken(title string) (string, error) {
	db, err := store.Open("")
	if err != nil {
		return "", err
	}
	defer db.Close()
	tokens, err := db.ListTokens()
	if err != nil {
		return "", err
	}
	var active []store.Token
	for _, t := range tokens {
		if !t.Revoked {
			active = append(active, t)
		}
	}
	if len(active) == 0 {
		output.Info("No active tokens.")
		return "", nil
	}
	opts := make([]huh.Option[string], 0, len(active))
	for _, t := range active {
		opts = append(opts, huh.NewOption(fmt.Sprintf("%s  (%s)", t.Label, t.ID), t.ID))
	}
	var id string
	err = tui.SelectString(title, "", opts, &id)
	return id, err
}

func runTokenRotateWizard() error {
	if err := tui.RequireTTY(); err != nil {
		return err
	}
	id, err := pickToken("Rotate which token?")
	if err != nil || id == "" {
		return err
	}
	ok, _ := tui.Confirm("Rotate token "+id+"?", "The old secret becomes invalid immediately.", false)
	if !ok {
		return nil
	}
	return newTokenRotateCmd().RunE(newTokenRotateCmd(), []string{id})
}

func runTokenRevokeWizard() error {
	if err := tui.RequireTTY(); err != nil {
		return err
	}
	id, err := pickToken("Revoke which token?")
	if err != nil || id == "" {
		return err
	}
	ok, _ := tui.Confirm("Revoke token "+id+"?", "This cannot be undone. The client will lose access immediately.", false)
	if !ok {
		return nil
	}
	return newTokenRevokeCmd().RunE(newTokenRevokeCmd(), []string{id})
}
