package herdrreport

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
)

// fakeGate answers settle attempts from a scripted list of decisions and keeps
// the event JSON it was asked with, so a test drives the extension's handlers
// directly, the way pig drives them.
type fakeGate struct {
	answers []string
	events  []gateInput
	err     error
}

func (f *fakeGate) run(_ sdk.Context, event string) (string, error) {
	var in gateInput
	if err := json.Unmarshal([]byte(event), &in); err != nil {
		return "", err
	}
	f.events = append(f.events, in)
	if f.err != nil {
		return "", f.err
	}
	if len(f.answers) == 0 {
		return `{"outcome":"allow"}`, nil
	}
	answer := f.answers[0]
	f.answers = f.answers[1:]
	return answer, nil
}

// settleData is one agent_before_settle event: the activity outcome and the
// turn's last assistant message, which the decision quotes on a give-up.
func settleData(outcome, text string) map[string]any {
	return map[string]any{
		"type":    "agent_before_settle",
		"outcome": outcome,
		"context": map[string]any{
			"contextMessages": []any{
				map[string]any{"role": "user", "content": "do the work"},
				map[string]any{"role": "assistant", "content": []any{
					map[string]any{"type": "text", "text": text},
				}},
			},
		},
	}
}

// continued reads the boundary answer a block returns: the reminder rides as a
// custom message the model reads, and continue keeps the agent running.
func continued(t *testing.T, result any) string {
	t.Helper()
	answer, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("a block must answer with a boundary result, got %#v", result)
	}
	if answer["continue"] != true {
		t.Errorf("a block must continue the agent, got %#v", answer)
	}
	entries, ok := answer["entries"].([]any)
	if !ok || len(entries) != 1 {
		t.Fatalf("a block must carry exactly one entry, got %#v", answer)
	}
	entry, _ := entries[0].(map[string]any)
	if entry["type"] != "custom_message" || entry["customType"] != gateCustomType || entry["display"] != false {
		t.Errorf("the reminder must ride as the gate's own custom message, got %#v", entry)
	}
	content, _ := entry["content"].(string)
	if content == "" {
		t.Errorf("the reminder must be the entry's content, got %#v", entry)
	}
	return content
}

// No submit: the handler continues with reminder 1, 2 and 3 — pinning on the
// third — and lets the agent settle on the fourth.
func TestBeforeSettleContinuesWithTheReminderThenSettlesOnGiveUp(t *testing.T) {
	fake := &fakeGate{answers: []string{
		`{"outcome":"block","reminder":"reminder 1 of 3: keep working"}`,
		`{"outcome":"block","reminder":"reminder 2 of 3: keep working"}`,
		`{"outcome":"pin","reminder":"reminder 3 of 3: submit is pinned"}`,
		`{"outcome":"giveup"}`,
	}}
	g := newGate(fake.run)

	for _, want := range []string{"reminder 1 of 3", "reminder 2 of 3", "reminder 3 of 3"} {
		result, err := g.beforeSettle(sdk.Context{}, settleData("completed", "I am stuck on the flaky test"))
		if err != nil {
			t.Fatalf("beforeSettle: %v", err)
		}
		if got := continued(t, result); !strings.Contains(got, want) {
			t.Errorf("the handler must continue with %q, got %q", want, got)
		}
	}

	result, err := g.beforeSettle(sdk.Context{}, settleData("completed", "out of ideas"))
	if err != nil {
		t.Fatalf("beforeSettle: %v", err)
	}
	if result != nil {
		t.Errorf("a give-up must let the agent settle, got %#v", result)
	}

	if len(fake.events) != 4 {
		t.Fatalf("the gate must be asked once per settle, got %d", len(fake.events))
	}
	if !fake.events[0].Fresh {
		t.Error("the first settle of a turn is fresh")
	}
	for i, in := range fake.events[1:] {
		if in.Fresh {
			t.Errorf("settle %d must not start the attempt count over", i+2)
		}
	}
	if got := fake.events[3].LastMessage; got != "out of ideas" {
		t.Errorf("the gate must hear the last message, got %q", got)
	}
}

// A new turn starts the attempt count over, so its first settle is fresh again.
func TestAfterANewTurnTheNextSettleIsFresh(t *testing.T) {
	fake := &fakeGate{}
	g := newGate(fake.run)
	if _, err := g.beforeSettle(sdk.Context{}, settleData("completed", "one")); err != nil {
		t.Fatal(err)
	}
	if _, err := g.turnStarted(sdk.Context{}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := g.beforeSettle(sdk.Context{}, settleData("completed", "two")); err != nil {
		t.Fatal(err)
	}
	if len(fake.events) != 2 || !fake.events[0].Fresh || !fake.events[1].Fresh {
		t.Errorf("a settle after a new turn must be fresh, got %#v", fake.events)
	}
}

// registrar records the events an extension handler is registered for.
type registrar struct{ events []string }

func (r *registrar) OnEvent(event string, _ sdk.EventFunc) func() {
	r.events = append(r.events, event)
	return func() {}
}

// The reset signal is before_agent_start, never agent_start: pig emits
// agent_start again for the low-level run that continues a blocked settle, so a
// reset there would start the count over every attempt — reminder 1 forever —
// and drop the pin before the request it belongs to.
func TestGateResetsOnTheSignalAContinuationNeverEmits(t *testing.T) {
	r := &registrar{}
	newGate((&fakeGate{}).run).register(r)
	want := []string{sdk.EventBeforeAgentStart, sdk.EventAgentBeforeSettle, sdk.EventBeforeProviderRequest}
	if !reflect.DeepEqual(r.events, want) {
		t.Errorf("the gate must register %v, got %v", want, r.events)
	}
	for _, event := range r.events {
		if event == sdk.EventAgentStart {
			t.Error("agent_start fires again for the run that continues a blocked settle; it must not reset the gate")
		}
	}
}

// A submit recorded this turn, and a turn that ended in a provider error, both
// let the agent settle at once.
func TestBeforeSettleSettlesOnASubmitAndOnAProviderError(t *testing.T) {
	fake := &fakeGate{answers: []string{`{"outcome":"allow"}`, `{"outcome":"allow"}`}}
	g := newGate(fake.run)
	if result, err := g.beforeSettle(sdk.Context{}, settleData("completed", "shipped")); err != nil || result != nil {
		t.Errorf("an allowed settle must settle, got %#v, %v", result, err)
	}
	if result, err := g.beforeSettle(sdk.Context{}, settleData("error", "provider blew up")); err != nil || result != nil {
		t.Errorf("a provider error must settle, got %#v, %v", result, err)
	}
	if !fake.events[1].Failed {
		t.Error("the gate must hear that the turn ended in a provider error")
	}
	if got := fake.events[1].Error; got != "provider blew up" {
		t.Errorf("the gate must forward the provider error text, got %q", got)
	}
	if fake.events[0].Failed {
		t.Error("a completed turn is not a provider error")
	}
	if fake.events[0].Error != "" {
		t.Errorf("a completed turn carries no error, got %q", fake.events[0].Error)
	}
}

// A gate that cannot answer must never hold an agent at its settle.
func TestBeforeSettleSettlesWhenTheDecisionCannotBeAsked(t *testing.T) {
	g := newGate(func(sdk.Context, string) (string, error) {
		return "", errors.New("emberloop pig-gate: not found")
	})
	result, err := g.beforeSettle(sdk.Context{}, settleData("completed", "done"))
	if err != nil || result != nil {
		t.Errorf("a failed ask must settle, got %#v, %v", result, err)
	}
}

// The pin forces submit on the request that follows the pinned attempt, in the
// shape the provider's own wire body uses, and only that one request.
func TestBeforeProviderRequestPinsSubmitOnThePinnedRequestOnly(t *testing.T) {
	anthropic := map[string]any{
		"model":      "claude-fable-5",
		"max_tokens": float64(4096),
		"messages":   []any{map[string]any{"role": "user", "content": "go"}},
		"tools": []any{
			map[string]any{"name": "read", "input_schema": map[string]any{"type": "object"}},
			map[string]any{"name": "mcp__emberloop__submit", "input_schema": map[string]any{"type": "object"}},
		},
	}
	g := newGate((&fakeGate{answers: []string{`{"outcome":"pin","reminder":"reminder 3 of 3"}`}}).run)

	before, err := json.Marshal(anthropic)
	if err != nil {
		t.Fatal(err)
	}
	// No pinned attempt yet: the payload must come back untouched.
	result, err := g.beforeRequest(sdk.Context{}, map[string]any{"type": "before_provider_request", "payload": anthropic})
	if err != nil || result != nil {
		t.Fatalf("an unpinned request must be left alone, got %#v, %v", result, err)
	}
	if after, err := json.Marshal(anthropic); err != nil || string(after) != string(before) {
		t.Fatalf("an unpinned request was changed:\n%s\n%s (%v)", before, after, err)
	}

	if _, err := g.beforeSettle(sdk.Context{}, settleData("completed", "stuck")); err != nil {
		t.Fatal(err)
	}
	result, err = g.beforeRequest(sdk.Context{}, map[string]any{"type": "before_provider_request", "payload": anthropic})
	if err != nil {
		t.Fatalf("beforeRequest: %v", err)
	}
	pinned, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("the pinned request must replace the payload, got %#v", result)
	}
	want := map[string]any{"type": "tool", "name": "mcp__emberloop__submit"}
	if !reflect.DeepEqual(pinned["tool_choice"], want) {
		t.Errorf("anthropic tool_choice = %#v, want %#v", pinned["tool_choice"], want)
	}
	for key, value := range anthropic {
		if key == "tool_choice" {
			continue
		}
		if !reflect.DeepEqual(pinned[key], value) {
			t.Errorf("the pin changed %q: %#v", key, value)
		}
	}

	// Only the request that follows the pinned attempt is pinned.
	if result, err := g.beforeRequest(sdk.Context{}, map[string]any{"payload": anthropic}); err != nil || result != nil {
		t.Errorf("only one request may be pinned, got %#v, %v", result, err)
	}
}

// The pin names the submit tool in each provider's own shape, because the wire
// body of every provider spells its tool choice differently.
func TestPinNamesSubmitInEachProvidersShape(t *testing.T) {
	cases := []struct {
		name    string
		payload map[string]any
		key     string
		want    any
	}{
		{
			name: "anthropic messages",
			payload: map[string]any{
				"messages": []any{},
				"tools":    []any{map[string]any{"name": "mcp__emberloop__submit", "input_schema": map[string]any{}}},
			},
			key:  "tool_choice",
			want: map[string]any{"type": "tool", "name": "mcp__emberloop__submit"},
		},
		{
			name: "openai chat",
			payload: map[string]any{
				"messages": []any{},
				"tools": []any{map[string]any{
					"type":     "function",
					"function": map[string]any{"name": "mcp__emberloop__submit"},
				}},
			},
			key:  "tool_choice",
			want: map[string]any{"type": "function", "function": map[string]any{"name": "mcp__emberloop__submit"}},
		},
		{
			name: "openai responses",
			payload: map[string]any{
				"input": []any{},
				"tools": []any{map[string]any{"type": "function", "name": "mcp__emberloop__submit"}},
			},
			key:  "tool_choice",
			want: map[string]any{"type": "function", "name": "mcp__emberloop__submit"},
		},
		{
			name: "gemini",
			payload: map[string]any{
				"contents":   []any{},
				"toolConfig": map[string]any{},
				"tools": []any{map[string]any{"functionDeclarations": []any{
					map[string]any{"name": "mcp__emberloop__submit"},
				}}},
			},
			key: "toolConfig",
			want: map[string]any{"functionCallingConfig": map[string]any{
				"mode":                 "ANY",
				"allowedFunctionNames": []any{"mcp__emberloop__submit"},
			}},
		},
		{
			name: "mistral",
			payload: map[string]any{
				"messages":   []any{},
				"toolChoice": "auto",
				"tools":      []any{map[string]any{"function": map[string]any{"name": "mcp__emberloop__submit"}}},
			},
			key:  "toolChoice",
			want: map[string]any{"type": "function", "function": map[string]any{"name": "mcp__emberloop__submit"}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := newGate((&fakeGate{answers: []string{`{"outcome":"pin","reminder":"reminder 3 of 3"}`}}).run)
			if _, err := g.beforeSettle(sdk.Context{}, settleData("completed", "stuck")); err != nil {
				t.Fatal(err)
			}
			result, err := g.beforeRequest(sdk.Context{}, map[string]any{"payload": c.payload})
			if err != nil {
				t.Fatalf("beforeRequest: %v", err)
			}
			pinned, ok := result.(map[string]any)
			if !ok {
				t.Fatalf("the pinned request must replace the payload, got %#v", result)
			}
			if got := pinned[c.key]; !reflect.DeepEqual(got, c.want) {
				t.Errorf("%s = %#v, want %#v", c.key, got, c.want)
			}
		})
	}
}

// A body that does not list the submit tool is left alone: the pin is skipped
// rather than guessed.
func TestPinIsSkippedWhenTheBodyListsNoSubmitTool(t *testing.T) {
	fake := &fakeGate{answers: []string{`{"outcome":"pin","reminder":"reminder 3 of 3"}`}}
	g := newGate(fake.run)
	if _, err := g.beforeSettle(sdk.Context{}, settleData("completed", "stuck")); err != nil {
		t.Fatal(err)
	}
	if result, err := g.beforeRequest(sdk.Context{}, map[string]any{"payload": map[string]any{"messages": []any{}}}); err != nil || result != nil {
		t.Errorf("a body without the submit tool must be left alone, got %#v, %v", result, err)
	}
}
