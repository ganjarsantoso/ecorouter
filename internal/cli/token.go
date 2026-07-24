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
	"github.com/spf13/cobra"
)

func newTokenCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "token",
		Short: "Manage client Bearer tokens",
		Long:  `Manage client Bearer tokens. Each token is shown ONCE on creation — copy it immediately.`,
	}
	cmd.AddCommand(
		newTokenNewCmd(),
		newTokenListCmd(),
		newTokenRotateCmd(),
		newTokenRevokeCmd(),
		newTokenScopeCmd(),
	)
	return cmd
}

func newTokenNewCmd() *cobra.Command {
	var route, models, expires, rate string
	var dailyCap float64
	var maxConcurrent int
	cmd := &cobra.Command{
		Use:   "new [label]",
		Short: "Generate a Bearer token (shown once)",
		Long: `Generate a new Bearer token. The plaintext token is printed ONCE.

💡 Run with no arguments (or --wizard) to be guided step-by-step.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := config.EnsureDirs(); err != nil {
				return err
			}
			cfg, err := requireConfig()
			if err != nil {
				return err
			}
			force := WizardRequested()

			label := ""
			if len(args) == 1 {
				label = args[0]
			}

			// 1. label
			label, err = askString(label, "label",
				"Who is this token for?",
				"A label helps you identify it later. Not shared with the client.",
				"e.g. alice-laptop, ci, staging", force,
				func(s string) error {
					if strings.TrimSpace(s) == "" {
						return fmt.Errorf("label required")
					}
					return nil
				})
			if err != nil {
				return err
			}
			label = strings.TrimSpace(label)

			// 2. route scope
			routeOpts := []huh.Option[string]{huh.NewOption("🌐  Any route (unrestricted)", "")}
			for _, o := range routeOptions(cfg) {
				routeOpts = append(routeOpts, o)
			}
			route, err = askPick(route, "route",
				"Which route can this token use?", "", routeOpts, force)
			if err != nil {
				return err
			}

			// 3. rate limit (choice + custom)
			if rate == "" {
				rate = "60/min"
			}
			rateChoice, err := askChoice(rate, "rate",
				"Rate limit", "How many requests per minute?",
				[]huh.Option[string]{
					huh.NewOption("Light      — 30/min", "30/min"),
					huh.NewOption("Normal     — 60/min", "60/min"),
					huh.NewOption("Heavy      — 120/min", "120/min"),
					huh.NewOption("Very heavy — 600/min", "600/min"),
					huh.NewOption("Custom…", "__custom__"),
				}, force)
			if err != nil {
				return err
			}
			if rateChoice == "__custom__" {
				rate, err = askString("", "rate", "Custom rate",
					"Format: N/min or N/s or N/h", "e.g. 200/min", force,
					func(s string) error {
						if _, _, err := config.ParseRate(s); err != nil {
							return err
						}
						return nil
					})
				if err != nil {
					return err
				}
			} else {
				rate = rateChoice
			}

			// 4. daily cap (choice + custom). Default = 0 (no cap)
			capStr := ""
			if dailyCap > 0 {
				capStr = strconv.FormatFloat(dailyCap, 'f', -1, 64)
			}
			capChoice, err := askChoice(capStr, "daily-cap",
				"Daily spend cap", "",
				[]huh.Option[string]{
					huh.NewOption("💰  No cap", "0"),
					huh.NewOption("💵  $1 / day", "1"),
					huh.NewOption("💵  $5 / day", "5"),
					huh.NewOption("💵  $10 / day", "10"),
					huh.NewOption("💵  $25 / day", "25"),
					huh.NewOption("Custom…", "__custom__"),
				}, force)
			if err != nil {
				return err
			}
			if capChoice == "__custom__" {
				capStr, err = askString("", "daily-cap", "Custom cap (USD/day)", "", "e.g. 12.50", force, nil)
				if err != nil {
					return err
				}
				dailyCap = parseFloat(capStr)
			} else {
				dailyCap = parseFloat(capChoice)
			}

			// 5. concurrency (choice + custom)
			concStr := ""
			if maxConcurrent > 0 {
				concStr = strconv.Itoa(maxConcurrent)
			}
			concChoice, err := askChoice(concStr, "concurrency",
				"Max concurrent requests", "",
				[]huh.Option[string]{
					huh.NewOption("No cap", "0"),
					huh.NewOption("1", "1"),
					huh.NewOption("2", "2"),
					huh.NewOption("4", "4"),
					huh.NewOption("8", "8"),
					huh.NewOption("Custom…", "__custom__"),
				}, force)
			if err != nil {
				return err
			}
			if concChoice == "__custom__" {
				concStr, err = askString("", "concurrency", "Custom concurrency", "", "e.g. 16", force, nil)
				if err != nil {
					return err
				}
				maxConcurrent, _ = strconv.Atoi(concStr)
			} else {
				maxConcurrent, _ = strconv.Atoi(concChoice)
			}

			// 6. expiry (choice + custom)
			expChoice, err := askChoice(expires, "expires",
				"Expiry", "",
				[]huh.Option[string]{
					huh.NewOption("⏳  Never", ""),
					huh.NewOption("📅  30 days", "30d"),
					huh.NewOption("📅  90 days", "90d"),
					huh.NewOption("📅  1 year", "365d"),
					huh.NewOption("Custom…", "__custom__"),
				}, force)
			if err != nil {
				return err
			}
			if expChoice == "__custom__" {
				expires, err = askString("", "expires", "Custom expiry",
					"e.g. 45d, 12h, 6m", "", force, nil)
				if err != nil {
					return err
				}
			} else {
				expires = expChoice
			}

			// 7. Model scope (optional) — only when route is set and has models
			if models == "" && route != "" {
				if r, ok := cfg.Routes[route]; ok && len(r.Models) > 0 && (force || tui.IsInteractive()) {
					scope := "all"
					if err := tui.SelectString("Model scope", "",
						[]huh.Option[string]{
							huh.NewOption("All models on that route", "all"),
							huh.NewOption("Restrict to a subset", "subset"),
						}, &scope); err == nil && scope == "subset" {
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

			// Insert
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

			if output.JSON {
				_ = output.PrintJSON(map[string]any{
					"id":             id,
					"label":          label,
					"token":          plain,
					"daily_cap":      dailyCap,
					"max_concurrent": maxConcurrent,
					"notice":         "store this token now; it will not be shown again",
				})
				equiv := []string{fmt.Sprintf("%q", label)}
				if route != "" {
					equiv = append(equiv, "--route "+route)
				}
				if models != "" {
					equiv = append(equiv, "--models "+models)
				}
				if rate != "" {
					equiv = append(equiv, "--rate "+rate)
				}
				if dailyCap > 0 {
					equiv = append(equiv, fmt.Sprintf("--daily-cap %.2f", dailyCap))
				}
				if maxConcurrent > 0 {
					equiv = append(equiv, fmt.Sprintf("--concurrency %d", maxConcurrent))
				}
				if expires != "" {
					equiv = append(equiv, "--expires "+expires)
				}
				tui.PrintEquivalent("eco token new", equiv)
				return nil
			}
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
			equiv := []string{fmt.Sprintf("%q", label)}
			if route != "" {
				equiv = append(equiv, "--route "+route)
			}
			if models != "" {
				equiv = append(equiv, "--models "+models)
			}
			if rate != "" {
				equiv = append(equiv, "--rate "+rate)
			}
			if dailyCap > 0 {
				equiv = append(equiv, fmt.Sprintf("--daily-cap %.2f", dailyCap))
			}
			if maxConcurrent > 0 {
				equiv = append(equiv, fmt.Sprintf("--concurrency %d", maxConcurrent))
			}
			if expires != "" {
				equiv = append(equiv, "--expires "+expires)
			}
			tui.PrintEquivalent("eco token new", equiv)
			return nil
		},
	}
	cmd.Flags().StringVar(&route, "route", "", "scope to a single route")
	cmd.Flags().StringVar(&models, "models", "", "comma-separated allowed models")
	cmd.Flags().StringVar(&expires, "expires", "", "expiry e.g. 90d, 24h (default: never)")
	cmd.Flags().StringVar(&rate, "rate", "60/min", "per-token rate limit")
	cmd.Flags().Float64Var(&dailyCap, "daily-cap", 0, "daily USD spend cap (0 = disabled)")
	cmd.Flags().IntVar(&maxConcurrent, "concurrency", 0, "max concurrent requests (0 = unlimited)")
	return cmd
}

func newTokenListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List tokens (never the secret)",
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := store.Open("")
			if err != nil {
				return err
			}
			defer db.Close()
			tokens, err := db.ListTokens()
			if err != nil {
				return err
			}
			type row struct {
				ID          string     `json:"id"`
				Label       string     `json:"label"`
				ScopeRoute  string     `json:"scope_route,omitempty"`
				ScopeModels []string   `json:"scope_models,omitempty"`
				Rate        string     `json:"rate,omitempty"`
				ExpiresAt   *time.Time `json:"expires_at,omitempty"`
				CreatedAt   time.Time  `json:"created_at"`
				LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
				LastIP      string     `json:"last_ip,omitempty"`
				Revoked     bool       `json:"revoked"`
			}
			var rows []row
			var table [][]string
			for _, t := range tokens {
				rows = append(rows, row{
					ID: t.ID, Label: t.Label, ScopeRoute: t.ScopeRoute, ScopeModels: t.ScopeModels,
					Rate: t.Rate, ExpiresAt: t.ExpiresAt, CreatedAt: t.CreatedAt, LastUsedAt: t.LastUsedAt,
					LastIP: t.LastIP, Revoked: t.Revoked,
				})
				status := "active"
				if t.Revoked {
					status = "revoked"
				} else if t.ExpiresAt != nil && time.Now().After(*t.ExpiresAt) {
					status = "expired"
				}
				exp := "never"
				if t.ExpiresAt != nil {
					exp = t.ExpiresAt.Format("2006-01-02")
				}
				last := "—"
				if t.LastUsedAt != nil {
					last = t.LastUsedAt.Format(time.RFC3339)
				}
				table = append(table, []string{t.ID, t.Label, status, t.ScopeRoute, exp, last, t.LastIP})
			}
			if output.JSON {
				return output.PrintJSON(rows)
			}
			if len(table) == 0 {
				output.Info("No tokens. Create one: eco token new \"my-laptop\"")
				return nil
			}
			output.Table([]string{"ID", "LABEL", "STATUS", "ROUTE", "EXPIRES", "LAST USED", "LAST IP"}, table)
			return nil
		},
	}
}

func newTokenRotateCmd() *cobra.Command {
	var assumeYes bool
	cmd := &cobra.Command{
		Use:   "rotate [id]",
		Short: "Issue a new secret; invalidate the old one",
		Long: `Issue a new secret for an existing token. The old secret stops working
immediately.

💡 Run with no arguments to pick from a list.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := store.Open("")
			if err != nil {
				return err
			}
			defer db.Close()
			force := WizardRequested()
			id := ""
			if len(args) == 1 {
				id = args[0]
			}
			opts, err := tokenOptions(db)
			if err != nil {
				return err
			}
			id, err = askPick(id, "id", "Rotate which token?",
				"The old secret will stop working immediately.", opts, force)
			if err != nil {
				return err
			}
			if id == "" {
				return exitErr(1, fmt.Errorf("no token selected"))
			}
			t, err := db.GetToken(id)
			if err != nil {
				return exitErr(1, fmt.Errorf("token %s not found", id))
			}
			ok, err := confirmDestructive(assumeYes,
				"Rotate token "+id+"?",
				"The old secret becomes invalid immediately.")
			if err != nil {
				return err
			}
			if !ok {
				return nil
			}
			plain, err := auth.Generate()
			if err != nil {
				return err
			}
			hr, err := auth.Hash(plain)
			if err != nil {
				return err
			}
			if err := db.UpdateTokenHash(t.ID, hr.Encoded); err != nil {
				return err
			}
			_ = db.InsertAudit("token_rotate", "rotated", "", t.ID)
			if output.JSON {
				_ = output.PrintJSON(map[string]any{"id": t.ID, "token": plain, "notice": "store this token now"})
				tui.PrintEquivalent("eco token rotate", []string{t.ID, "--yes"})
				return nil
			}
			output.Success(fmt.Sprintf("Token %s rotated. Old secret is invalid.", t.ID))
			fmt.Println()
			fmt.Printf("  %s\n", plain)
			fmt.Println("  ← copy now, shown once")
			tui.PrintEquivalent("eco token rotate", []string{t.ID, "--yes"})
			return nil
		},
	}
	cmd.Flags().BoolVarP(&assumeYes, "yes", "y", false, "skip confirmation prompt")
	return cmd
}

func newTokenRevokeCmd() *cobra.Command {
	var assumeYes bool
	cmd := &cobra.Command{
		Use:   "revoke [id]",
		Short: "Instantly revoke a token",
		Long: `Instantly revoke a token. The client will lose access immediately.

💡 Run with no arguments to pick from a list.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := store.Open("")
			if err != nil {
				return err
			}
			defer db.Close()
			force := WizardRequested()
			id := ""
			if len(args) == 1 {
				id = args[0]
			}
			opts, err := tokenOptions(db)
			if err != nil {
				return err
			}
			id, err = askPick(id, "id", "Revoke which token?",
				"This cannot be undone.", opts, force)
			if err != nil {
				return err
			}
			if id == "" {
				return exitErr(1, fmt.Errorf("no token selected"))
			}
			ok, err := confirmDestructive(assumeYes,
				fmt.Sprintf("Revoke %s?", id),
				"This immediately stops the token from working.")
			if err != nil {
				return err
			}
			if !ok {
				return nil
			}
			if err := db.RevokeToken(id); err != nil {
				return exitErr(1, err)
			}
			_ = db.InsertAudit("token_revoke", "revoked", "", id)
			if output.JSON {
				return output.PrintJSON(map[string]string{"revoked": id})
			}
			output.Success(fmt.Sprintf("Token %s revoked.", id))
			tui.PrintEquivalent("eco token revoke", []string{id, "--yes"})
			return nil
		},
	}
	cmd.Flags().BoolVarP(&assumeYes, "yes", "y", false, "skip confirmation prompt")
	return cmd
}

func newTokenScopeCmd() *cobra.Command {
	var route, models string
	cmd := &cobra.Command{
		Use:   "scope [id]",
		Short: "Adjust token scope after creation",
		Long: `Adjust a token's route and model scope after creation.

💡 Run with no arguments to pick from a list.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := store.Open("")
			if err != nil {
				return err
			}
			defer db.Close()
			cfg, err := requireConfig()
			if err != nil {
				return err
			}
			force := WizardRequested()
			id := ""
			if len(args) == 1 {
				id = args[0]
			}
			opts, err := tokenOptions(db)
			if err != nil {
				return err
			}
			id, err = askPick(id, "id", "Adjust scope for which token?", "", opts, force)
			if err != nil {
				return err
			}
			if id == "" {
				return exitErr(1, fmt.Errorf("no token selected"))
			}
			if _, err := db.GetToken(id); err != nil {
				return exitErr(1, fmt.Errorf("token %s not found", id))
			}

			// Route scope
			routeOpts := []huh.Option[string]{huh.NewOption("(no change / cleared)", "")}
			for _, o := range routeOptions(cfg) {
				routeOpts = append(routeOpts, o)
			}
			// If --route flag not provided AND not in wizard, skip; otherwise prompt
			if route != "" || force {
				route, err = askPick(route, "route",
					"Scope token to which route?", "(empty = no scope)",
					routeOpts, force)
				if err != nil {
					return err
				}
			}

			// Model multi-select (optional)
			if models != "" || force {
				all := modelOptions(cfg)
				chosen := splitModels(models)
				if len(all) > 0 {
					if err := tui.MultiSelect(
						"Restrict to which models?",
						"Space to toggle. Empty = no restriction.",
						all, &chosen); err == nil {
						models = strings.Join(chosen, ",")
					}
				}
			}

			if err := db.UpdateTokenScope(id, route, splitModels(models)); err != nil {
				return err
			}
			_ = db.InsertAudit("token_scope", "route="+route+" models="+models, "", id)
			if output.JSON {
				return output.PrintJSON(map[string]any{"id": id, "scope_route": route, "scope_models": splitModels(models)})
			}
			output.Success(fmt.Sprintf("Token %s scope updated.", id))
			return nil
		},
	}
	cmd.Flags().StringVar(&route, "route", "", "scope to route")
	cmd.Flags().StringVar(&models, "models", "", "comma-separated models")
	return cmd
}
