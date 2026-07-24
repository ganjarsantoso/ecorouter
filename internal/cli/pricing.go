package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/ganjar/ecorouter/internal/cost"
	"github.com/ganjar/ecorouter/internal/output"
	"github.com/ganjar/ecorouter/internal/tui"
	"github.com/spf13/cobra"
)

func newPricingCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pricing",
		Short: "Manage model prices (pricing.toml)",
		Long:  `Manage per-model prices used to estimate USD cost in activity/stats.`,
	}
	cmd.AddCommand(newPricingSetCmd(), newPricingListCmd(), newPricingRemoveCmd())
	return cmd
}

func newPricingSetCmd() *cobra.Command {
	var in, out float64
	cmd := &cobra.Command{
		Use:   "set [key]",
		Short: "Set per-1M-token USD prices for a model",
		Long: `Set per-1M-token USD prices for a model key (provider/model).

💡 Run with no arguments (or --wizard) to be guided step-by-step.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := requireConfig()
			if err != nil {
				return err
			}
			force := WizardRequested()
			key := ""
			if len(args) == 1 {
				key = args[0]
			}

			// If we have providers, build a dynamic picker; otherwise free-form.
			if len(cfg.Providers) > 0 {
				providerOpts := providerOptions(cfg)
				providerOpts = append([]huh.Option[string]{
					huh.NewOption("📝  Type a model key directly…", "__custom__"),
				}, providerOpts...)
				providerChoice, err := askChoice("", "provider",
					"Pick a provider", "Or type a model key directly.",
					providerOpts, force)
				if err != nil {
					return err
				}
				if providerChoice == "__custom__" {
					key, err = askString(key, "key",
						"Model key",
						"Format: provider/model (e.g. openai/gpt-4o)",
						"provider/model", force, nil)
					if err != nil {
						return err
					}
				} else {
					// Pick models of that provider
					pm, ok := cfg.Providers[providerChoice]
					if !ok || len(pm.Models) == 0 {
						key, err = askString(key, "key",
							"Model key",
							"Format: provider/model (e.g. openai/gpt-4o)",
							providerChoice+"/", force, nil)
						if err != nil {
							return err
						}
					} else {
						modelOpts := make([]huh.Option[string], 0, len(pm.Models))
						for _, m := range pm.Models {
							full := providerChoice + "/" + m
							modelOpts = append(modelOpts, huh.NewOption(full, full))
						}
						modelOpts = append(modelOpts, huh.NewOption("📝  Other (type the key)…", "__custom__"))
						key, err = askPick("", "model", "Pick a model", "", modelOpts, force)
						if err != nil {
							return err
						}
						if key == "__custom__" {
							key, err = askString("", "key",
								"Model key", "",
								providerChoice+"/", force, nil)
							if err != nil {
								return err
							}
						}
					}
				}
			} else {
				key, err = askString(key, "key",
					"Model key",
					"Format: provider/model (e.g. openai/gpt-4o)",
					"provider/model", force, nil)
				if err != nil {
					return err
				}
			}
			key = strings.TrimSpace(key)
			if key == "" {
				return exitErr(2, fmt.Errorf("model key required (e.g. openai/gpt-4o)"))
			}

			// Pre-fill from existing price if any
			if !cmd.Flags().Changed("in") || !cmd.Flags().Changed("out") {
				if existing, ok := cost.GetPrices()[key]; ok {
					if !cmd.Flags().Changed("in") {
						in = existing.InputPer1M
					}
					if !cmd.Flags().Changed("out") {
						out = existing.OutputPer1M
					}
				}
			}

			// Input price
			inStr := ""
			if cmd.Flags().Changed("in") || (in != 0) {
				inStr = fmt.Sprintf("%g", in)
			}
			inStr, err = askString(inStr, "in",
				key+"  ▸  Input price (USD per 1M tokens)",
				"e.g. 2.50", "2.50", force, nil)
			if err != nil {
				return err
			}
			var inF float64
			fmt.Sscanf(strings.TrimSpace(inStr), "%f", &inF)

			// Output price
			outStr := ""
			if cmd.Flags().Changed("out") || (out != 0) {
				outStr = fmt.Sprintf("%g", out)
			}
			outStr, err = askString(outStr, "out",
				key+"  ▸  Output price (USD per 1M tokens)",
				"e.g. 10.00", "10.00", force, nil)
			if err != nil {
				return err
			}
			var outF float64
			fmt.Sscanf(strings.TrimSpace(outStr), "%f", &outF)

			prices := cost.GetPrices()
			prices[key] = cost.ModelPrice{InputPer1M: inF, OutputPer1M: outF}
			if err := cost.SavePrices(prices); err != nil {
				return err
			}
			if output.JSON {
				_ = output.PrintJSON(map[string]any{
					"key": key, "input_per_1m": inF, "output_per_1m": outF,
					"path": cost.PricingPath(),
				})
				tui.PrintEquivalent("eco pricing set", []string{key, fmt.Sprintf("--in %g", inF), fmt.Sprintf("--out %g", outF)})
				return nil
			}
			output.Success(fmt.Sprintf("Set %s → $%.4f in / $%.4f out per 1M tokens.", key, inF, outF))
			output.Info("  " + cost.PricingPath())
			tui.PrintEquivalent("eco pricing set", []string{key, fmt.Sprintf("--in %g", inF), fmt.Sprintf("--out %g", outF)})
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
	var assumeYes bool
	cmd := &cobra.Command{
		Use:   "remove [key]",
		Short: "Remove a model price entry",
		Long: `Remove a price entry. Activity for that model will go back to "unpriced".

💡 Run with no arguments to pick from a list.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			force := WizardRequested()
			key := ""
			if len(args) == 1 {
				key = args[0]
			}
			prices := cost.GetPrices()
			if key == "" {
				opts := make([]huh.Option[string], 0, len(prices))
				keys := make([]string, 0, len(prices))
				for k := range prices {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					p := prices[k]
					opts = append(opts, huh.NewOption(
						fmt.Sprintf("%s  ($%.2f in / $%.2f out)", k, p.InputPer1M, p.OutputPer1M), k))
				}
				k, err := askPick("", "key", "Remove price for which key?",
					"Activity for that model will go back to unpriced.", opts, force)
				if err != nil {
					return err
				}
				key = k
			}
			key = strings.TrimSpace(key)
			if _, ok := prices[key]; !ok {
				return exitErr(1, fmt.Errorf("no price for %q", key))
			}
			ok, err := confirmDestructive(assumeYes,
				fmt.Sprintf("Remove price for %q?", key),
				"Activity for that model will go back to unpriced.")
			if err != nil {
				return err
			}
			if !ok {
				return nil
			}
			delete(prices, key)
			if err := cost.SavePrices(prices); err != nil {
				return err
			}
			if output.JSON {
				return output.PrintJSON(map[string]string{"removed": key})
			}
			output.Success(fmt.Sprintf("Removed price for %q.", key))
			tui.PrintEquivalent("eco pricing remove", []string{key, "--yes"})
			return nil
		},
	}
	cmd.Flags().BoolVarP(&assumeYes, "yes", "y", false, "skip confirmation prompt")
	return cmd
}
