package cli

import (
	"fmt"

	"github.com/charmbracelet/huh"
	"github.com/ganjar/ecorouter/internal/cost"
	"github.com/ganjar/ecorouter/internal/output"
	"github.com/ganjar/ecorouter/internal/tui"
)

func runPricingWizard() error {
	if err := tui.RequireTTY(); err != nil {
		return err
	}
	cfg, err := requireConfig()
	if err != nil {
		return err
	}
	name, err := pickProvider(cfg, "Set prices for which provider?")
	if err != nil || name == "" {
		return err
	}
	return runPricingWizardFor(name)
}

func runPricingWizardFor(provider string) error {
	cfg, err := requireConfig()
	if err != nil {
		return err
	}
	p, ok := cfg.Providers[provider]
	if !ok || len(p.Models) == 0 {
		output.Warn("Provider has no models. Add/enable models first.")
		return nil
	}
	existing := cost.GetPrices()

	// Choose which models to price
	var chosen []string
	opts := make([]huh.Option[string], 0, len(p.Models))
	for _, m := range p.Models {
		key := provider + "/" + m
		label := m
		if px, ok := existing[key]; ok {
			label += fmt.Sprintf("  (currently $%.2f in / $%.2f out per 1M)", px.InputPer1M, px.OutputPer1M)
		}
		opts = append(opts, huh.NewOption(label, key))
	}
	if err := tui.MultiSelect("Which models to set prices for?", "Space to toggle.", opts, &chosen); err != nil {
		return err
	}
	if len(chosen) == 0 {
		output.Info("No models selected.")
		return nil
	}

	prices := existing
	for _, key := range chosen {
		var in, out string
		if px, ok := prices[key]; ok {
			in = fmt.Sprintf("%.4f", px.InputPer1M)
			out = fmt.Sprintf("%.4f", px.OutputPer1M)
		}
		if err := huh.NewForm(huh.NewGroup(
			huh.NewInput().Title(key+"  ▸  Input price (USD per 1M tokens)").
				Placeholder("e.g. 2.50").Value(&in),
			huh.NewInput().Title(key+"  ▸  Output price (USD per 1M tokens)").
				Placeholder("e.g. 10.00").Value(&out),
		)).WithTheme(tui.Theme()).Run(); err != nil {
			return err
		}
		var inF, outF float64
		fmt.Sscanf(in, "%f", &inF)
		fmt.Sscanf(out, "%f", &outF)
		prices[key] = cost.ModelPrice{InputPer1M: inF, OutputPer1M: outF}
	}
	if err := cost.SavePrices(prices); err != nil {
		return err
	}
	output.Success(fmt.Sprintf("Saved %d price(s) to %s", len(chosen), cost.PricingPath()))
	return nil
}
