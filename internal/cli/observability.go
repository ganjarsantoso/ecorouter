package cli

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ganjar/ecorouter/internal/output"
	"github.com/ganjar/ecorouter/internal/store"
	"github.com/spf13/cobra"
)

func newActivityCmd() *cobra.Command {
	var since, tokenID string
	var limit int
	cmd := &cobra.Command{
		Use:   "activity",
		Short: "Recent requests: token, IP, route, model, latency, status",
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := store.Open("")
			if err != nil {
				return err
			}
			defer db.Close()
			var sinceT time.Time
			if since != "" {
				d, err := parseSince(since)
				if err != nil {
					return exitErr(2, err)
				}
				sinceT = time.Now().Add(-d)
			}
			rows, err := db.ListActivity(sinceT, tokenID, limit)
			if err != nil {
				return err
			}
			if output.JSON {
				return output.PrintJSON(rows)
			}
			if len(rows) == 0 {
				output.Info("No activity yet.")
				return nil
			}
			var table [][]string
			for _, a := range rows {
				costStr := "unpriced"
				if a.CostEstimate != nil {
					costStr = fmt.Sprintf("$%.4f", *a.CostEstimate)
				} else if a.TokensIn == 0 && a.TokensOut == 0 {
					costStr = "—"
				}
				table = append(table, []string{
					a.TS.Local().Format("15:04:05"),
					a.TokenLabel,
					a.SrcIP,
					a.Route,
					a.Model,
					fmt.Sprintf("%d/%d", a.TokensIn, a.TokensOut),
					fmt.Sprintf("%dms", a.LatencyMs),
					strconv.Itoa(a.Status),
					costStr,
				})
			}
			output.Table([]string{"TIME", "TOKEN", "IP", "ROUTE", "MODEL", "TOK IN/OUT", "LAT", "STATUS", "COST"}, table)
			return nil
		},
	}
	cmd.Flags().StringVar(&since, "since", "", "time window e.g. 1h, 24h, 7d")
	cmd.Flags().StringVar(&tokenID, "token", "", "filter by token id")
	cmd.Flags().IntVar(&limit, "limit", 50, "max rows")
	return cmd
}

func newStatsCmd() *cobra.Command {
	var group, since string
	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Rollups by day / model / route / token",
		RunE: func(cmd *cobra.Command, args []string) error {
			if group == "" {
				group = "route"
			}
			db, err := store.Open("")
			if err != nil {
				return err
			}
			defer db.Close()
			d := 24 * time.Hour
			if since != "" {
				var err error
				d, err = parseSince(since)
				if err != nil {
					return exitErr(2, err)
				}
			}
			rows, err := db.StatsBy(group, time.Now().Add(-d))
			if err != nil {
				return err
			}
			if output.JSON {
				return output.PrintJSON(rows)
			}
			if len(rows) == 0 {
				output.Info("No stats yet.")
				return nil
			}
			var table [][]string
			for _, r := range rows {
				table = append(table, []string{
					r.Key,
					strconv.Itoa(r.Requests),
					strconv.Itoa(r.TokensIn),
					strconv.Itoa(r.TokensOut),
					fmt.Sprintf("%.0f", r.AvgLatMs),
					strconv.Itoa(r.Errors),
				})
			}
			output.Table([]string{strings.ToUpper(group), "REQS", "TOK IN", "TOK OUT", "AVG MS", "ERRS"}, table)
			return nil
		},
	}
	cmd.Flags().StringVar(&group, "by", "route", "group by: route|model|token|day")
	cmd.Flags().StringVar(&since, "since", "24h", "time window")
	return cmd
}

func newAuditCmd() *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Security-focused view: auth failures, lockouts, revocations",
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := store.Open("")
			if err != nil {
				return err
			}
			defer db.Close()
			rows, err := db.ListAudit(limit)
			if err != nil {
				return err
			}
			if output.JSON {
				return output.PrintJSON(rows)
			}
			if len(rows) == 0 {
				output.Info("No audit events.")
				return nil
			}
			var table [][]string
			for _, e := range rows {
				table = append(table, []string{
					e.TS.Local().Format(time.RFC3339),
					e.Event,
					e.SrcIP,
					e.TokenID,
					e.Detail,
				})
			}
			output.Table([]string{"TIME", "EVENT", "IP", "TOKEN", "DETAIL"}, table)
			return nil
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 50, "max rows")
	return cmd
}

func parseSince(s string) (time.Duration, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if d, err := time.ParseDuration(s); err == nil {
		return d, nil
	}
	if strings.HasSuffix(s, "d") {
		var n int
		if _, err := fmt.Sscanf(s, "%dd", &n); err != nil {
			return 0, fmt.Errorf("invalid --since %q", s)
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	return 0, fmt.Errorf("invalid --since %q (use 1h, 24h, 7d)", s)
}
