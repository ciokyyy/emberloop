//go:build live

package web

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// This file holds throwaway measurement probes for the assistant-chunk burst
// investigation (final claude messages arriving as a ~10-30ms burst of ~100
// assistant_chunk events instead of streaming live). Never in the main check.
//
// Method: drive ClaudeBackend against the real pinned adapter with stdio
// routed through spike/claude/acp-tee.mjs, which timestamps every ACP line
// leaving the adapter, and timestamp every assistant_chunk at OnUpdate
// dispatch. A burst already in the tee log is upstream (adapter/SDK/CLI); a
// tee log that streams while dispatch bursts is the Daemon read/append path.
//
//	EL_LIVE=claude go test -tags live -run 'TestLiveStreamTiming' . -v
//
// Needs claude login, node and network.

type timingChunk struct {
	at time.Time
	n  int // text length
}

type timingPhase struct {
	name     string
	chunks   []timingChunk
	start    time.Time
	end      time.Time
	teeLog   string
	reply    strings.Builder
	sawTool  bool
	markNeed string
}

func runTimingPrompt(be *ClaudeBackend, id, text string, p *timingPhase) {
	var mu sync.Mutex
	p.start = time.Now()
	be.OnPrompt(id, text, func(kind, txt, status string) {
		mu.Lock()
		defer mu.Unlock()
		if kind == "assistant_chunk" {
			p.chunks = append(p.chunks, timingChunk{at: time.Now(), n: len(txt)})
			p.reply.WriteString(txt)
		}
		if kind == "tool_call_started" && strings.Contains(txt, "spike_echo") {
			p.sawTool = true
		}
	})
	p.end = time.Now()
}

func summarizeTiming(name string, times []int64, wallMs int64, extra string) {
	gaps := []float64{}
	for i := 1; i < len(times); i++ {
		gaps = append(gaps, float64(times[i]-times[i-1]))
	}
	var maxGap, sum float64
	for _, g := range gaps {
		sum += g
		if g > maxGap {
			maxGap = g
		}
	}
	mean := 0.0
	if len(gaps) > 0 {
		mean = sum / float64(len(gaps))
	}
	sort.Float64s(gaps)
	p50, p99 := 0.0, 0.0
	if len(gaps) > 0 {
		p50 = gaps[len(gaps)/2]
		p99 = gaps[int(float64(len(gaps))*0.99)]
	}
	span := int64(0)
	if len(times) > 1 {
		span = times[len(times)-1] - times[0]
	}
	fmt.Printf("%s: n=%d spanMs=%d meanGapMs=%.2f p50=%.2f p99=%.2f maxGapMs=%.1f wallMs=%d %s\n",
		name, len(times), span, mean, p50, p99, maxGap, wallMs, extra)
}

func reportTimingDispatch(p *timingPhase) {
	ms := make([]int64, len(p.chunks))
	for i, c := range p.chunks {
		ms[i] = c.at.Sub(p.start).Milliseconds()
	}
	summarizeTiming("DISPATCH "+p.name, ms, p.end.Sub(p.start).Milliseconds(),
		fmt.Sprintf("replyChars=%d sawTool=%v want=%q found=%v", p.reply.Len(), p.sawTool,
			p.markNeed, strings.Contains(p.reply.String(), p.markNeed)))
}

// reportTimingTee parses one tee log on the adapter's own monotonic clock:
// every C2S session/prompt opens a segment; S2C agent_message_chunk lines
// inside it are the adapter's emission times, and the next S2C result closes
// the turn. A live stream spreads chunks across the segment; a consolidated
// flush piles them just before the result. Segments group by tee pid so
// concurrent adapter processes sharing one log never mix clocks.
func reportTimingTee(path string) {
	raw, err := os.ReadFile(path)
	if err != nil {
		fmt.Printf("TEE %s: unreadable: %v\n", path, err)
		return
	}
	type seg struct {
		promptNs int64
		chunks   []int64
		resultNs int64
	}
	byPid := map[string][]seg{}
	order := []string{}
	for _, line := range strings.Split(string(raw), "\n") {
		f := strings.Split(line, "\t")
		if len(f) != 6 {
			continue
		}
		ns, _ := strconv.ParseInt(f[0], 10, 64)
		pid, dir, kind, detail := f[1], f[2], f[3], f[4]
		if _, ok := byPid[pid]; !ok {
			order = append(order, pid)
		}
		segs := byPid[pid]
		if dir == "C2S" && kind == "request" && strings.HasPrefix(detail, "session/prompt") {
			segs = append(segs, seg{promptNs: ns})
		} else if dir == "S2C" && kind == "notify" && strings.HasPrefix(detail, "session/update:agent_message_chunk") {
			if len(segs) == 0 {
				segs = append(segs, seg{})
			}
			segs[len(segs)-1].chunks = append(segs[len(segs)-1].chunks, ns)
		} else if dir == "S2C" && kind == "result" {
			if len(segs) > 0 && segs[len(segs)-1].resultNs == 0 {
				segs[len(segs)-1].resultNs = ns
			}
		}
		byPid[pid] = segs
	}
	for _, pid := range order {
		for i, s := range byPid[pid] {
			if len(s.chunks) == 0 {
				continue
			}
			ms := make([]int64, len(s.chunks))
			for j, ns := range s.chunks {
				ms[j] = (ns - s.promptNs) / 1e6
			}
			wall := int64(-1)
			tailGap := int64(-1)
			if s.resultNs != 0 {
				wall = (s.resultNs - s.promptNs) / 1e6
				tailGap = (s.resultNs - s.chunks[len(s.chunks)-1]) / 1e6
			}
			firstLead := (s.chunks[0] - s.promptNs) / 1e6
			summarizeTiming(fmt.Sprintf("ADAPTER %s pid%s seg%d", filepath.Base(path), pid, i), ms, wall,
				fmt.Sprintf("firstChunkAtMs=%d lastChunkToResultMs=%d", firstLead, tailGap))
		}
	}
}

type timingEnv struct {
	node    string
	absEcho string
	absTee  string
}

func setupTiming(t *testing.T) timingEnv {
	t.Helper()
	if os.Getenv("EL_LIVE") != "claude" {
		t.Skip("set EL_LIVE=claude for the real-adapter probe")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not on PATH")
	}
	echo := filepath.Join("spike", "claude", "mcp-echo-server.mjs")
	if _, err := os.Stat(echo); err != nil {
		t.Fatalf("echo server missing: %v", err)
	}
	tee := filepath.Join("spike", "claude", "acp-tee.mjs")
	if _, err := os.Stat(tee); err != nil {
		t.Fatalf("tee proxy missing: %v", err)
	}
	absEcho, _ := filepath.Abs(echo)
	absTee, _ := filepath.Abs(tee)
	return timingEnv{node: node, absEcho: absEcho, absTee: absTee}
}

func newTimingBackend(env timingEnv, log string) *ClaudeBackend {
	be := NewClaudeBackend()
	be.mcpName = "spike"
	be.mcpCommand = env.node
	be.mcpArgs = []string{env.absEcho}
	be.allowedTools = []string{"mcp__spike__spike_echo"}
	be.adapterCommand = []string{env.node, env.absTee, "--log", log, "--", "npx", "-y", ClaudeAdapterPackage}
	return be
}

func checkTimingPhase(t *testing.T, p *timingPhase) {
	t.Helper()
	if !strings.Contains(p.reply.String(), p.markNeed) {
		t.Errorf("phase %s: want %q in reply, got %q", p.name, p.markNeed, p.reply.String())
	}
	if len(p.chunks) == 0 {
		t.Errorf("phase %s: no assistant chunks", p.name)
	}
}

// TestLiveStreamTiming: fresh session/new + plain prompt (control), second
// prompt in the same process with a tool call, then prompt after
// session/load (the Daemon's idle-resume path). Short replies.
func TestLiveStreamTiming(t *testing.T) {
	env := setupTiming(t)
	cwd := t.TempDir()
	tmp := t.TempDir()

	pa := &timingPhase{name: "A-new-plain", teeLog: filepath.Join(tmp, "tee-a.log"), markNeed: "PLAIN-OK"}
	be := newTimingBackend(env, pa.teeLog)
	meta := SessionMeta{ID: "live-timing-1", Harness: "claude", Model: "haiku", WorkDir: cwd}
	if err := be.StartSession(meta); err != nil {
		t.Fatalf("phase A start: %v", err)
	}
	runTimingPrompt(be, meta.ID, "Reply with exactly this line and nothing else: PLAIN-OK", pa)
	reportTimingDispatch(pa)

	pb := &timingPhase{name: "B-sameproc-tool", teeLog: pa.teeLog, markNeed: "TOOL-THEN-TEXT-OK"}
	runTimingPrompt(be, meta.ID, "Call the spike_echo tool once with text beta. Then reply with exactly: TOOL-THEN-TEXT-OK", pb)
	reportTimingDispatch(pb)
	reportTimingTee(pa.teeLog)

	pc := &timingPhase{name: "C-load-plain", teeLog: filepath.Join(tmp, "tee-c.log"), markNeed: "LOADED-OK"}
	be.mu.Lock()
	acpID := be.sessions[meta.ID].acpID
	be.mu.Unlock()
	if acpID == "" {
		t.Fatalf("no ACP session id to load")
	}
	be2 := newTimingBackend(env, pc.teeLog)
	if err := be2.LoadSession("live-timing-2", acpID, "haiku", cwd, "web"); err != nil {
		t.Fatalf("phase C load: %v", err)
	}
	runTimingPrompt(be2, "live-timing-2", "Reply with exactly this line and nothing else: LOADED-OK", pc)
	reportTimingDispatch(pc)
	reportTimingTee(pc.teeLog)

	for _, p := range []*timingPhase{pa, pb, pc} {
		checkTimingPhase(t, p)
	}
	if !pb.sawTool {
		t.Errorf("phase B: echo tool never called")
	}
	fmt.Printf("TEELOGS dir=%s\n", tmp)
}

// TestLiveStreamTimingBig: same matrix with burst-scale replies (~2KB) and,
// for E/F, a ~40K-token pasted context mimicking the 50-80K-token resumed
// sessions that burst in production.
func TestLiveStreamTimingBig(t *testing.T) {
	env := setupTiming(t)
	cwd := t.TempDir()
	tmp := t.TempDir()

	essay := "Write a 400-word essay on the history of lighthouses, ending with the word %s on its own last line."

	// D0: fresh session, long reply, small context.
	pd0 := &timingPhase{name: "D0-new-longreply", teeLog: filepath.Join(tmp, "tee-d0.log"), markNeed: "LIGHTHOUSE"}
	be := newTimingBackend(env, pd0.teeLog)
	meta := SessionMeta{ID: "live-timing-big", Harness: "claude", Model: "haiku", WorkDir: cwd}
	if err := be.StartSession(meta); err != nil {
		t.Fatalf("phase D0 start: %v", err)
	}
	runTimingPrompt(be, meta.ID, fmt.Sprintf(essay, "LIGHTHOUSE"), pd0)
	reportTimingDispatch(pd0)
	reportTimingTee(pd0.teeLog)

	// D1: stuff ~40K tokens of context into the same session.
	pd1 := &timingPhase{name: "D1-context-load", teeLog: pd0.teeLog, markNeed: "CONTEXT-LOADED"}
	filler := strings.Repeat("The lighthouse keeper logged the tide, the wind and the gull count in the margin of the salt-warped ledger. ", 2000)
	runTimingPrompt(be, meta.ID, "Here is background context you must remember for later questions:\n\n"+filler+"\n\nReply with exactly: CONTEXT-LOADED", pd1)
	reportTimingDispatch(pd1)

	// D2: same process, long reply with big context.
	pd2 := &timingPhase{name: "D2-sameproc-bigctx-long", teeLog: pd0.teeLog, markNeed: "HARBOUR"}
	runTimingPrompt(be, meta.ID, fmt.Sprintf(essay, "HARBOUR"), pd2)
	reportTimingDispatch(pd2)
	reportTimingTee(pd0.teeLog)

	// D3: new process, session/load, long reply with big context.
	pd3 := &timingPhase{name: "D3-load-bigctx-long", teeLog: filepath.Join(tmp, "tee-d3.log"), markNeed: "BEACON"}
	be.mu.Lock()
	acpID := be.sessions[meta.ID].acpID
	be.mu.Unlock()
	if acpID == "" {
		t.Fatalf("no ACP session id to load")
	}
	be2 := newTimingBackend(env, pd3.teeLog)
	if err := be2.LoadSession("live-timing-big-2", acpID, "haiku", cwd, "web"); err != nil {
		t.Fatalf("phase D3 load: %v", err)
	}
	runTimingPrompt(be2, "live-timing-big-2", fmt.Sprintf(essay, "BEACON"), pd3)
	reportTimingDispatch(pd3)
	reportTimingTee(pd3.teeLog)

	for _, p := range []*timingPhase{pd0, pd1, pd2, pd3} {
		checkTimingPhase(t, p)
	}
	fmt.Printf("TEELOGS dir=%s\n", tmp)
}

// TestLiveStreamTimingModel bisects the burst lever: model (sonnet) vs
// reasoning effort (medium). Each phase is a fresh session/new with a
// burst-scale reply, so session age and resume play no role.
func TestLiveStreamTimingModel(t *testing.T) {
	env := setupTiming(t)
	cwd := t.TempDir()
	tmp := t.TempDir()
	essay := "Write a 400-word essay on the history of %s, ending with the word %s on its own last line."
	type cfg struct {
		name   string
		model  string
		effort string
		topic  string
		mark   string
	}
	for _, c := range []cfg{
		{name: "E0-sonnet-medium", model: "sonnet", effort: "medium", topic: "sonnets", mark: "SONNET"},
		{name: "E1-sonnet-plain", model: "sonnet", effort: "", topic: "harbours", mark: "HARBOUR"},
		{name: "E2-haiku-medium", model: "haiku", effort: "medium", topic: "beacons", mark: "BEACON"},
	} {
		p := &timingPhase{name: c.name, teeLog: filepath.Join(tmp, "tee-"+c.name+".log"), markNeed: c.mark}
		be := newTimingBackend(env, p.teeLog)
		meta := SessionMeta{ID: "live-timing-" + c.name, Harness: "claude", Model: c.model, Effort: c.effort, WorkDir: cwd}
		if err := be.StartSession(meta); err != nil {
			t.Fatalf("phase %s start: %v", c.name, err)
		}
		runTimingPrompt(be, meta.ID, fmt.Sprintf(essay, c.topic, c.mark), p)
		reportTimingDispatch(p)
		reportTimingTee(p.teeLog)
		checkTimingPhase(t, p)
	}
	fmt.Printf("TEELOGS dir=%s\n", tmp)
}
