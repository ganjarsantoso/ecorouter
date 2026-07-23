package cli

import (
	"github.com/ganjar/ecorouter/internal/output"
	"github.com/ganjar/ecorouter/internal/secrets"
	"github.com/ganjar/ecorouter/internal/tui"
)

func runProviderRemoveWizard() error {
	if err := tui.RequireTTY(); err != nil {
		return err
	}
	cfg, err := requireConfig()
	if err != nil {
		return err
	}
	name, err := pickProvider(cfg, "Remove which provider?")
	if err != nil || name == "" {
		return err
	}
	ok, _ := tui.Confirm(
		"Remove provider "+name+"?",
		"This deletes the provider from config and purges its API key. Routes that reference its models will break.",
		false,
	)
	if !ok {
		return nil
	}
	delete(cfg.Providers, name)
	if err := cfg.Save(); err != nil {
		return err
	}
	if sec, err := secrets.Load(""); err == nil {
		_ = sec.Delete(name)
	}
	output.Success("Removed.")
	tui.PrintEquivalent("eco provider remove", []string{name})
	return nil
}
