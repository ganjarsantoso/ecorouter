package cli

import (
	"fmt"
	"net"
	"strings"

	"github.com/ganjar/ecorouter/internal/output"
	"github.com/ganjar/ecorouter/internal/tui"
	"github.com/spf13/cobra"
)

func newAccessCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "access",
		Short: "Optional IP allow/deny lists",
		Long:  `Optional IP allow/deny lists. The endpoint requires a valid Bearer token regardless.`,
	}
	cmd.AddCommand(
		newAccessAllowCmd(),
		newAccessDenyCmd(),
		newAccessListCmd(),
		newAccessClearCmd(),
	)
	return cmd
}

func newAccessAllowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "allow [cidr]",
		Short: "Restrict endpoint to given CIDR(s)",
		Long: `Add an allow rule (CIDR or single IP). When allow list is non-empty,
only matching IPs can reach the endpoint.

💡 Run with no arguments to be prompted for the CIDR.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := requireConfig()
			if err != nil {
				return err
			}
			force := WizardRequested()
			cidr := ""
			if len(args) == 1 {
				cidr = args[0]
			}
			cidr, err = askString(cidr, "cidr",
				"Allow CIDR",
				"IP or CIDR. Examples: 203.0.113.0/24 or 198.51.100.10",
				"e.g. 10.0.0.0/8", force,
				func(s string) error { return validateCIDR(strings.TrimSpace(s)) })
			if err != nil {
				return err
			}
			cidr = strings.TrimSpace(cidr)
			if !contains(cfg.Access.Allow, cidr) {
				cfg.Access.Allow = append(cfg.Access.Allow, cidr)
			}
			if err := cfg.Save(); err != nil {
				return err
			}
			if output.JSON {
				return output.PrintJSON(map[string]any{"allow": cfg.Access.Allow})
			}
			output.Success(fmt.Sprintf("Allow rule added: %s", cidr))
			tui.PrintEquivalent("eco access allow", []string{cidr})
			return nil
		},
	}
	return cmd
}

func newAccessDenyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deny [cidr]",
		Short: "Block CIDR(s)",
		Long: `Add a deny rule. Deny rules always win over allow rules.

💡 Run with no arguments to be prompted for the CIDR.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := requireConfig()
			if err != nil {
				return err
			}
			force := WizardRequested()
			cidr := ""
			if len(args) == 1 {
				cidr = args[0]
			}
			cidr, err = askString(cidr, "cidr",
				"Deny CIDR",
				"IP or CIDR. Examples: 203.0.113.0/24 or 198.51.100.10",
				"e.g. 10.0.0.0/8", force,
				func(s string) error { return validateCIDR(strings.TrimSpace(s)) })
			if err != nil {
				return err
			}
			cidr = strings.TrimSpace(cidr)
			if !contains(cfg.Access.Deny, cidr) {
				cfg.Access.Deny = append(cfg.Access.Deny, cidr)
			}
			if err := cfg.Save(); err != nil {
				return err
			}
			if output.JSON {
				return output.PrintJSON(map[string]any{"deny": cfg.Access.Deny})
			}
			output.Success(fmt.Sprintf("Deny rule added: %s", cidr))
			tui.PrintEquivalent("eco access deny", []string{cidr})
			return nil
		},
	}
	return cmd
}

func newAccessListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Show current allow/deny rules",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := requireConfig()
			if err != nil {
				return err
			}
			if output.JSON {
				return output.PrintJSON(map[string]any{"allow": cfg.Access.Allow, "deny": cfg.Access.Deny})
			}
			if len(cfg.Access.Allow) == 0 && len(cfg.Access.Deny) == 0 {
				output.Info("Access is open (anywhere). Allow list empty.")
				return nil
			}
			output.Info("Allow:")
			if len(cfg.Access.Allow) == 0 {
				output.Info("  (empty — open if no deny match)")
			}
			for _, a := range cfg.Access.Allow {
				output.Info("  " + a)
			}
			output.Info("Deny:")
			if len(cfg.Access.Deny) == 0 {
				output.Info("  (none)")
			}
			for _, d := range cfg.Access.Deny {
				output.Info("  " + d)
			}
			return nil
		},
	}
}

func newAccessClearCmd() *cobra.Command {
	var assumeYes bool
	cmd := &cobra.Command{
		Use:   "clear",
		Short: "Return to open (anywhere) access",
		Long: `Clear all allow/deny rules. The endpoint becomes reachable from anywhere
(token still required).`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := requireConfig()
			if err != nil {
				return err
			}
			ok, err := confirmDestructive(assumeYes,
				"Clear all access rules?",
				"Endpoint becomes open to anywhere (token still required).")
			if err != nil {
				return err
			}
			if !ok {
				return nil
			}
			cfg.Access.Allow = []string{}
			cfg.Access.Deny = []string{}
			if err := cfg.Save(); err != nil {
				return err
			}
			if output.JSON {
				return output.PrintJSON(map[string]string{"status": "open"})
			}
			output.Success("Access cleared — endpoint open to anywhere (token still required).")
			tui.PrintEquivalent("eco access clear", []string{"--yes"})
			return nil
		},
	}
	cmd.Flags().BoolVarP(&assumeYes, "yes", "y", false, "skip confirmation prompt")
	return cmd
}

func validateCIDR(s string) error {
	s = strings.TrimSpace(s)
	if !strings.Contains(s, "/") {
		if net.ParseIP(s) == nil {
			return fmt.Errorf("invalid IP/CIDR: %s", s)
		}
		return nil
	}
	_, _, err := net.ParseCIDR(s)
	if err != nil {
		return fmt.Errorf("invalid CIDR: %w", err)
	}
	return nil
}

func contains(ss []string, x string) bool {
	for _, s := range ss {
		if s == x {
			return true
		}
	}
	return false
}
