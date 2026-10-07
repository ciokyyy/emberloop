package herdrreport

import (
	"strings"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
)

// voiceCustomType labels the message the reminder rides in, so a reader can
// tell emberloop's reminder from one the user wrote.
const voiceCustomType = "emberloop-voice"

// voiceRunner asks emberloop for the per-turn reminder text. It is a type, not a
// call, so a test answers for it instead of running emberloop.
type voiceRunner func(ctx sdk.Context) (string, error)

// execVoice is the real voiceRunner: `emberloop voice-reminder`, the one source
// of the text, so the pig and claude harnesses read the same words.
func execVoice(ctx sdk.Context) (string, error) {
	res, err := ctx.Exec(gateBin(), []string{"voice-reminder"})
	if err != nil || res == nil || res.ExitCode != 0 {
		return "", err
	}
	return res.Stdout, nil
}

// registerVoice adds the register's per-turn reminder to every user message,
// the way the caveman plugin's UserPromptSubmit hook does: the rules in the
// system prompt lose to the latest message, so the reminder rides next to it.
// A reminder that cannot be had is skipped: it must never stop a turn.
func registerVoice(r eventRegistrar, run voiceRunner) {
	r.OnEvent(sdk.EventBeforeAgentStart, func(ctx sdk.Context, _ map[string]any) (any, error) {
		text, err := run(ctx)
		if err != nil || strings.TrimSpace(text) == "" {
			return nil, nil
		}
		return map[string]any{"message": map[string]any{
			"customType": voiceCustomType,
			"content":    strings.TrimSpace(text),
			"display":    false,
		}}, nil
	})
}
