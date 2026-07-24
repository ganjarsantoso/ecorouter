package cli

import (
	"fmt"

	"github.com/charmbracelet/huh"
	"github.com/ganjar/ecorouter/internal/tui"
)

// need decides whether we must prompt for a value.
// current: the value already gathered from a flag ("" means unset).
// force:   --wizard was passed (prompt even if set, using the value as default).
func need(current string, force bool) bool {
	if force {
		return true
	}
	return current == ""
}

// askString resolves a free-text value.
// flagName is used to build a helpful error in non-TTY mode.
func askString(current, flagName, title, desc, placeholder string, force bool, validate func(string) error) (string, error) {
	if !need(current, force) {
		return current, nil
	}
	if !tui.IsInteractive() {
		return "", fmt.Errorf("missing --%s (required in non-interactive mode)", flagName)
	}
	val := current
	if err := tui.Input(title, desc, placeholder, &val, validate); err != nil {
		return "", err
	}
	return val, nil
}

// askSecret resolves a hidden value (API keys, passwords).
func askSecret(current, flagName, title, desc string, force bool) (string, error) {
	if !need(current, force) {
		return current, nil
	}
	if !tui.IsInteractive() {
		return "", fmt.Errorf("missing --%s (required in non-interactive mode)", flagName)
	}
	val := current
	if err := tui.Password(title, desc, &val); err != nil {
		return "", err
	}
	return val, nil
}

// askChoice resolves a single choice from a fixed set.
func askChoice(current, flagName, title, desc string, opts []huh.Option[string], force bool) (string, error) {
	if !need(current, force) {
		return current, nil
	}
	if !tui.IsInteractive() {
		return "", fmt.Errorf("missing --%s (choose one of the documented values)", flagName)
	}
	val := current
	if err := tui.SelectString(title, desc, opts, &val); err != nil {
		return "", err
	}
	return val, nil
}

// askPick resolves a choice from a DYNAMIC set (existing providers, routes,
// tokens, savers). This is what removes the need to know IDs/names.
// items: display->value options. Returns the chosen value.
func askPick(current, flagName, title, desc string, items []huh.Option[string], force bool) (string, error) {
	if !need(current, force) {
		return current, nil
	}
	if !tui.IsInteractive() {
		// In non-TTY we don't know if items is empty, and the user can't
		// pick from a list anyway. Tell them the actionable thing: pass
		// the identifier explicitly.
		return "", fmt.Errorf("missing --%s (pass the name/id explicitly)", flagName)
	}
	if len(items) == 0 {
		return "", fmt.Errorf("nothing to choose from")
	}
	val := current
	if err := tui.SelectString(title, desc, items, &val); err != nil {
		return "", err
	}
	return val, nil
}

// confirmDestructive gates remove/revoke/clear/rotate operations.
// In non-TTY, requires --yes to have been passed (caller checks assumeYes).
func confirmDestructive(assumeYes bool, title, desc string) (bool, error) {
	if assumeYes {
		return true, nil
	}
	if !tui.IsInteractive() {
		return false, fmt.Errorf("destructive operation needs --yes in non-interactive mode")
	}
	return tui.Confirm(title, desc, false)
}
