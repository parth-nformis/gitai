package commands

import "testing"

// Compile-time check: Hook must satisfy the Handler interface
// (Run is promoted from the embedded Commit).
var _ Handler = (*Hook)(nil)

func TestHookName(t *testing.T) {
	if got := (&Hook{}).Name(); got != "hook" {
		t.Errorf("Name() = %q, want %q", got, "hook")
	}
}
