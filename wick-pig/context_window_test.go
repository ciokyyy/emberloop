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

func TestModelWindowProviderAndHarnessPrefixes(t *testing.T) {
	cases := []struct {
		model string
		want  int64
	}{
		{"openrouter/deepseek/deepseek-v4-flash-0731", 1000000},
		{"antigravity-gemini-3.8-flash", 1000000},
		{"OPENROUTER/DEEPSEEK/DEEPSEEK-V4-FLASH-0731", 1000000},
		{"ANTIGRAVITY-GEMINI-3.8-FLASH", 1000000},
		{"openrouter/provider/sub/deepseek-v4", 1000000},
		{"deepseek-v4-flash", 1000000},
		{"gemini-3.8-flash", 1000000},
		{"openrouter/mystery-model", 200000},
		{"antigravity-unknown-model", 200000},
		{"unknown-model", 200000},
	}
	for _, tc := range cases {
		if got := ModelWindow(tc.model, nil); got != tc.want {
			t.Errorf("ModelWindow(%q, nil) = %d, want %d", tc.model, got, tc.want)
		}
	}

	// Exact override beats table
	overrides := map[string]int64{
		"openrouter/deepseek/deepseek-v4-flash-0731": 500000,
		"antigravity-gemini-3.8-flash":               2000000,
	}
	if got := ModelWindow("openrouter/deepseek/deepseek-v4-flash-0731", overrides); got != 500000 {
		t.Errorf("override for openrouter model = %d, want 500000", got)
	}
	if got := ModelWindow("antigravity-gemini-3.8-flash", overrides); got != 2000000 {
		t.Errorf("override for antigravity model = %d, want 2000000", got)
	}
}
