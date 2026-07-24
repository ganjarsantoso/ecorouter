package cli

import (
	"fmt"
	"sort"

	"github.com/charmbracelet/huh"
	"github.com/ganjar/ecorouter/internal/config"
	"github.com/ganjar/ecorouter/internal/store"
)

func providerOptions(cfg *config.Config) []huh.Option[string] {
	var o []huh.Option[string]
	for n, p := range cfg.Providers {
		o = append(o, huh.NewOption(fmt.Sprintf("%s  (%s)", n, p.BaseURL), n))
	}
	return o
}

func routeOptions(cfg *config.Config) []huh.Option[string] {
	var o []huh.Option[string]
	for n, r := range cfg.Routes {
		label := fmt.Sprintf("%s  [%s]", n, r.Mode)
		if n == cfg.Defaults.ActiveRoute {
			label += "  ⭐ active"
		}
		o = append(o, huh.NewOption(label, n))
	}
	return o
}

func saverOptions(cfg *config.Config) []huh.Option[string] {
	var o []huh.Option[string]
	for n, s := range cfg.Savers {
		mark := ""
		if n == cfg.Defaults.SaverDefault {
			mark = "  ⭐ default"
		}
		o = append(o, huh.NewOption(fmt.Sprintf("%s  (%s)%s", n, s.URL, mark), n))
	}
	return o
}

// modelOptions flattens every provider's catalog into "provider/model" IDs.
// Sorted for stable, predictable order in pickers.
func modelOptions(cfg *config.Config) []huh.Option[string] {
	var ids []string
	for pName, p := range cfg.Providers {
		for _, m := range p.Models {
			ids = append(ids, pName+"/"+m)
		}
	}
	sort.Strings(ids)
	o := make([]huh.Option[string], 0, len(ids))
	for _, id := range ids {
		o = append(o, huh.NewOption(id, id))
	}
	return o
}

func tokenOptions(db *store.Store) ([]huh.Option[string], error) {
	toks, err := db.ListTokens()
	if err != nil {
		return nil, err
	}
	var o []huh.Option[string]
	for _, t := range toks {
		status := "active"
		if t.Revoked {
			status = "revoked"
		}
		o = append(o, huh.NewOption(
			fmt.Sprintf("%s  (%s, %s)", t.Label, t.ID, status), t.ID))
	}
	return o, nil
}
