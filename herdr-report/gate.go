package herdrreport

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync/atomic"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
)

// The stop gate. A spawned pig agent cannot settle without calling submit: on
// every agent_before_settle the extension asks wick's shared decision and, on
// a block, keeps the agent running with the reminder instead of letting it
// settle. On the attempt the decision pins, the next provider request forces
// the submit tool — the pin pig can do at the API level and claude cannot. The
// decision itself, including the "stopped without submitting" notice, lives in
// `wick-pig pig-gate`, so every harness follows one set of rules.
const (
	gateOutcomeBlock = "block"
	gateOutcomePin   = "pin"
	// gateCustomType labels the custom message the reminder rides in, so a
	// reader can tell the gate's message from one the user wrote.
	gateCustomType = "wick-stop-gate"
	// submitSegment is the last `__`-separated segment of wick's submit tool,
	// which providers list as `mcp__<server>__submit`.
	submitSegment = "submit"
)

// gateInput is one settle attempt, the JSON `wick-pig pig-gate` reads. It
// mirrors wick.PigGateInput, which is the same contract on the other side of
// the process boundary.
type gateInput struct {
	Fresh       bool   `json:"fresh"`
	Failed      bool   `json:"failed"`
	LastMessage string `json:"last_message"`
}

// gateDecision is what `wick-pig pig-gate` answers.
type gateDecision struct {
	Outcome  string `json:"outcome"`
	Reminder string `json:"reminder,omitempty"`
}

// eventRegistrar is the part of an extension the gate registers on. It is an
// interface so a test can read the events the gate takes: the reset signal is
// the one thing about the gate that cannot be seen from its handlers.
type eventRegistrar interface {
	OnEvent(event string, handler sdk.EventFunc) func()
}

// gateRunner asks wick's decision for one settle attempt. It is a type, not a
// call, so a test answers for it instead of running wick-pig.
type gateRunner func(ctx sdk.Context, event string) (string, error)

// gate is the whole gate half of the extension: the next settle's freshness,
// the request that must pin the submit tool, and the run that asks the
// decision.
type gate struct {
	run gateRunner
	// fresh reports that a new turn began since the last attempt, so the next
	// settle starts the attempt count over.
	fresh atomic.Bool
	// pin reports that the decision pinned this attempt: the next provider
	// request forces submit, and only that one does.
	pin atomic.Bool
}

// newGate returns the gate the extension registers. A test builds one with a
// runner of its own and calls the handlers directly.
func newGate(run gateRunner) *gate {
	g := &gate{run: run}
	g.fresh.Store(true)
	return g
}

// register wires the gate's three events. A new turn is before_agent_start and
// deliberately not agent_start: pig emits agent_start again for the low-level
// run that continues a blocked settle, so a reset there would replay reminder 1
// forever and drop the pin before its request.
func (g *gate) register(r eventRegistrar) {
	r.OnEvent(sdk.EventBeforeAgentStart, g.turnStarted)
	r.OnEvent(sdk.EventAgentBeforeSettle, g.beforeSettle)
	r.OnEvent(sdk.EventBeforeProviderRequest, g.beforeRequest)
}

// turnStarted is the before_agent_start handler: a new turn starts the attempt
// count over and drops a pin the request for it never came for.
func (g *gate) turnStarted(sdk.Context, map[string]any) (any, error) {
	g.pin.Store(false)
	g.fresh.Store(true)
	return nil, nil
}

// beforeSettle is the agent_before_settle handler. It asks the decision; a
// block (or a pin) continues the agent with the reminder, and anything else
// lets it settle. A decision that cannot be asked lets it settle too: a gate
// that cannot answer must never leave an agent unable to stop.
func (g *gate) beforeSettle(ctx sdk.Context, data map[string]any) (any, error) {
	event, err := json.Marshal(gateInput{
		Fresh:       g.fresh.Swap(false),
		Failed:      data["outcome"] == "error",
		LastMessage: lastAssistantText(data),
	})
	if err != nil {
		return nil, nil
	}
	answer, err := g.run(ctx, string(event))
	if err != nil {
		return nil, nil
	}
	var decision gateDecision
	if json.Unmarshal([]byte(answer), &decision) != nil || strings.TrimSpace(decision.Reminder) == "" {
		return nil, nil
	}
	switch decision.Outcome {
	case gateOutcomePin:
		g.pin.Store(true)
		return continueWith(decision.Reminder), nil
	case gateOutcomeBlock:
		return continueWith(decision.Reminder), nil
	default:
		// allow and giveup both let the agent settle, and a give-up has
		// already sent the parent its "stopped without submitting" notice.
		return nil, nil
	}
}

// beforeRequest is the before_provider_request handler. It forces the submit
// tool on the request that follows the pinned attempt, and returns nothing for
// every other request, so the host keeps those payloads exactly as they were.
func (g *gate) beforeRequest(_ sdk.Context, data map[string]any) (any, error) {
	if !g.pin.Swap(false) {
		return nil, nil
	}
	payload, _ := data["payload"].(map[string]any)
	name := submitToolName(payload)
	if payload == nil || name == "" {
		return nil, nil
	}
	return pinToolChoice(payload, name), nil
}

// continueWith is the boundary answer that keeps a pig agent running: the
// reminder rides as a custom message, which the model reads as a user turn,
// and continue makes the host run that turn instead of settling.
func continueWith(reminder string) map[string]any {
	return map[string]any{
		"continue": true,
		"entries": []any{map[string]any{
			"type":       "custom_message",
			"customType": gateCustomType,
			"content":    reminder,
			"display":    false,
		}},
	}
}

// execGate is the real gateRunner: it asks `wick-pig pig-gate`, the way the
// claude Stop hook asks `wick-pig stop-gate`. The event is one argv, so no
// shell sees it.
func execGate(ctx sdk.Context, event string) (string, error) {
	res, err := ctx.Exec(gateBin(), []string{"pig-gate", event})
	if err != nil {
		return "", err
	}
	if res == nil {
		return "", fmt.Errorf("%s pig-gate: no result", gateBin())
	}
	if res.ExitCode != 0 {
		return "", fmt.Errorf("%s pig-gate: %s", gateBin(), strings.TrimSpace(res.Stderr))
	}
	return res.Stdout, nil
}

// gateBin is the wick-pig that answers the gate, found on PATH as claude's
// hook finds it. The env var names a whole executable.
func gateBin() string {
	if bin := os.Getenv("WICK_PIG_BIN"); bin != "" {
		return bin
	}
	return "wick-pig"
}

// lastAssistantText is the text of the turn's final assistant message, which a
// give-up quotes. The boundary event carries the projected context, so it is
// the message the turn ended with.
func lastAssistantText(data map[string]any) string {
	context, _ := data["context"].(map[string]any)
	messages, _ := context["contextMessages"].([]any)
	for i := len(messages) - 1; i >= 0; i-- {
		message, _ := messages[i].(map[string]any)
		if message["role"] != "assistant" {
			continue
		}
		return messageText(message["content"])
	}
	return ""
}

// messageText joins the text of an assistant message's content blocks. A
// message whose content is a plain string is that string.
func messageText(content any) string {
	switch value := content.(type) {
	case string:
		return value
	case []any:
		parts := make([]string, 0, len(value))
		for _, raw := range value {
			block, _ := raw.(map[string]any)
			if block["type"] != "text" {
				continue
			}
			if text, ok := block["text"].(string); ok && text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

// submitToolName finds wick's submit tool in the provider's own tool list, so
// the pin names the tool exactly as the model knows it. A body with no such
// tool yields "" and the pin is skipped rather than guessed.
func submitToolName(payload map[string]any) string {
	exact, suffixed := "", ""
	forEachToolName(payload, func(name string) {
		switch {
		case name == submitSegment:
			exact = name
		case strings.HasSuffix(name, "__"+submitSegment) && suffixed == "":
			suffixed = name
		}
	})
	if exact != "" {
		return exact
	}
	return suffixed
}

// forEachToolName visits every tool name in a wire body. Each provider nests
// them differently: anthropic, mistral, gemini's declarations and the openai
// responses API name the tool directly, the openai chat API nests it under
// `function`, and bedrock under `toolSpec`.
func forEachToolName(payload map[string]any, visit func(string)) {
	tools, _ := payload["tools"].([]any)
	for _, raw := range tools {
		tool, _ := raw.(map[string]any)
		if tool == nil {
			continue
		}
		if name, ok := tool["name"].(string); ok {
			visit(name)
		}
		for _, nested := range []string{"function", "toolSpec"} {
			inner, _ := tool[nested].(map[string]any)
			if name, ok := inner["name"].(string); ok {
				visit(name)
			}
		}
		declarations, _ := tool["functionDeclarations"].([]any)
		for _, raw := range declarations {
			declaration, _ := raw.(map[string]any)
			if name, ok := declaration["name"].(string); ok {
				visit(name)
			}
		}
	}
}

// pinToolChoice forces name as the provider's only tool choice, in the shape
// that provider's wire body uses. The body says which shape it wants: gemini
// carries toolConfig, the openai responses API carries input, mistral carries
// toolChoice, an anthropic body lists tools with input_schema, and every other
// openai-compatible body takes an openai chat choice.
//
// A pin needs the tool's name, and only a body that lists its tools carries
// one. A pi-messages body is {model, context:{messages}, options}: its tools
// ride inside the transcript's system message and its choice field is
// provider-neutral, so there is no name to read and no shape to write. Such an
// agent is blocked and reminded like any other and only skips the pin.
func pinToolChoice(payload map[string]any, name string) map[string]any {
	if name == "" {
		return payload
	}
	choice := any(map[string]any{"type": "function", "function": map[string]any{"name": name}})
	switch {
	case payload["toolConfig"] != nil:
		config, _ := payload["toolConfig"].(map[string]any)
		if config == nil {
			config = map[string]any{}
		}
		config["functionCallingConfig"] = map[string]any{
			"mode":                 "ANY",
			"allowedFunctionNames": []any{name},
		}
		payload["toolConfig"] = config
		return payload
	case payload["input"] != nil:
		choice = map[string]any{"type": "function", "name": name}
	case payload["toolChoice"] != nil:
		payload["toolChoice"] = choice
		return payload
	case anthropicTools(payload):
		choice = map[string]any{"type": "tool", "name": name}
	}
	payload["tool_choice"] = choice
	return payload
}

// anthropicTools reports whether the body is the anthropic messages shape: its
// tools carry input_schema, which no other provider's do.
func anthropicTools(payload map[string]any) bool {
	tools, _ := payload["tools"].([]any)
	for _, raw := range tools {
		tool, _ := raw.(map[string]any)
		if _, ok := tool["input_schema"]; ok {
			return true
		}
	}
	return false
}
