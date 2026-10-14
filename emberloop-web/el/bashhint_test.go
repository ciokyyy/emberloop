package el

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"example.com/emberloop-web/el/tools"
)

// fakeBash returns a fixed output so the hint wrapper is tested alone.
type fakeBash struct{ out string }

func (fakeBash) Name() string           { return "bash" }
func (fakeBash) Description() string    { return "fake bash" }
func (fakeBash) Schema() map[string]any { return map[string]any{} }
func (f fakeBash) Execute(context.Context, string, json.RawMessage) (string, error) {
	return f.out, nil
}

// Seam: a stored bash output gains the filter-in-execute tip; a plain one is
// returned unchanged.
func TestBashHintTipsOnlyStoredOutput(t *testing.T) {
	stored := "[overflow] output stored as ctx:1 (9 bytes, 1 lines) from bash x\nhead\n"
	out, err := bashHintTool{inner: fakeBash{out: stored}}.Execute(context.Background(), "", nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.HasPrefix(out, stored) || !strings.Contains(out, bashOverflowTip) {
		t.Fatalf("stored output must keep its summary and gain the tip: %q", out)
	}
	plain, _ := bashHintTool{inner: fakeBash{out: "hello"}}.Execute(context.Background(), "", nil)
	if plain != "hello" {
		t.Fatalf("plain output must not change: %q", plain)
	}
}

// Seam: only a role that serves execute gets the wrapped bash.
func TestRoleServesExecute(t *testing.T) {
	with := []tools.Tool{fakeBash{}, executeNamed{}}
	if !roleServesExecute("worker", with, tools.Settings{}) {
		t.Fatal("a base list with execute must serve execute")
	}
	if roleServesExecute("worker", []tools.Tool{fakeBash{}}, tools.Settings{}) {
		t.Fatal("a base list without execute must not")
	}
}

type executeNamed struct{ fakeBash }

func (executeNamed) Name() string { return "execute" }
