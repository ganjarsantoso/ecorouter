package cli

import (
	"fmt"

	"github.com/charmbracelet/huh"
	"github.com/ganjar/ecorouter/internal/output"
	"github.com/ganjar/ecorouter/internal/secrets"
	"github.com/ganjar/ecorouter/internal/tui"
	"github.com/spf13/cobra"
)

func newModelsCmd() *cobra.Command {
	var provider string
	var refresh bool
	cmd := &cobra.Command{
		Use:   "models",
		Short: "List models across providers",
		Long: `List models across providers. Use --refresh to re-fetch the catalog
from each provider.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := requireConfig()
			if err != nil {
				return err
			}
			force := WizardRequested()

			if refresh {
				// If no --provider given, on interactive / wizard, offer "all or pick one?"
				if provider == "" && tui.IsInteractive() {
					if force || !cmd.Flags().Changed("provider") {
						choice := "all"
						if err := tui.SelectString("Refresh which providers?", "",
							[]huh.Option[string]{
								huh.NewOption("All providers", "all"),
								huh.NewOption("Pick one…", "pick"),
							}, &choice); err == nil && choice == "pick" {
							picked, err := askPick("", "provider", "Refresh which provider?",
								"", providerOptions(cfg), force)
							if err != nil {
								return err
							}
							provider = picked
						}
					}
				}
				sec, err := secrets.Load("")
				if err != nil {
					return err
				}
				for name, p := range cfg.Providers {
					if provider != "" && name != provider {
						continue
					}
					key, _ := sec.Get(name)
					models, err := fetchModels(p.Type, p.BaseURL, key)
					if err != nil {
						output.Warn(fmt.Sprintf("%s: refresh failed: %v", name, err))
						continue
					}
					p.Models = models
					cfg.Providers[name] = p
					output.Success(fmt.Sprintf("%s: %d models", name, len(models)))
				}
				if err := cfg.Save(); err != nil {
					return err
				}
			}

			type row struct {
				Provider string `json:"provider"`
				Model    string `json:"model"`
			}
			var rows []row
			var table [][]string
			for name, p := range cfg.Providers {
				if provider != "" && name != provider {
					continue
				}
				for _, m := range p.Models {
					rows = append(rows, row{name, m})
					table = append(table, []string{name, m})
				}
			}
			if output.JSON {
				return output.PrintJSON(rows)
			}
			if len(table) == 0 {
				output.Info("No models cached. Add a provider or run: eco models --refresh")
				return nil
			}
			output.Table([]string{"PROVIDER", "MODEL"}, table)
			return nil
		},
	}
	cmd.Flags().StringVar(&provider, "provider", "", "filter to one provider")
	cmd.Flags().BoolVar(&refresh, "refresh", false, "re-fetch catalogs from providers")
	return cmd
}
