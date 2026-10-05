package wick

import (
	"os"
	"path/filepath"
	"testing"
)

// resolverSettings points the settings store at a temp file and clears the env
// overrides, so a test reads only the file it writes. An empty body leaves the
// file missing.
func resolverSettings(t *testing.T, body string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "settings.json")
	t.Setenv("WICK_SETTINGS", p)
	t.Setenv("WICK_DECISION_MODEL", "")
	t.Setenv("WICK_FIND_THRESHOLD", "")
	// The read-only harness override is the resolver's flag layer and it wins
	// over any slot: clear it, so a launch reads only what the test writes.
	t.Setenv("WICK_READONLY_HARNESS", "")
	if body != "" {
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// Resolve applies each precedence layer per field, independently: a flag beats
// the type or spawn argument, which beats the settings file, which beats the
// built-in default. A missing or empty file still resolves to the default.
func TestResolvePrecedencePerField(t *testing.T) {
	cases := []struct {
		name  string
		file  string
		flags Choice
		typ   Choice
		want  Choice
	}{
		{
			name: "missing file: every field is the built-in default",
			file: "",
			want: Choice{Harness: "pig"},
		},
		{
			name: "empty file: every field is the built-in default",
			file: "{}",
			want: Choice{Harness: "pig"},
		},
		{
			name: "the settings file supplies every field",
			file: `{"roles":{"implementer":{"harness":"claude","model":"file-model","effort":"low"}}}`,
			want: Choice{Harness: "claude", Model: "file-model", Effort: "low"},
		},
		{
			name: "the type or spawn argument beats the settings file",
			file: `{"roles":{"implementer":{"harness":"claude","model":"file-model","effort":"low"}}}`,
			typ:  Choice{Harness: "pig", Model: "type-model", Effort: "high"},
			want: Choice{Harness: "pig", Model: "type-model", Effort: "high"},
		},
		{
			name:  "the flag beats the type or spawn argument",
			file:  `{"roles":{"implementer":{"harness":"claude","model":"file-model","effort":"low"}}}`,
			flags: Choice{Harness: "pig", Model: "flag-model", Effort: "xhigh"},
			typ:   Choice{Harness: "claude", Model: "type-model", Effort: "high"},
			want:  Choice{Harness: "pig", Model: "flag-model", Effort: "xhigh"},
		},
		{
			name:  "each field resolves on its own layer",
			file:  `{"roles":{"implementer":{"harness":"claude","model":"file-model","effort":"low"}}}`,
			flags: Choice{Model: "flag-model"},
			typ:   Choice{Effort: "medium"},
			want:  Choice{Harness: "claude", Model: "flag-model", Effort: "medium"},
		},
		{
			name: "only the named slot's settings apply",
			file: `{"roles":{"orchestrator":{"harness":"claude","model":"o","effort":"high"}}}`,
			want: Choice{Harness: "pig"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resolverSettings(t, c.file)
			if got := Resolve("implementer", c.flags, c.typ); got != c.want {
				t.Errorf("Resolve = %+v, want %+v", got, c.want)
			}
		})
	}
}
