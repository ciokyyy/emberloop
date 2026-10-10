package herdrreport

import (
	"errors"
	"strings"
	"testing"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
)

// voiceRegistrar keeps the handler registered for each event.
type voiceRegistrar struct{ handlers map[string]sdk.EventFunc }

func (r *voiceRegistrar) OnEvent(event string, h sdk.EventFunc) func() {
	if r.handlers == nil {
		r.handlers = map[string]sdk.EventFunc{}
	}
	r.handlers[event] = h
	return func() {}
}

// Every turn gets el's reminder as a hidden custom message, asked of el so
// both harnesses read the same words.
func TestVoiceReminderRidesEveryTurn(t *testing.T) {
	r := &voiceRegistrar{}
	registerVoice(r, func(sdk.Context) (string, error) { return "CAVEMAN MODE ACTIVE (ultra).\n", nil })
	h := r.handlers[sdk.EventBeforeAgentStart]
	if h == nil {
		t.Fatal("the reminder must hook before_agent_start")
	}
	got, err := h(sdk.Context{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	msg, _ := got.(map[string]any)["message"].(map[string]any)
	if msg["customType"] != voiceCustomType || msg["display"] != false || !strings.HasPrefix(msg["content"].(string), "CAVEMAN MODE ACTIVE") {
		t.Errorf("reminder message = %v", msg)
	}
	if strings.HasSuffix(msg["content"].(string), "\n") {
		t.Error("the reminder text must be trimmed")
	}
}

// A reminder that cannot be had never stops a turn.
func TestVoiceReminderSkippedWhenUnavailable(t *testing.T) {
	for name, run := range map[string]voiceRunner{
		"error": func(sdk.Context) (string, error) { return "", errors.New("no el") },
		"empty": func(sdk.Context) (string, error) { return " \n", nil },
	} {
		r := &voiceRegistrar{}
		registerVoice(r, run)
		got, err := r.handlers[sdk.EventBeforeAgentStart](sdk.Context{}, nil)
		if got != nil || err != nil {
			t.Errorf("%s: want no message and no error, got %v, %v", name, got, err)
		}
	}
}
