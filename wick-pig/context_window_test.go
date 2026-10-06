package wick

import "testing"

// Seam: model window table. A model's window resolves from the settings
// overrides first, then the built-in table, else the 200k default. Expected
// windows come from the documented harness context sizes, never the code.

// ModelWindow is tested through the table: exact override wins, then the
// built-in family, then the default.
func TestModelWindowResolution(t *testing.T) {
	if got := ModelWindow("claude-opus-5", nil); got != 200000 {
		t.Errorf("opus window = %d, want 200000", got)
	}
	if got := ModelWindow("mystery-model", nil); got != 200000 {
		t.Errorf("unknown window = %d, want default 200000", got)
	}
	over := map[string]int64{"mystery-model": 1000000}
	if got := ModelWindow("mystery-model", over); got != 1000000 {
		t.Errorf("override window = %d, want 1000000", got)
	}
	over["claude-opus-5"] = 500000
	if got := ModelWindow("claude-opus-5", over); got != 500000 {
		t.Errorf("override beats builtin = %d, want 500000", got)
	}
}
