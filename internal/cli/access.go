package cli

import (
	"fmt"
	"net"
	"strings"

	"github.com/ganjar/ecorouter/internal/output"
	"github.com/spf13/cobra"
)

func newAccessCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "access",
		Short: "Optional IP allow/deny lists",
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
	return &cobra.Command{
		Use:   "allow <cidr>",
		Short: "Restrict endpoint to given CIDR(s)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateCIDR(args[0]); err != nil {
				return exitErr(2, err)
			}
			cfg, err := requireConfig()
			if err != nil {
				return err
			}
			if !contains(cfg.Access.Allow, args[0]) {
				cfg.Access.Allow = append(cfg.Access.Allow, args[0])
			}
			if err := cfg.Save(); err != nil {
				return err
			}
			if output.JSON {
				return output.PrintJSON(map[string]any{"allow": cfg.Access.Allow})
			}
			output.Success(fmt.Sprintf("Allow rule added: %s", args[0]))
			return nil
		},
	}
}

func newAccessDenyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "deny <cidr>",
		Short: "Block CIDR(s)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateCIDR(args[0]); err != nil {
				return exitErr(2, err)
			}
			cfg, err := requireConfig()
			if err != nil {
				return err
			}
			if !contains(cfg.Access.Deny, args[0]) {
				cfg.Access.Deny = append(cfg.Access.Deny, args[0])
			}
			if err := cfg.Save(); err != nil {
				return err
			}
			if output.JSON {
				return output.PrintJSON(map[string]any{"deny": cfg.Access.Deny})
			}
			output.Success(fmt.Sprintf("Deny rule added: %s", args[0]))
			return nil
		},
	}
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
	return &cobra.Command{
		Use:   "clear",
		Short: "Return to open (anywhere) access",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := requireConfig()
			if err != nil {
				return err
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
			return nil
		},
	}
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
