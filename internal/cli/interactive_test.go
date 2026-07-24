package cli

import (
	"strings"
	"testing"

	"github.com/charmbracelet/huh"
)

func TestNeed(t *testing.T) {
	cases := []struct {
		name    string
		current string
		force   bool
		want    bool
	}{
		{"empty + no force", "", false, true},
		{"empty + force", "", true, true},
		{"set + no force", "x", false, false},
		{"set + force (wizard override)", "x", true, true},
	}
	for _, c := range cases {
		if got := need(c.current, c.force); got != c.want {
			t.Errorf("need(%q, %v) = %v, want %v", c.current, c.force, got, c.want)
		}
	}
}

func TestAskString_ValuePresent_NoForce_NoPrompt(t *testing.T) {
	// Force non-interactive: any ask* helper without --wizard and a value
	// set must return that value immediately, never touching stdin.
	t.Setenv("ECO_NONINTERACTIVE", "1")
	got, err := askString("alice", "name", "ignored title", "ignored desc", "ignored", false, nil)
	if err != nil {
		t.Fatalf("askString should not error when value is set: %v", err)
	}
	if got != "alice" {
		t.Fatalf("askString returned %q, want %q", got, "alice")
	}
}

func TestAskString_ValueSet_Force_StillNoPromptInNonTTY(t *testing.T) {
	// --wizard/force means "re-prompt for everything", so even with a value
	// set, on a non-TTY there's nothing to prompt with → it must error
	// cleanly (and never hang). The test asserts: force changes the
	// behavior from "use the value" to "try to prompt", which surfaces the
	// same non-TTY error path.
	t.Setenv("ECO_NONINTERACTIVE", "1")
	_, err := askString("bob", "name", "t", "d", "p", true, nil)
	if err == nil {
		t.Fatal("askString with force on non-TTY must error (cannot prompt); got nil")
	}
	if !strings.Contains(err.Error(), "--name") {
		t.Fatalf("error should mention --name, got: %v", err)
	}
}

func TestAskString_EmptyValue_NonTTY_Errors(t *testing.T) {
	t.Setenv("ECO_NONINTERACTIVE", "1")
	_, err := askString("", "name", "ignored", "ignored", "ignored", false, nil)
	if err == nil {
		t.Fatal("askString should error when value missing and non-TTY")
	}
	if !strings.Contains(err.Error(), "--name") {
		t.Fatalf("error should mention the missing flag, got: %v", err)
	}
}

func TestAskString_EmptyValue_Force_NonTTY_Errors(t *testing.T) {
	// --wizard on a non-TTY must NOT hang; it must still error out.
	t.Setenv("ECO_NONINTERACTIVE", "1")
	_, err := askString("", "url", "ignored", "ignored", "ignored", true, nil)
	if err == nil {
		t.Fatal("askString should error when value missing even with --wizard on non-TTY")
	}
	if !strings.Contains(err.Error(), "--url") {
		t.Fatalf("error should mention the missing flag, got: %v", err)
	}
}

func TestAskSecret_EmptyValue_NonTTY_Errors(t *testing.T) {
	t.Setenv("ECO_NONINTERACTIVE", "1")
	_, err := askSecret("", "key", "API key", "desc", false)
	if err == nil {
		t.Fatal("askSecret should error when value missing and non-TTY")
	}
	if !strings.Contains(err.Error(), "--key") {
		t.Fatalf("error should mention --key, got: %v", err)
	}
}

func TestAskSecret_Present_NoForce_NoPrompt(t *testing.T) {
	t.Setenv("ECO_NONINTERACTIVE", "1")
	got, err := askSecret("sk-abc", "key", "API key", "desc", false)
	if err != nil {
		t.Fatalf("askSecret should not error when set: %v", err)
	}
	if got != "sk-abc" {
		t.Fatalf("askSecret returned %q, want %q", got, "sk-abc")
	}
}

func TestAskChoice_EmptyValue_NonTTY_Errors(t *testing.T) {
	t.Setenv("ECO_NONINTERACTIVE", "1")
	opts := []huh.Option[string]{
		huh.NewOption("a", "a"),
		huh.NewOption("b", "b"),
	}
	_, err := askChoice("", "auth", "title", "desc", opts, false)
	if err == nil {
		t.Fatal("askChoice should error when value missing and non-TTY")
	}
	if !strings.Contains(err.Error(), "--auth") {
		t.Fatalf("error should mention --auth, got: %v", err)
	}
}

func TestAskChoice_Present_NoForce_NoPrompt(t *testing.T) {
	t.Setenv("ECO_NONINTERACTIVE", "1")
	opts := []huh.Option[string]{
		huh.NewOption("a", "a"),
		huh.NewOption("b", "b"),
	}
	got, err := askChoice("b", "auth", "t", "d", opts, false)
	if err != nil {
		t.Fatalf("askChoice should not error when set: %v", err)
	}
	if got != "b" {
		t.Fatalf("askChoice returned %q, want %q", got, "b")
	}
}

func TestAskPick_EmptyItems_Errors(t *testing.T) {
	t.Setenv("ECO_NONINTERACTIVE", "1")
	_, err := askPick("", "name", "t", "d", nil, false)
	if err == nil {
		t.Fatal("askPick with empty items should error")
	}
}

func TestAskPick_EmptyValue_NonTTY_Errors(t *testing.T) {
	t.Setenv("ECO_NONINTERACTIVE", "1")
	opts := []huh.Option[string]{huh.NewOption("one", "one")}
	_, err := askPick("", "name", "t", "d", opts, false)
	if err == nil {
		t.Fatal("askPick should error when value missing and non-TTY")
	}
	if !strings.Contains(err.Error(), "--name") {
		t.Fatalf("error should mention --name, got: %v", err)
	}
}

func TestAskPick_Present_NoForce_NoPrompt(t *testing.T) {
	t.Setenv("ECO_NONINTERACTIVE", "1")
	opts := []huh.Option[string]{
		huh.NewOption("a", "a"),
		huh.NewOption("b", "b"),
	}
	got, err := askPick("a", "name", "t", "d", opts, false)
	if err != nil {
		t.Fatalf("askPick should not error when set: %v", err)
	}
	if got != "a" {
		t.Fatalf("askPick returned %q, want %q", got, "a")
	}
}

func TestConfirmDestructive_AssumeYes_NoPrompt(t *testing.T) {
	t.Setenv("ECO_NONINTERACTIVE", "1")
	ok, err := confirmDestructive(true, "Remove?", "desc")
	if err != nil {
		t.Fatalf("confirmDestructive(yes) should not error: %v", err)
	}
	if !ok {
		t.Fatal("confirmDestructive(yes) should return true")
	}
}

func TestConfirmDestructive_NoYes_NonTTY_Errors(t *testing.T) {
	// Non-TTY without --yes must NEVER hang; it must error out cleanly.
	t.Setenv("ECO_NONINTERACTIVE", "1")
	ok, err := confirmDestructive(false, "Remove?", "desc")
	if err == nil {
		t.Fatal("confirmDestructive without --yes on non-TTY should error")
	}
	if ok {
		t.Fatal("confirmDestructive should not return true on error path")
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("error should mention --yes, got: %v", err)
	}
}
