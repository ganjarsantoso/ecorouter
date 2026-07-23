// Package tui wraps charmbracelet/huh with EcoRouter conventions.
// It provides TTY detection, consistent theming, and a small set of helpers
// so command code stays declarative.
package tui

import (
	"errors"
	"os"

	"github.com/charmbracelet/huh"
	"golang.org/x/term"
)

// ErrNotInteractive is returned by wizards when stdin is not a TTY
// and the caller must fall back to flag-driven flow.
var ErrNotInteractive = errors.New("stdin is not a terminal; use flags")

// IsInteractive reports whether we can safely open a wizard.
// It checks stdin AND stdout (both must be TTYs to draw menus).
func IsInteractive() bool {
	if os.Getenv("ECO_NONINTERACTIVE") == "1" {
		return false
	}
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}

// RequireTTY returns ErrNotInteractive when we're not on a TTY.
// Use at the top of any wizard entrypoint.
func RequireTTY() error {
	if !IsInteractive() {
		return ErrNotInteractive
	}
	return nil
}

// Theme returns the shared EcoRouter huh theme (green accents, dim helpers).
func Theme() *huh.Theme {
	t := huh.ThemeBase()
	// Keep it minimal so it blends with all terminals.
	return t
}

// RunForm runs a huh.Form with the shared theme.
// Returns ErrNotInteractive if not on a TTY.
func RunForm(form *huh.Form) error {
	if err := RequireTTY(); err != nil {
		return err
	}
	return form.WithTheme(Theme()).Run()
}

// Confirm asks a yes/no with a default.
func Confirm(title, description string, def bool) (bool, error) {
	if err := RequireTTY(); err != nil {
		return false, err
	}
	var out bool
	err := huh.NewConfirm().
		Title(title).
		Description(description).
		Affirmative("Yes").
		Negative("No").
		Value(&out).
		WithTheme(Theme()).
		Run()
	if err != nil {
		return def, err
	}
	return out, nil
}

// SelectString is a shortcut for a single-choice list.
func SelectString(title, description string, options []huh.Option[string], value *string) error {
	if err := RequireTTY(); err != nil {
		return err
	}
	return huh.NewSelect[string]().
		Title(title).
		Description(description).
		Options(options...).
		Value(value).
		WithTheme(Theme()).
		Run()
}

// MultiSelect is a shortcut for a multi-choice checklist.
func MultiSelect(title, description string, options []huh.Option[string], value *[]string) error {
	if err := RequireTTY(); err != nil {
		return err
	}
	return huh.NewMultiSelect[string]().
		Title(title).
		Description(description).
		Options(options...).
		Value(value).
		WithTheme(Theme()).
		Run()
}

// Input is a shortcut for a labeled text input.
func Input(title, description, placeholder string, value *string, validate func(string) error) error {
	if err := RequireTTY(); err != nil {
		return err
	}
	f := huh.NewInput().
		Title(title).
		Description(description).
		Placeholder(placeholder).
		Value(value)
	if validate != nil {
		f = f.Validate(validate)
	}
	return f.WithTheme(Theme()).Run()
}

// Password is like Input but masks characters.
func Password(title, description string, value *string) error {
	if err := RequireTTY(); err != nil {
		return err
	}
	return huh.NewInput().
		Title(title).
		Description(description).
		EchoMode(huh.EchoModePassword).
		Value(value).
		WithTheme(Theme()).
		Run()
}
