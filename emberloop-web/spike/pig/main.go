// Spike probe: pig Backend through the embedded Go SDK.
//
// Creates a Session stripped to one marker tool (the stand-in for the
// emberloop MCP tool set), subscribes to every event, sends a prompt,
// calls QueueSteer and QueueFollowUp mid-run, and prints it all.
// Findings go to .scratch/enderbloom-web/spike-findings.md.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/coding"
)

// echoTool stands in for one emberloop MCP tool: the only tool the
// stripped Session may offer.
type echoTool struct{ calls *int }

func (t echoTool) Name() string  { return "mcp__emberloop__spike_echo" }
func (t echoTool) Label() string { return "spike echo" }
func (t echoTool) Schema() ai.ToolSchema {
	return ai.ToolSchema{
		Name:        t.Name(),
		Description: "Echo back the given text. For the Backend spike only.",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{"text": map[string]any{"type": "string"}},
		},
	}
}
func (t echoTool) Execute(_ context.Context, _ string, params json.RawMessage, _ agent.ToolUpdateCallback) (agent.AgentToolResult, error) {
	*t.calls++
	return agent.AgentToolResult{
		Content: []ai.ToolResultMessageContent{ai.TextContent{Text: "echo:" + string(params)}},
	}, nil
}
func (t echoTool) ExecutionMode() agent.ToolExecutionMode { return agent.ToolModeSequential }

func main() {
	log.SetFlags(0)
	modelID := os.Getenv("PI_SPIKE_MODEL")
	if modelID == "" {
		modelID = "deepseek/deepseek-flash"
	}
	cwd, err := os.MkdirTemp("", "pig-spike-")
	if err != nil {
		log.Fatalf("mkdir temp cwd: %v", err)
	}
	defer func() { _ = os.RemoveAll(cwd) }()

	svcs, err := coding.NewServices(coding.ServicesOptions{CWD: cwd, AgentDir: coding.DefaultAgentDir()})
	if err != nil {
		log.Fatalf("NewServices: %v", err)
	}
	defer svcs.Close()

	rt, err := coding.NewRuntime(coding.RuntimeOptions{Services: svcs})
	if err != nil {
		log.Fatalf("NewRuntime: %v", err)
	}
	defer func() { _ = rt.Close() }()

	model, err := coding.BuildModel(modelID, svcs)
	if err != nil {
		log.Fatalf("BuildModel(%q): %v", modelID, err)
	}

	var mu sync.Mutex
	counts := map[string]int{}
	ev := func(name, detail string) {
		mu.Lock()
		counts[name]++
		mu.Unlock()
		fmt.Printf("EV %-28s %s\n", name, detail)
	}

	var toolCalls int
	sess, err := rt.New(coding.SessionStartOptions{
		Model:            model,
		SystemPrompt:     "You are a probe target. Use the spike_echo tool when asked. Keep replies to one short sentence.",
		SkipBuiltinTools: true,
		ThinkingLevel:    ai.ThinkingLow,
		ExtraTools:       []agent.AgentTool{echoTool{calls: &toolCalls}},
	})
	if err != nil {
		log.Fatalf("rt.New: %v", err)
	}
	defer func() { _ = sess.Close() }()

	names := []string{}
	for _, t := range sess.Tools() {
		names = append(names, t.Name())
	}
	sort.Strings(names)
	fmt.Printf("TOOLS %q\n", names)
	fmt.Printf("SESSION path=%s\n", sess.Path())

	unsub := sess.Subscribe(func(e agent.AgentEvent) {
		switch e := e.(type) {
		case agent.QueueUpdateEvent:
			ev("QueueUpdate", fmt.Sprintf("steering=%q followUp=%q", e.Steering, e.FollowUp))
		case agent.ToolExecutionStartEvent:
			ev("ToolStart", e.ToolName)
		case agent.ToolExecutionUpdateEvent:
			ev("ToolUpdate", e.ToolName)
		case agent.ToolExecutionEndEvent:
			ev("ToolEnd", e.ToolName)
		case agent.TurnStartEvent:
			ev("TurnStart", fmt.Sprintf("index=%d", e.TurnIndex))
		case agent.TurnEndEvent:
			ev("TurnEnd", fmt.Sprintf("index=%d", e.TurnIndex))
		case agent.MessageStartEvent:
			ev("MessageStart", "")
		case agent.MessageUpdateEvent:
			ev("MessageUpdate", fmt.Sprintf("stream=%T", e.AssistantMessageEvent))
		case agent.AgentStartEvent:
			ev("AgentStart", "")
		case agent.AgentEndEvent:
			ev("AgentEnd", fmt.Sprintf("willRetry=%v messages=%d", e.WillRetry, len(e.Messages)))
		case agent.AgentSettledEvent:
			ev("AgentSettled", "")
		default:
			ev(fmt.Sprintf("%T", e), "")
		}
	})
	defer unsub()

	ctx, cancel := context.WithTimeout(context.Background(), 9*time.Minute)
	defer cancel()

	done := make(chan struct{})
	var msgs []agent.AgentMessage
	var sendErr error
	go func() {
		t := time.NewTicker(20 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case now := <-t.C:
				fmt.Printf("HEARTBEAT %s steering=%q followUp=%q\n", now.Format("15:04:05"), sess.GetSteeringMessages(), sess.GetFollowUpMessages())
			}
		}
	}()
	go func() {
		defer close(done)
		prompt := "Call the spike_echo tool once with text alpha. Then reply with exactly: SPIKE-DONE"
		if os.Getenv("PI_SPIKE_NOTOOLS") != "" {
			prompt = "Do not call any tools. Reply with exactly: NOTOOLS-OK and nothing else."
		}
		if os.Getenv("PI_SPIKE_LONG") != "" {
			prompt = "Do not call any tools. Write a 200-word story about a lighthouse keeper."
		}
		msgs, sendErr = sess.Send(ctx, prompt)
	}()

	steerDelay := 3 * time.Second
	if os.Getenv("PI_SPIKE_FAST") != "" {
		steerDelay = 1 * time.Second
	}
	time.Sleep(steerDelay)
	if os.Getenv("PI_SPIKE_NOSTEER") == "" {
		sess.QueueSteer("Steered note: also mention the word STEERED in your reply.", nil)
		ev("ProbeAction", "QueueSteer sent")
		fmt.Printf("STEERING %q\n", sess.GetSteeringMessages())
		time.Sleep(2 * time.Second)
		sess.QueueFollowUp("Follow-up: after this turn, reply with exactly: FOLLOWUP-OK", nil)
		ev("ProbeAction", "QueueFollowUp sent")
		fmt.Printf("FOLLOWUP %q\n", sess.GetFollowUpMessages())
	} else {
		ev("ProbeAction", "queues skipped (PI_SPIKE_NOSTEER)")
		time.Sleep(2 * time.Second)
	}
	<-done
	if sendErr != nil {
		log.Fatalf("Send: %v", sendErr)
	}
	for _, m := range msgs {
		if m.Assistant != nil {
			b, _ := json.Marshal(m.Assistant)
			s := string(b)
			if len(s) > 500 {
				s = s[:500] + "..."
			}
			fmt.Printf("REPLY %s\n", s)
		}
	}
	fmt.Printf("TOOLCALLS spike_echo=%d\n", toolCalls)
	mu.Lock()
	fmt.Printf("COUNTS %v\n", counts)
	mu.Unlock()
}
