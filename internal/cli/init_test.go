package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestInitNonInteractiveRequiresBaseURL(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ECO_HOME", dir)
	t.Setenv("ECO_NONINTERACTIVE", "1")

	cmd := newInitCmd()
	cmd.SetArgs([]string{
		"--yes",
		"--provider-name", "openai",
		"--provider-auth", "bearer",
		"--provider-key", "sk-test",
		// intentionally no --provider-base-url
		"--route-mode", "single",
		"--route-models", "gpt-4o-mini",
		"--token-label", "ci",
	})
	// silence cobra usage noise
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when --provider-base-url is missing")
	}
	if !strings.Contains(err.Error(), "base-url") && !strings.Contains(err.Error(), "provider-base-url") {
		t.Fatalf("expected base-url error, got: %v", err)
	}
}

func TestInitNonInteractiveWithBaseURLSucceeds(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ECO_HOME", dir)
	t.Setenv("ECO_NONINTERACTIVE", "1")

	// Use unreachable URL so fetchModels fails but init still saves
	cmd := newInitCmd()
	cmd.SetArgs([]string{
		"--yes",
		"--provider-name", "openai",
		"--provider-auth", "bearer",
		"--provider-base-url", "http://127.0.0.1:1",
		"--provider-key", "sk-test",
		"--route-mode", "single",
		"--route-models", "gpt-4o-mini",
		"--token-label", "ci",
	})
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	// capture output
	cmd.SetOut(os.Stdout)
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("init should succeed even if provider verify fails: %v", err)
	}
	// config should exist
	if _, err := os.Stat(filepath.Join(dir, "config.toml")); err != nil {
		t.Fatal(err)
	}
}

func TestInitLegacyProviderTypeAlias(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ECO_HOME", dir)
	t.Setenv("ECO_NONINTERACTIVE", "1")

	cmd := newInitCmd()
	cmd.SetArgs([]string{
		"--yes",
		"--provider-type", "openai", // legacy
		"--provider-name", "myopenai",
		"--provider-base-url", "http://127.0.0.1:1",
		"--provider-key", "sk-test",
		"--route-mode", "single",
		"--route-models", "gpt-4o-mini",
		"--token-label", "ci",
	})
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
}

// ensure cobra is referenced for future flag tests
var _ = cobra.Command{}
