package cli

import (
	"fmt"
	"time"

	"github.com/ganjar/ecorouter/internal/auth"
	"github.com/ganjar/ecorouter/internal/config"
	"github.com/ganjar/ecorouter/internal/output"
	"github.com/ganjar/ecorouter/internal/store"
	"github.com/spf13/cobra"
)

func newTokenCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "token",
		Short: "Manage client Bearer tokens",
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
		Use:   "new <label>",
		Short: "Generate a Bearer token (shown once)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			label := args[0]
			if err := config.EnsureDirs(); err != nil {
				return err
			}
			if _, err := requireConfig(); err != nil {
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

			if output.JSON {
				return output.PrintJSON(map[string]any{
					"id":             id,
					"label":          label,
					"token":          plain,
					"daily_cap":      dailyCap,
					"max_concurrent": maxConcurrent,
					"notice":         "store this token now; it will not be shown again",
				})
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
	return &cobra.Command{
		Use:   "rotate <id>",
		Short: "Issue a new secret; invalidate the old one",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := store.Open("")
			if err != nil {
				return err
			}
			defer db.Close()
			t, err := db.GetToken(args[0])
			if err != nil {
				return exitErr(1, fmt.Errorf("token %s not found", args[0]))
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
				return output.PrintJSON(map[string]any{"id": t.ID, "token": plain, "notice": "store this token now"})
			}
			output.Success(fmt.Sprintf("Token %s rotated. Old secret is invalid.", t.ID))
			fmt.Println()
			fmt.Printf("  %s\n", plain)
			fmt.Println("  ← copy now, shown once")
			return nil
		},
	}
}

func newTokenRevokeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "revoke <id>",
		Short: "Instantly revoke a token",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := store.Open("")
			if err != nil {
				return err
			}
			defer db.Close()
			if err := db.RevokeToken(args[0]); err != nil {
				return exitErr(1, err)
			}
			_ = db.InsertAudit("token_revoke", "revoked", "", args[0])
			if output.JSON {
				return output.PrintJSON(map[string]string{"revoked": args[0]})
			}
			output.Success(fmt.Sprintf("Token %s revoked.", args[0]))
			return nil
		},
	}
}

func newTokenScopeCmd() *cobra.Command {
	var route, models string
	cmd := &cobra.Command{
		Use:   "scope <id>",
		Short: "Adjust token scope after creation",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := store.Open("")
			if err != nil {
				return err
			}
			defer db.Close()
			if _, err := db.GetToken(args[0]); err != nil {
				return exitErr(1, fmt.Errorf("token %s not found", args[0]))
			}
			if err := db.UpdateTokenScope(args[0], route, splitModels(models)); err != nil {
				return err
			}
			_ = db.InsertAudit("token_scope", "route="+route+" models="+models, "", args[0])
			if output.JSON {
				return output.PrintJSON(map[string]any{"id": args[0], "scope_route": route, "scope_models": splitModels(models)})
			}
			output.Success(fmt.Sprintf("Token %s scope updated.", args[0]))
			return nil
		},
	}
	cmd.Flags().StringVar(&route, "route", "", "scope to route")
	cmd.Flags().StringVar(&models, "models", "", "comma-separated models")
	return cmd
}
