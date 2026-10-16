package main

import (
	"errors"
	"testing"
)

func TestClassifyTurn(t *testing.T) {
	errTimeout := errors.New("acp session/prompt timed out after 3s")
	errClosed := errors.New("adapter closed")
	errMethod := errors.New("acp: Method not found")
	errProto := errors.New("acp: unknown session xyz")
	for _, tc := range []struct {
		name string
		in   turnOutcome
		want string
	}{
		{"ok tool+chunk", turnOutcome{updates: 3, sawToolCall: true, sawChunk: true, expectTool: true}, VerdictOK},
		{"ok chunk only no tool expected", turnOutcome{updates: 1, sawChunk: true}, VerdictOK},
		{"timeout", turnOutcome{err: errTimeout, timedOut: true}, VerdictTimeoutHang},
		{"timeout string without flag", turnOutcome{err: errTimeout}, VerdictTimeoutHang},
		{"crash flag wins", turnOutcome{err: errTimeout, timedOut: true, crashed: true}, VerdictCrash},
		{"adapter closed is crash", turnOutcome{err: errClosed}, VerdictCrash},
		{"method not found", turnOutcome{err: errMethod}, VerdictProtocolError},
		{"generic error", turnOutcome{err: errProto}, VerdictProtocolError},
		{"no updates", turnOutcome{}, VerdictSilentEmptyTurn},
		{"updates but neither tool nor chunk", turnOutcome{updates: 2}, VerdictSilentEmptyTurn},
		{"missing expected tool", turnOutcome{updates: 1, sawChunk: true, expectTool: true}, VerdictWrongResult},
		{"text match", turnOutcome{updates: 1, sawChunk: true, expectText: "hi", gotText: "say hi"}, VerdictOK},
		{"text mismatch", turnOutcome{updates: 1, sawChunk: true, expectText: "hi", gotText: "bye"}, VerdictWrongResult},
	} {
		if got := classifyTurn(tc.in); got != tc.want {
			t.Errorf("%s: got %q want %q", tc.name, got, tc.want)
		}
	}
}

func TestPickAllowOption(t *testing.T) {
	opts := []any{
		map[string]any{"optionId": "reject", "kind": "reject_once"},
		map[string]any{"optionId": "allow-once", "kind": "allow_once"},
		map[string]any{"optionId": "allow-always", "kind": "allow_always"},
	}
	if got := pickAllowOption(opts); got != "allow-once" {
		t.Fatalf("got %q", got)
	}
	if got := pickAllowOption(opts[2:3]); got != "allow-always" {
		t.Fatalf("fallback got %q", got)
	}
	if got := pickAllowOption(opts[:1]); got != "" {
		t.Fatalf("reject-only got %q", got)
	}
	if got := pickAllowOption(nil); got != "" {
		t.Fatalf("nil got %q", got)
	}
}

func TestSplitArgv(t *testing.T) {
	got, err := splitArgv(`npx -y pkg --flag 'a b' "c d" e\ f`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"npx", "-y", "pkg", "--flag", "a b", "c d", "e f"}
	if len(got) != len(want) {
		t.Fatalf("got %q", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %q want %q", got, want)
		}
	}
	if _, err := splitArgv(`'abc`); err == nil {
		t.Fatal("want unterminated quote error")
	}
}

func TestPercentile(t *testing.T) {
	lats := []float64{10, 20, 30, 40, 50}
	if p := percentile(lats, 50); p != 30 {
		t.Fatalf("p50=%v", p)
	}
	if p := percentile(lats, 95); p != 40 {
		t.Fatalf("p95=%v", p)
	}
	if p := percentile(nil, 50); p != 0 {
		t.Fatalf("empty=%v", p)
	}
}

func TestUpdateText(t *testing.T) {
	u := map[string]any{"content": map[string]any{"type": "text", "text": "hello"}}
	if got := updateText(u); got != "hello" {
		t.Fatalf("got %q", got)
	}
	u2 := map[string]any{"content": []any{
		map[string]any{"type": "text", "text": "a"},
		map[string]any{"type": "image", "data": "zz"},
	}}
	if got := updateText(u2); got != "a" {
		t.Fatalf("got %q", got)
	}
}

func TestDeriveBaseToolName(t *testing.T) {
	for in, want := range map[string]string{
		"echo": "echo", "acpbench-stub__echo": "echo",
		"mcp__acpbench-stub__echo": "echo", "stub/echo": "echo",
		"stub.echo": "echo", "ns:echo": "echo", "SLOW": "slow", "": "",
	} {
		if got := deriveBaseToolName(in); got != want {
			t.Errorf("deriveBaseToolName(%q)=%q want %q", in, got, want)
		}
	}
}

func TestDiscoverModels(t *testing.T) {
	raw := []byte(`{"sessionId":"s1","models":{"currentModelId":"fake-default",` +
		`"availableModels":[{"modelId":"fake-default"},{"modelId":"fake-alt"}]},` +
		`"configOptions":[{"id":"model","category":"model","values":["fake-default","fake-alt"]},` +
		`{"id":"thought_level","category":"model","values":["low","high"]}]}`)
	d := discoverModels(raw)
	if d.Current != "fake-default" || len(d.Models) != 2 {
		t.Fatalf("discovery: %+v", d)
	}
	cid, val, mid := modelConfigTarget(d)
	if cid == "" || val == "" || val == "fake-default" {
		t.Fatalf("target not a real different id: %q %q %q", cid, val, mid)
	}
	if d2 := discoverModels([]byte(`{"sessionId":"s1"}`)); len(d2.Models) != 0 {
		t.Fatalf("empty discovery: %+v", d2)
	}
}

func TestPickAuthMethod(t *testing.T) {
	raw := []byte(`{"authMethods":[{"id":"oauth","name":"OAuth"},` +
		`{"id":"env-key","type":"api-key","name":"env"}]}`)
	if got, ok := pickAuthMethod(raw, "auto"); !ok || got != "env-key" {
		t.Fatalf("auto got %q %v", got, ok)
	}
	if got, ok := pickAuthMethod(raw, "oauth"); !ok || got != "oauth" {
		t.Fatalf("explicit got %q %v", got, ok)
	}
	if _, ok := pickAuthMethod([]byte(`{}`), "auto"); ok {
		t.Fatal("auto with no methods should fail")
	}
}

func TestFaultClass(t *testing.T) {
	if got := faultClass("do FAULT:http500 now"); got != "http500" {
		t.Fatalf("got %q", got)
	}
	if got := faultClass("no directive"); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestReadStubLog(t *testing.T) {
	list, calls := readStubLog("/nonexistent-acpbench-log")
	if list || len(calls) != 0 {
		t.Fatal("missing log should be empty")
	}
}
