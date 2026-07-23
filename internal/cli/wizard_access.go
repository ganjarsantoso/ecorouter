package cli

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/ganjar/ecorouter/internal/output"
	"github.com/ganjar/ecorouter/internal/tui"
)

func runAccessMenu() error {
	for {
		var choice string
		if err := tui.SelectString("🛡️  Access & limits", "Optional IP allow/deny lists.",
			[]huh.Option[string]{
				huh.NewOption("📋  Show current rules", "list"),
				huh.NewOption("✅  Add allow CIDR", "allow"),
				huh.NewOption("🚫  Add deny CIDR", "deny"),
				huh.NewOption("🗑️   Remove a rule", "remove"),
				huh.NewOption("🧹  Clear all rules", "clear"),
				huh.NewOption("↩️   Back", "back"),
			}, &choice); err != nil {
			return err
		}
		switch choice {
		case "list":
			_ = newAccessListCmd().RunE(newAccessListCmd(), nil)
		case "allow":
			_ = runAccessAddWizard("allow")
		case "deny":
			_ = runAccessAddWizard("deny")
		case "remove":
			_ = runAccessRemoveWizard()
		case "clear":
			_ = runAccessClearWizard()
		case "back", "":
			return nil
		}
	}
}

func runAccessAddWizard(kind string) error {
	if err := tui.RequireTTY(); err != nil {
		return err
	}
	var cidr string
	title := "Allow CIDR"
	if kind == "deny" {
		title = "Deny CIDR"
	}
	if err := tui.Input(title,
		"IP or CIDR. Examples: 203.0.113.0/24 or 198.51.100.10",
		"e.g. 10.0.0.0/8",
		&cidr,
		func(s string) error {
			return validateCIDR(strings.TrimSpace(s))
		},
	); err != nil {
		return err
	}
	cidr = strings.TrimSpace(cidr)
	if kind == "allow" {
		return newAccessAllowCmd().RunE(newAccessAllowCmd(), []string{cidr})
	}
	return newAccessDenyCmd().RunE(newAccessDenyCmd(), []string{cidr})
}

func runAccessRemoveWizard() error {
	if err := tui.RequireTTY(); err != nil {
		return err
	}
	cfg, err := requireConfig()
	if err != nil {
		return err
	}
	var opts []huh.Option[string]
	for _, a := range cfg.Access.Allow {
		opts = append(opts, huh.NewOption("✅ allow  "+a, "allow:"+a))
	}
	for _, d := range cfg.Access.Deny {
		opts = append(opts, huh.NewOption("🚫 deny   "+d, "deny:"+d))
	}
	if len(opts) == 0 {
		output.Info("No rules to remove.")
		return nil
	}
	var pick string
	if err := tui.SelectString("Remove which rule?", "", opts, &pick); err != nil {
		return err
	}
	parts := strings.SplitN(pick, ":", 2)
	if len(parts) != 2 {
		return nil
	}
	kind, val := parts[0], parts[1]
	if kind == "allow" {
		cfg.Access.Allow = removeString(cfg.Access.Allow, val)
	} else {
		cfg.Access.Deny = removeString(cfg.Access.Deny, val)
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	output.Success(fmt.Sprintf("Removed %s rule: %s", kind, val))
	return nil
}

func runAccessClearWizard() error {
	if err := tui.RequireTTY(); err != nil {
		return err
	}
	ok, _ := tui.Confirm("Clear all access rules?", "Endpoint becomes open to anywhere (token still required).", false)
	if !ok {
		return nil
	}
	return newAccessClearCmd().RunE(newAccessClearCmd(), nil)
}

func removeString(ss []string, x string) []string {
	var out []string
	for _, s := range ss {
		if s != x {
			out = append(out, s)
		}
	}
	return out
}
