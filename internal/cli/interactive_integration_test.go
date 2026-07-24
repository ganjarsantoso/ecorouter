package cli

import (
	"strings"
	"testing"
)

// runRoot builds a fresh root and executes it with the given argv tail.
// Using the root (not a subcommand directly) is important because
// persistent flags like --wizard are inherited from the root and
// would be "unknown flag" if SetArgs'd on a bare subcommand.
func runRoot(t *testing.T, args ...string) error {
	t.Helper()
	root := NewRoot()
	root.SetArgs(args)
	root.SilenceUsage = true
	root.SilenceErrors = true
	return root.Execute()
}

// All integration tests in this file run in non-TTY mode (set by the
// caller via ECO_NONINTERACTIVE=1). They verify the three contract
// requirements from revision2 §11.2:
//
//   8.  Full flags, non-TTY  → executes, no prompt, exit 0
//   9.  Missing flag, non-TTY → exits non-zero with an error naming the
//                                missing flag (no hang)
//   10. --wizard in non-TTY   → exits non-zero cleanly (cannot prompt),
//                                does NOT hang
//
// Each test sets ECO_NONINTERACTIVE=1 via t.Setenv and uses an isolated
// ECO_HOME so config/secrets are temporary.

func TestProviderRemove_Script_MissingYes(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ECO_HOME", dir)
	t.Setenv("ECO_NONINTERACTIVE", "1")
	// No provider exists — full flag path should still work for argument
	// resolution and error out cleanly with "not found" (no prompt, no hang).
	err := runRoot(t, "provider", "remove", "does-not-exist", "--yes")
	if err == nil {
		t.Fatal("expected error for non-existent provider")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProviderRemove_MissingName_NonTTY_Errors(t *testing.T) {
	// Bare `eco provider remove` on non-TTY must error naming --name.
	dir := t.TempDir()
	t.Setenv("ECO_HOME", dir)
	t.Setenv("ECO_NONINTERACTIVE", "1")
	err := runRoot(t, "provider", "remove") // no name, no flags
	if err == nil {
		t.Fatal("expected error when name missing on non-TTY")
	}
	if !strings.Contains(err.Error(), "--name") && !strings.Contains(err.Error(), "name") {
		t.Fatalf("error should mention the missing name flag, got: %v", err)
	}
}

func TestProviderRemove_Wizard_NonTTY_Errors(t *testing.T) {
	// --wizard on non-TTY must NOT hang; it must error out.
	dir := t.TempDir()
	t.Setenv("ECO_HOME", dir)
	t.Setenv("ECO_NONINTERACTIVE", "1")
	err := runRoot(t, "provider", "remove", "--wizard")
	if err == nil {
		t.Fatal("expected error when --wizard on non-TTY")
	}
	if !strings.Contains(err.Error(), "--name") && !strings.Contains(err.Error(), "name") {
		t.Fatalf("error should mention the missing name flag, got: %v", err)
	}
}

func TestTokenRevoke_MissingID_NonTTY_Errors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ECO_HOME", dir)
	t.Setenv("ECO_NONINTERACTIVE", "1")
	err := runRoot(t, "token", "revoke", "--yes")
	if err == nil {
		t.Fatal("expected error when id missing on non-TTY")
	}
	if !strings.Contains(err.Error(), "--id") && !strings.Contains(err.Error(), "id") {
		t.Fatalf("error should mention --id, got: %v", err)
	}
}

func TestTokenRevoke_DestructiveNoYes_NonTTY_Errors(t *testing.T) {
	// Even with a non-existent id, the destructive confirmation path runs
	// first (in current code) — so without --yes we expect --yes to be the
	// first missing thing.
	dir := t.TempDir()
	t.Setenv("ECO_HOME", dir)
	t.Setenv("ECO_NONINTERACTIVE", "1")
	err := runRoot(t, "token", "revoke", "tok_doesnotexist")
	if err == nil {
		t.Fatal("expected error: --yes required on non-TTY for destructive op")
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("error should mention --yes, got: %v", err)
	}
}

func TestAccessClear_MissingYes_NonTTY_Errors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ECO_HOME", dir)
	t.Setenv("ECO_NONINTERACTIVE", "1")
	err := runRoot(t, "access", "clear")
	if err == nil {
		t.Fatal("expected error: --yes required on non-TTY")
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("error should mention --yes, got: %v", err)
	}
}

func TestAccessAllow_MissingCIDR_NonTTY_Errors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ECO_HOME", dir)
	t.Setenv("ECO_NONINTERACTIVE", "1")
	err := runRoot(t, "access", "allow")
	if err == nil {
		t.Fatal("expected error when cidr missing on non-TTY")
	}
	if !strings.Contains(err.Error(), "--cidr") {
		t.Fatalf("error should mention --cidr, got: %v", err)
	}
}

func TestAccessAllow_ScriptWithCIDR_NoHang(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ECO_HOME", dir)
	t.Setenv("ECO_NONINTERACTIVE", "1")
	// Should succeed: valid CIDR, full flag invocation, no prompt.
	if err := runRoot(t, "access", "allow", "10.0.0.0/8"); err != nil {
		t.Fatalf("full-flag access allow should succeed, got: %v", err)
	}
}

func TestRouteAdd_MissingName_NonTTY_Errors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ECO_HOME", dir)
	t.Setenv("ECO_NONINTERACTIVE", "1")
	err := runRoot(t, "route", "add", "--single", "gpt-4o-mini")
	if err == nil {
		t.Fatal("expected error when name missing on non-TTY")
	}
	if !strings.Contains(err.Error(), "--name") {
		t.Fatalf("error should mention --name, got: %v", err)
	}
}

func TestRouteAdd_FullFlags_NoHang(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ECO_HOME", dir)
	t.Setenv("ECO_NONINTERACTIVE", "1")
	if err := runRoot(t, "route", "add", "default", "--single", "gpt-4o-mini"); err != nil {
		t.Fatalf("full-flag route add should succeed, got: %v", err)
	}
}

func TestRouteAdd_Wizard_NonTTY_Errors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ECO_HOME", dir)
	t.Setenv("ECO_NONINTERACTIVE", "1")
	err := runRoot(t, "route", "add", "--wizard")
	if err == nil {
		t.Fatal("expected error when --wizard on non-TTY")
	}
	// Either name or mode will be the first missing field.
	if !strings.Contains(err.Error(), "--") {
		t.Fatalf("error should mention a missing flag, got: %v", err)
	}
}

func TestProviderTest_MissingName_NonTTY_Errors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ECO_HOME", dir)
	t.Setenv("ECO_NONINTERACTIVE", "1")
	err := runRoot(t, "provider", "test")
	if err == nil {
		t.Fatal("expected error when name missing on non-TTY")
	}
	if !strings.Contains(err.Error(), "--name") {
		t.Fatalf("error should mention --name, got: %v", err)
	}
}

func TestConfigSet_MissingArgs_NonTTY_Errors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ECO_HOME", dir)
	t.Setenv("ECO_NONINTERACTIVE", "1")
	err := runRoot(t, "config", "set")
	if err == nil {
		t.Fatal("expected error when key missing on non-TTY")
	}
	if !strings.Contains(err.Error(), "--key") {
		t.Fatalf("error should mention --key, got: %v", err)
	}
}

func TestConfigSet_Script_NoHang(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ECO_HOME", dir)
	t.Setenv("ECO_NONINTERACTIVE", "1")
	if err := runRoot(t, "config", "set", "port", "8081"); err != nil {
		t.Fatalf("full-flag config set should succeed, got: %v", err)
	}
}

func TestUse_MissingName_NonTTY_Errors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ECO_HOME", dir)
	t.Setenv("ECO_NONINTERACTIVE", "1")
	err := runRoot(t, "use")
	if err == nil {
		t.Fatal("expected error when route missing on non-TTY")
	}
	if !strings.Contains(err.Error(), "--route") {
		t.Fatalf("error should mention --route, got: %v", err)
	}
}

func TestSaverAdd_MissingName_NonTTY_Errors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ECO_HOME", dir)
	t.Setenv("ECO_NONINTERACTIVE", "1")
	err := runRoot(t, "saver", "add", "--url", "http://127.0.0.1:8787")
	if err == nil {
		t.Fatal("expected error when name missing on non-TTY")
	}
	if !strings.Contains(err.Error(), "--name") {
		t.Fatalf("error should mention --name, got: %v", err)
	}
}

func TestSaverRemove_MissingYes_NonTTY_Errors(t *testing.T) {
	// First create a real saver via the script path, then attempt to
	// remove it without --yes on non-TTY. The destructive-op gate must
	// trigger and surface the --yes requirement.
	dir := t.TempDir()
	t.Setenv("ECO_HOME", dir)
	t.Setenv("ECO_NONINTERACTIVE", "1")

	if err := runRoot(t, "saver", "add", "real-saver", "--url", "http://127.0.0.1:8787"); err != nil {
		t.Fatalf("saver add setup failed: %v", err)
	}

	err := runRoot(t, "saver", "remove", "real-saver")
	if err == nil {
		t.Fatal("expected error: --yes required on non-TTY for destructive op")
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("error should mention --yes, got: %v", err)
	}
}

func TestPricingRemove_MissingKey_NonTTY_Errors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ECO_HOME", dir)
	t.Setenv("ECO_NONINTERACTIVE", "1")
	err := runRoot(t, "pricing", "remove", "--yes")
	if err == nil {
		t.Fatal("expected error when key missing on non-TTY")
	}
	if !strings.Contains(err.Error(), "--key") {
		t.Fatalf("error should mention --key, got: %v", err)
	}
}

// use the helper so it stays referenced in case future tests need it
var _ = runRoot
