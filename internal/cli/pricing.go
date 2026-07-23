package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ganjar/ecorouter/internal/cost"
	"github.com/ganjar/ecorouter/internal/output"
	"github.com/spf13/cobra"
)

func newPricingCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pricing",
		Short: "Manage model prices (pricing.toml)",
	}
	cmd.AddCommand(newPricingSetCmd(), newPricingListCmd(), newPricingRemoveCmd())
	return cmd
}

func newPricingSetCmd() *cobra.Command {
	var in, out float64
	cmd := &cobra.Command{
		Use:   "set <provider>/<model>",
		Short: "Set per-1M-token USD prices for a model",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := strings.TrimSpace(args[0])
			if key == "" {
				return exitErr(2, fmt.Errorf("model key required (e.g. openai/gpt-4o)"))
			}
			if !cmd.Flags().Changed("in") || !cmd.Flags().Changed("out") {
				return exitErr(2, fmt.Errorf("--in and --out are required (USD per 1M tokens)"))
			}
			prices := cost.GetPrices()
			prices[key] = cost.ModelPrice{InputPer1M: in, OutputPer1M: out}
			if err := cost.SavePrices(prices); err != nil {
				return err
			}
			if output.JSON {
				return output.PrintJSON(map[string]any{
					"key": key, "input_per_1m": in, "output_per_1m": out,
					"path": cost.PricingPath(),
				})
			}
			output.Success(fmt.Sprintf("Set %s → $%.4f in / $%.4f out per 1M tokens.", key, in, out))
			output.Info("  " + cost.PricingPath())
			return nil
		},
	}
	cmd.Flags().Float64Var(&in, "in", 0, "input price USD per 1M tokens")
	cmd.Flags().Float64Var(&out, "out", 0, "output price USD per 1M tokens")
	return cmd
}

func newPricingListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all configured model prices",
		RunE: func(cmd *cobra.Command, args []string) error {
			prices := cost.GetPrices()
			if output.JSON {
				return output.PrintJSON(prices)
			}
			if len(prices) == 0 {
				output.Info("No prices configured. All activity will show as \"unpriced\".")
				output.Info("  Fix:  eco pricing set <provider>/<model> --in 2.50 --out 10.00")
				output.Info("  Or:   eco  → Providers → Set / edit model pricing")
				return nil
			}
			keys := make([]string, 0, len(prices))
			for k := range prices {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			var table [][]string
			for _, k := range keys {
				p := prices[k]
				table = append(table, []string{
					k,
					fmt.Sprintf("%.4f", p.InputPer1M),
					fmt.Sprintf("%.4f", p.OutputPer1M),
				})
			}
			output.Table([]string{"MODEL", "IN $/1M", "OUT $/1M"}, table)
			output.Info("  File: " + cost.PricingPath())
			return nil
		},
	}
}

func newPricingRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <provider>/<model>",
		Short: "Remove a model price entry",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := strings.TrimSpace(args[0])
			prices := cost.GetPrices()
			if _, ok := prices[key]; !ok {
				return exitErr(1, fmt.Errorf("no price for %q", key))
			}
			delete(prices, key)
			if err := cost.SavePrices(prices); err != nil {
				return err
			}
			if output.JSON {
				return output.PrintJSON(map[string]string{"removed": key})
			}
			output.Success(fmt.Sprintf("Removed price for %q.", key))
			return nil
		},
	}
}
