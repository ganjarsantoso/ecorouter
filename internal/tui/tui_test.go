package tui

import "testing"

func TestIsInteractiveRespectsEnv(t *testing.T) {
	t.Setenv("ECO_NONINTERACTIVE", "1")
	if IsInteractive() {
		t.Fatal("expected non-interactive when ECO_NONINTERACTIVE=1")
	}
	if err := RequireTTY(); err != ErrNotInteractive {
		t.Fatalf("expected ErrNotInteractive, got %v", err)
	}
}
