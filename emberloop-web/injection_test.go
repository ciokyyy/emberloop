package web

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// injectionEventsOf returns the injection Events of a transcript, decoded.
func injectionEventsOf(t *testing.T, evs []Event) []InjectionEvent {
	t.Helper()
	var out []InjectionEvent
	for _, e := range evs {
		if e.Kind != "injection" {
			continue
		}
		if e.Sender != senderSystem {
			t.Errorf("injection Event sender = %q, want system", e.Sender)
		}
		if e.User != "" {
			t.Errorf("injection Event user = %q, want empty: nothing injected is a user message", e.User)
		}
		ie, ok := parseInjectionEvent(e.Text)
		if !ok {
			t.Fatalf("injection Event text is not JSON: %q", e.Text)
		}
		out = append(out, ie)
	}
	return out
}

// injectHook replaces the single injection hook until the test ends. One
// hook returns texts, Profile version and refs together, so the three
// cannot disagree.
func injectHook(t *testing.T, hook func(SessionMeta, handleOp) injection) {
	t.Helper()
	prev := memoryInjection
	memoryInjection = hook
	t.Cleanup(func() { memoryInjection = prev })
}

// createOnly returns hook for create only: resumed turns inject nothing.
func createOnly(in injection) func(SessionMeta, handleOp) injection {
	return func(_ SessionMeta, op handleOp) injection {
		if op == handleCreate {
			return in
		}
		return injection{}
	}
}

func TestInjectionEventOnEveryInjection(t *testing.T) {
	refs := []InjectedRule{{ID: "r1", Version: 1}, {ID: "r2", Version: 3}}
	injectHook(t, createOnly(injection{texts: []string{"PROFILE", "RECALL"}, profile: 4, refs: refs}))
	d, _, seen, base, id := handleDaemon(t, nil)
	runPrompt(t, base, id, "build the thing")
	waitReleased(t, d, id)

	if got := seen.all(); len(got) != 1 || got[0] != "PROFILE\n\nRECALL\n\nbuild the thing" {
		t.Fatalf("prompts received = %q, want the block ahead of the prompt", got)
	}
	ies := injectionEventsOf(t, transcriptOf(t, base, id))
	if len(ies) != 1 {
		t.Fatalf("injection Events = %d, want 1 per injection", len(ies))
	}
	if ies[0].ProfileVersion != 4 || len(ies[0].Rules) != 2 ||
		ies[0].Rules[0] != refs[0] || ies[0].Rules[1] != refs[1] {
		t.Fatalf("injection Event = %+v, want profile 4 and rules %+v", ies[0], refs)
	}

	// The resumed turn carries no injection and logs no second Event.
	runPrompt(t, base, id, "and more")
	ies = injectionEventsOf(t, transcriptOf(t, base, id))
	if len(ies) != 1 {
		t.Fatalf("injection Events after a plain turn = %d, want 1", len(ies))
	}
}

func TestNoInjectionLogsNoEvent(t *testing.T) {
	t.Parallel()
	_, _, _, base, id := handleDaemon(t, nil)
	runPrompt(t, base, id, "build the thing")
	for _, e := range transcriptOf(t, base, id) {
		if e.Kind == "injection" {
			t.Fatalf("injection Event without an injection: %+v", e)
		}
	}
}

func TestCompressLogsInjectionEvent(t *testing.T) {
	injectHook(t, func(_ SessionMeta, op handleOp) injection {
		if op == handleCompress {
			return injection{texts: []string{"PROFILE", "RECALL"}, profile: 2, refs: []InjectedRule{{ID: "rc", Version: 1}}}
		}
		return injection{}
	})
	_, _, seen, base, id := handleDaemon(t, nil)
	runPrompt(t, base, id, "build the thing")
	compressAndSettle(t, base, id, 1)

	ies := injectionEventsOf(t, transcriptOf(t, base, id))
	if len(ies) != 1 {
		t.Fatalf("injection Events after compress = %d, want 1", len(ies))
	}
	if ies[0].ProfileVersion != 2 || len(ies[0].Rules) != 1 || ies[0].Rules[0].ID != "rc" {
		t.Fatalf("compress injection Event = %+v", ies[0])
	}
	handoff := handoffText(transcriptOf(t, base, id))
	if got := seen.all(); got[len(got)-1] != "PROFILE\n\nRECALL\n\n"+handoff {
		t.Fatalf("compress turn = %q, want the block ahead of the Handoff", got[len(got)-1])
	}
}

func TestMinerReadableOnlyUserPrompts(t *testing.T) {
	t.Parallel()
	user := Event{Kind: "prompt", Sender: "user", User: "u1", Text: "fix it"}
	cases := []struct {
		name string
		e    Event
		want bool
	}{
		{"plain user prompt", user, true},
		{"user steer", Event{Kind: "prompt", Mode: "steer", Sender: "user", User: "u1"}, true},
		{"user followup", Event{Kind: "prompt", Mode: "followup", Sender: "user", User: "u2"}, true},
		{"system prompt", Event{Kind: "prompt", Sender: "system"}, false},
		{"grouped result", Event{Kind: "prompt", Mode: "steer", Sender: "system"}, false},
		{"handoff", Event{Kind: "prompt", Sender: "system", Handoff: true}, false},
		{"injection", Event{Kind: "injection", Sender: "system", Text: InjectionEvent{Rules: []InjectedRule{{ID: "r", Version: 1}}}.text()}, false},
		{"assistant", Event{Kind: "assistant_chunk", Text: "fix it"}, false},
		{"tool call", Event{Kind: "tool_call_started", Text: "{}"}, false},
		{"compressed", Event{Kind: "compressed", Text: "{}"}, false},
		{"status", Event{Kind: "status", Status: "idle"}, false},
	}
	for _, c := range cases {
		if got := MinerReadable(c.e); got != c.want {
			t.Errorf("%s: MinerReadable = %v, want %v", c.name, got, c.want)
		}
	}
	if !strings.Contains(InjectionEvent{}.text(), `"rules":[]`) {
		t.Errorf("empty injection Event = %s, want an explicit empty rules array", InjectionEvent{}.text())
	}
}

func TestInjectionEventSeparatesMemoryIDs(t *testing.T) {
	e := InjectionEvent{ProfileVersion: 2, Rules: []InjectedRule{{ID: "rule-1", Version: 3}, {MemoryID: 41}, {MemoryID: 42}}}
	got, ok := parseInjectionEvent(e.text())
	if !ok || len(got.Rules) != 1 || got.Rules[0].ID != "rule-1" || len(got.MemoryIDs) != 2 || got.MemoryIDs[0] != 41 || got.MemoryIDs[1] != 42 {
		t.Fatalf("event=%+v raw=%s", got, e.text())
	}
}

func TestFilterMinerEventsDropsEverythingButUserPrompts(t *testing.T) {
	t.Parallel()
	events := []Event{
		{Kind: "prompt", Sender: "user", User: "u1", Text: "keep me"},
		{Kind: "prompt", Mode: "steer", Sender: "user", User: "u2", Text: "keep me too"},
		{Kind: "prompt", Sender: "system", Text: "grouped result"},
		{Kind: "prompt", Sender: "system", Handoff: true, Text: "# Handoff"},
		{Kind: "injection", Sender: "system", Text: InjectionEvent{Rules: []InjectedRule{{ID: "r", Version: 1}}}.text()},
		{Kind: "assistant_chunk", Text: "reply"},
		{Kind: "compressed", Text: "{}"},
	}
	got := FilterMinerEvents(events)
	if len(got) != 2 || got[0].Text != "keep me" || got[1].Text != "keep me too" {
		t.Fatalf("FilterMinerEvents kept %d Events, want the 2 user prompts: %+v", len(got), got)
	}
}

// minerGuarded reports whether a source file that mines Sessions filters
// through MinerReadable or FilterMinerEvents: a file naming the miner and
// the event log must name the filter.
func minerGuarded(src string) bool {
	lowered := strings.ToLower(src)
	mines := strings.Contains(lowered, "miner") || strings.Contains(lowered, "mine session") ||
		strings.Contains(lowered, "mining")
	readsLog := strings.Contains(src, "events.jsonl") || strings.Contains(src, "FilterMinerEvents")
	if !mines || !readsLog {
		return true
	}
	return strings.Contains(src, "MinerReadable") || strings.Contains(src, "FilterMinerEvents")
}

func TestMinerReadsThroughFilter(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]bool{
		"// Miner scans events.jsonl\nfunc Mine() { FilterMinerEvents(evs) }":   true,
		"// Miner scans events.jsonl\nfunc Mine() { for _, e := range evs {} }": false,
		"package web\nfunc load() {}":                                           true,
		"// mining settled sessions\nx := MinerReadable(e)":                     true,
	} {
		if got := minerGuarded(in); got != want {
			t.Errorf("minerGuarded(%q) = %v, want %v", in, got, want)
		}
	}
	// The repo run: any non-test Go file in this module that mines the
	// event log must filter through MinerReadable/FilterMinerEvents, so the
	// ticket-19 miner cannot read injections raw.
	err := filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !minerGuarded(string(raw)) {
			t.Errorf("%s mines the event log without MinerReadable/FilterMinerEvents", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
