package emberloop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"example.com/emberloop/statedb"
)

// Seam 1: usage parsers — one transcript line in, zero or more normalized
// usage records out. Fixtures are verbatim lines from real pig and claude
// transcripts (see testdata/usage), so the expected records below are worked
// from the harness output, not from the implementation.

// readUsageFixtures loads the named fixture file's lines in order.
func readUsageFixtures(t *testing.T, name string) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "usage", name))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(raw), "\n")
	out := lines[:0]
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}

// usageEqual compares records by value, following the cost pointer: two
// pointers to the same number are equal.
func usageEqual(a, b UsageRecord) bool {
	if (a.ReportedCost == nil) != (b.ReportedCost == nil) {
		return false
	}
	if a.ReportedCost != nil && *a.ReportedCost != *b.ReportedCost {
		return false
	}
	a.ReportedCost, b.ReportedCost = nil, nil
	return a == b
}

func parseOne(t *testing.T, harness, agent, line string) ([]UsageRecord, string) {
	t.Helper()
	p, ok := UsageParserFor(harness)
	if !ok {
		t.Fatalf("no usage parser for %q", harness)
	}
	return p.ParseLine(agent, line)
}

func TestPigUsageLineBecomesOneRawRecord(t *testing.T) {
	lines := readUsageFixtures(t, "pig.jsonl")
	if len(lines) != 4 {
		t.Fatalf("want 4 pig fixtures, got %d", len(lines))
	}
	// A user message carries no usage: nothing to log.
	if recs, _ := parseOne(t, "pig", "w1", lines[0]); len(recs) != 0 {
		t.Errorf("user line = %+v, want no records", recs)
	}
	// A normal assistant message logs its usage raw, with the
	// harness-reported cost exactly as reported in usage.cost.total.
	recs, key := parseOne(t, "pig", "w1", lines[1])
	if len(recs) != 1 {
		t.Fatalf("assistant line = %+v, want one record", recs)
	}
	got := recs[0]
	wantCost := 0.000311926
	want := UsageRecord{Agent: "w1", At: "2026-10-05T20:15:23.360Z", Harness: "pig",
		Provider: "opencode-go", Model: "muse-spark-1.3-contributor",
		Input: 2913, Output: 102, CacheRead: 113, Reasoning: 7,
		ReportedCost: &wantCost, Kind: "request"}
	if !usageEqual(got, want) {
		t.Errorf("pig record = %+v, want %+v", got, want)
	}
	if key == "" {
		t.Error("pig record needs a dedupe key so a re-tailed line is never logged twice")
	}
	// A reasoning-heavy turn keeps its reasoning tokens; an errored turn
	// still logs (a request happened) with its zero usage and zero cost.
	recs, _ = parseOne(t, "pig", "w1", lines[2])
	if len(recs) != 1 || recs[0].Reasoning != 665 || recs[0].Input != 2340 || recs[0].Output != 957 {
		t.Errorf("reasoning line = %+v, want input 2340 output 957 reasoning 665", recs)
	}
	recs, _ = parseOne(t, "pig", "w1", lines[3])
	if len(recs) != 1 {
		t.Fatalf("error line = %+v, want one zero record", recs)
	}
	if recs[0].Input != 0 || recs[0].ReportedCost == nil || *recs[0].ReportedCost != 0 {
		t.Errorf("error record = %+v, want zeros with a reported zero cost", recs[0])
	}
	// Junk is not usage.
	if recs, _ := parseOne(t, "pig", "w1", "not json"); len(recs) != 0 {
		t.Errorf("junk = %+v, want no records", recs)
	}
}

func TestIdlessLinesGetDistinctKeys(t *testing.T) {
	a := "{\"type\":\"message\",\"timestamp\":\"2026-10-05T20:15:23.360Z\"," +
		"\"message\":{\"role\":\"assistant\",\"provider\":\"p\",\"model\":\"m\"," +
		"\"usage\":{\"input\":1,\"output\":1,\"cacheRead\":0,\"cacheWrite\":0,\"reasoning\":0,\"cost\":{\"total\":0.1}}}}"
	b := "{\"type\":\"assistant\",\"timestamp\":\"2026-09-14T12:29:29.843Z\"," +
		"\"message\":{\"model\":\"m\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1," +
		"\"cache_read_input_tokens\":0,\"cache_creation_input_tokens\":0}}}"
	recsA, keyA := parseOne(t, "pig", "w1", a)
	_, keyA2 := parseOne(t, "pig", "w1", a)
	recsB, keyB := parseOne(t, "claude", "w1", b)
	if len(recsA) != 1 || len(recsB) != 1 {
		t.Fatalf("id-less lines = %+v %+v, want one record each", recsA, recsB)
	}
	if keyA == "" || keyB == "" {
		t.Errorf("id-less keys = %q %q, want non-empty keys", keyA, keyB)
	}
	if keyA != keyA2 {
		t.Errorf("same id-less line keys = %q %q, want one stable key", keyA, keyA2)
	}
	c := strings.Replace(a, "\"input\":1,", "\"input\":2,", 1)
	_, keyC := parseOne(t, "pig", "w1", c)
	if keyC == keyA {
		t.Errorf("different id-less lines share key %q", keyC)
	}
}

func TestClaudeUsageLinesBecomeOneRecordPerRequest(t *testing.T) {
	lines := readUsageFixtures(t, "claude.jsonl")
	if len(lines) != 9 {
		t.Fatalf("want 9 claude fixtures, got %d", len(lines))
	}
	// A user message and a hook attachment carry no usage.
	for _, i := range []int{0, 6} {
		if recs, _ := parseOne(t, "claude", "w1", lines[i]); len(recs) != 0 {
			t.Errorf("fixture %d = %+v, want no records", i, recs)
		}
	}
	// Claude streams one request as several assistant blocks with the same
	// usage: every block parses to the same record under the same key, and
	// the tailer logs the key once. The per-message usage carries no cost,
	// so none is reported: cost comes from the cost-state record alone.
	first, firstKey := parseOne(t, "claude", "w1", lines[1])
	second, secondKey := parseOne(t, "claude", "w1", lines[2])
	if len(first) != 1 || len(second) != 1 {
		t.Fatalf("same-request blocks = %+v %+v, want one record each", first, second)
	}
	if firstKey == "" || firstKey != secondKey {
		t.Errorf("same-request keys = %q %q, want one shared key", firstKey, secondKey)
	}
	// Every block carries its own block time; the tailer logs the first
	// block it meets, so the log holds the request's start. Compare modulo
	// At.
	secondAtStart := second[0]
	secondAtStart.At = first[0].At
	if !usageEqual(first[0], secondAtStart) {
		t.Errorf("same-request records differ: %+v vs %+v", first[0], second[0])
	}
	got := first[0]
	if got.Agent != "w1" || got.Harness != "claude" || got.Model != "claude-opus-5" ||
		got.At != "2026-09-14T12:29:29.843Z" || got.Kind != "request" {
		t.Errorf("request record identity = %+v", got)
	}
	if got.Input != 2 || got.Output != 474 || got.CacheRead != 0 || got.CacheWrite != 17838 || got.Reasoning != 170 {
		t.Errorf("request tokens = %+v, want input 2 output 474 cacheRead 0 cacheWrite 17838 reasoning 170", got)
	}
	if got.ReportedCost != nil {
		t.Errorf("per-message cost = %v, want none: claude reports cost only in cost-state", *got.ReportedCost)
	}
	// Blocks without a request id still share their message id: one request,
	// one key. The second request's cache reads land in cacheRead.
	third, thirdKey := parseOne(t, "claude", "w1", lines[3])
	fourth, fourthKey := parseOne(t, "claude", "w1", lines[4])
	if len(third) != 1 || len(fourth) != 1 || thirdKey == "" || thirdKey != fourthKey {
		t.Errorf("no-request-id blocks = %+v/%q %+v/%q, want one record each under one key", third, thirdKey, fourth, fourthKey)
	}
	if thirdKey == firstKey {
		t.Errorf("different requests share key %q", thirdKey)
	}
	if len(third) == 1 && (third[0].Input != 33 || third[0].Output != 1602 || third[0].CacheRead != 18969 || third[0].CacheWrite != 15941 || third[0].Reasoning != 500) {
		t.Errorf("second request tokens = %+v, want input 33 output 1602 cacheRead 18969 cacheWrite 15941 reasoning 500", third[0])
	}
	// A failed synthetic request still logs raw: a request happened, with
	// the zero usage and the model exactly as reported.
	synth, synthKey := parseOne(t, "claude", "w1", lines[8])
	if len(synth) != 1 || synthKey == "" {
		t.Fatalf("synthetic line = %+v/%q, want one raw record", synth, synthKey)
	}
	if synth[0].Model != "<synthetic>" || synth[0].Input != 0 || synth[0].Output != 0 || synth[0].ReportedCost != nil {
		t.Errorf("synthetic record = %+v, want raw zeros with no cost", synth[0])
	}
}

func TestClaudeCostStateBecomesPerModelCostRows(t *testing.T) {
	lines := readUsageFixtures(t, "claude.jsonl")
	// The opening cost-state has empty model usage: nothing to log.
	if recs, _ := parseOne(t, "claude", "w1", lines[5]); len(recs) != 0 {
		t.Errorf("empty cost-state = %+v, want no records", recs)
	}
	// A cost-state with model usage logs one cumulative row per model, with
	// the harness-reported cost exactly as reported. These are snapshots,
	// not requests, so they carry their own kind.
	recs, key := parseOne(t, "claude", "w1", lines[7])
	if len(recs) != 1 {
		t.Fatalf("cost-state = %+v, want one row per model", recs)
	}
	got := recs[0]
	if got.Kind != "cost" || got.Agent != "w1" || got.Harness != "claude" || got.Model != "claude-opus-5" {
		t.Errorf("cost row identity = %+v", got)
	}
	if got.Input != 933 || got.Output != 41229 || got.CacheRead != 2462958 || got.CacheWrite != 281434 || got.Reasoning != 15510 {
		t.Errorf("cost tokens = %+v, want the reported totals", got)
	}
	if got.ReportedCost == nil || *got.ReportedCost != 5.07660025 {
		t.Errorf("cost row cost = %+v, want the reported 5.07660025", got.ReportedCost)
	}
	if key == "" {
		t.Error("cost-state needs a key so an unchanged snapshot is logged once")
	}
	// The same snapshot again carries the same key: the tailer logs it once.
	_, again := parseOne(t, "claude", "w1", lines[7])
	if again != key {
		t.Errorf("same snapshot keys = %q %q, want one stable key", key, again)
	}
}

// Seam 2: the usage tail — the session's own emberloop process tails the harness
// transcript from a stored byte offset and appends new records to the
// agent's log. Fake transcript appended in steps, the way the archive copier
// tests do it.

func writeLines(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func appendLines(t *testing.T, path, content string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
}

func readUsageLog(t *testing.T, agent string) []UsageRecord {
	t.Helper()
	db, err := statedb.Open(dbPath())
	if err != nil {
		t.Fatalf("open state database at %s: %v", dbPath(), err)
	}
	defer db.Close()
	rows, err := statedb.UsageRowsForAgent(db, agent)
	if err != nil {
		t.Fatal(err)
	}
	var out []UsageRecord
	for _, r := range rows {
		out = append(out, UsageRecord{
			Agent: r.Agent, At: r.At, Harness: r.Harness,
			Provider: r.Provider, Model: r.Model,
			Input: r.Input, Output: r.Output,
			CacheRead: r.CacheRead, CacheWrite: r.CacheWrite,
			Reasoning: r.Reasoning, ReportedCost: r.ReportedCost,
			Kind: r.Kind,
		})
	}
	return out
}

// pigLine builds a minimal pig assistant line carrying id with usage.
func pigLine(id string, input int64) string {
	return `{"type":"message","id":"` + id + `","timestamp":"2026-10-05T20:15:23.360Z",` +
		`"message":{"role":"assistant","provider":"p","model":"m",` +
		`"usage":{"input":` + strconv.FormatInt(input, 10) + `,"output":1,"cacheRead":0,"cacheWrite":0,"reasoning":0,` +
		`"cost":{"total":0.5}}}}` + "\n"
}

func TestTailUsageTracksResumesAndNeverDoubleCounts(t *testing.T) {
	isolateState(t)
	dir := t.TempDir()
	transcript := filepath.Join(dir, "sess.jsonl")
	// A missing transcript is not an error: the harness may not have
	// written one yet (or the session runs headless).
	if n, err := TailUsage(filepath.Join(dir, "absent.jsonl"), "w1", "pig"); n != 0 || err != nil {
		t.Fatalf("absent transcript = %d, %v; want 0, nil", n, err)
	}
	if _, err := TailUsage(transcript, "w1", "nope"); err == nil {
		t.Error("unknown harness must be an error")
	}
	// First pass logs what is there.
	writeLines(t, transcript, pigLine("m1", 10)+pigLine("m2", 20))
	if n, err := TailUsage(transcript, "w1", "pig"); n != 2 || err != nil {
		t.Fatalf("first tail = %d, %v; want 2, nil", n, err)
	}
	got := readUsageLog(t, "w1")
	if len(got) != 2 || got[0].Input != 10 || got[1].Input != 20 {
		t.Fatalf("log = %+v, want the two requests in order", got)
	}
	// A pass with nothing new appends nothing: the final pass when the
	// session settles is a plain re-tail.
	if n, err := TailUsage(transcript, "w1", "pig"); n != 0 || err != nil {
		t.Fatalf("settled re-tail = %d, %v; want 0, nil", n, err)
	}
	// New lines append; a re-seen line (same message id) never logs twice,
	// even across a restarted tailer: state lives on disk, reloaded every
	// pass.
	appendLines(t, transcript, pigLine("m1", 10)+pigLine("m3", 30))
	if n, err := TailUsage(transcript, "w1", "pig"); n != 1 || err != nil {
		t.Fatalf("second tail = %d, %v; want 1, nil", n, err)
	}
	got = readUsageLog(t, "w1")
	if len(got) != 3 || got[2].Input != 30 {
		t.Fatalf("log = %+v, want m1 m2 m3 exactly once each", got)
	}
	// A partial trailing line waits for the next pass: only complete lines
	// move the offset.
	appendLines(t, transcript, pigLine("m4", 40)[:50])
	if n, err := TailUsage(transcript, "w1", "pig"); n != 0 || err != nil {
		t.Fatalf("partial tail = %d, %v; want 0, nil", n, err)
	}
	appendLines(t, transcript, pigLine("m4", 40)[50:])
	if n, err := TailUsage(transcript, "w1", "pig"); n != 1 || err != nil {
		t.Fatalf("completed tail = %d, %v; want 1, nil", n, err)
	}
	if got := readUsageLog(t, "w1"); len(got) != 4 {
		t.Fatalf("log has %d records, want 4", len(got))
	}
}

func TestTailUsageDedupesClaudeBlocksAcrossPasses(t *testing.T) {
	isolateState(t)
	dir := t.TempDir()
	transcript := filepath.Join(dir, "claude.jsonl")
	lines := readUsageFixtures(t, "claude.jsonl")
	// First pass meets the first block of a request and the opening
	// (empty) cost-state: one record.
	writeLines(t, transcript, lines[1]+"\n"+lines[5]+"\n")
	if n, err := TailUsage(transcript, "w1", "claude"); n != 1 || err != nil {
		t.Fatalf("first tail = %d, %v; want 1, nil", n, err)
	}
	// The request's second block arrives later under the same key, and the
	// same cost snapshot repeats: neither logs again. The totals snapshot
	// logs once.
	appendLines(t, transcript, lines[2]+"\n"+lines[7]+"\n"+lines[7]+"\n")
	if n, err := TailUsage(transcript, "w1", "claude"); n != 1 || err != nil {
		t.Fatalf("second tail = %d, %v; want 1, nil", n, err)
	}
	got := readUsageLog(t, "w1")
	if len(got) != 2 || got[0].Kind != "request" || got[1].Kind != "cost" {
		t.Fatalf("log = %+v, want one request row and one cost row", got)
	}
}

// Two agents tailing identical line ids (the same pig message id, or two
// byte-identical claude cost-state snapshots) must each keep their row: the
// tailer's seen lists are per-agent, so the database keys must be too, or the
// second agent's row vanishes under the global unique index.
func TestTailUsageScopesDedupeKeysPerAgent(t *testing.T) {
	isolateState(t)
	dir := t.TempDir()
	ta := filepath.Join(dir, "a.jsonl")
	tb := filepath.Join(dir, "b.jsonl")
	writeLines(t, ta, pigLine("same-id", 10))
	writeLines(t, tb, pigLine("same-id", 10))
	if n, err := TailUsage(ta, "agent-a", "pig"); n != 1 || err != nil {
		t.Fatalf("first agent tail = %d, %v; want 1, nil", n, err)
	}
	if n, err := TailUsage(tb, "agent-b", "pig"); n != 1 || err != nil {
		t.Fatalf("second agent with identical line ids = %d, %v; want 1, nil (keys must be agent-scoped)", n, err)
	}
	if got := readUsageLog(t, "agent-a"); len(got) != 1 || got[0].Input != 10 {
		t.Fatalf("agent-a log = %+v, want its 10-token request", got)
	}
	if got := readUsageLog(t, "agent-b"); len(got) != 1 || got[0].Input != 10 {
		t.Fatalf("agent-b log = %+v, want its 10-token request", got)
	}
}

// Seam 3: wiring — the harness launch tells its emberloop-mcp which transcript to
// tail, and the tail loop follows it. The session's own emberloop process tails
// as requests happen; the final pass when it settles is a plain re-tail.

func envValue(env []string, key string) (string, bool) {
	for _, e := range env {
		if v, ok := strings.CutPrefix(e, key+"="); ok {
			return v, true
		}
	}
	return "", false
}

func TestHarnessCommandsExportUsageTailIdentity(t *testing.T) {
	isolateState(t)
	// Pig names the transcript file after --session-id (the session id,
	// else the ticket label) under its session dir.
	pig, err := BuildCommand("pig", Session{Role: "work", ID: "sess-1", Label: "01-x", Dir: "/tmp/sess"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"EMBERLOOP_HARNESS=pig", "EMBERLOOP_SESSION_ID=sess-1", "EMBERLOOP_SESSION_DIR=/tmp/sess"} {
		k, v, _ := strings.Cut(want, "=")
		if got, ok := envValue(pig.Env, k); !ok || got != v {
			t.Errorf("pig env %q = %q, want %q", k, got, v)
		}
	}
	label, err := BuildCommand("pig", Session{Role: "work", Label: "01-x"})
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := envValue(label.Env, "EMBERLOOP_SESSION_ID"); got != "01-x" {
		t.Errorf("pig label session id = %q, want 01-x", got)
	}
	if _, ok := envValue(label.Env, "EMBERLOOP_SESSION_DIR"); ok {
		t.Errorf("pig with no dir must not set EMBERLOOP_SESSION_DIR: %q", label.Env)
	}
	bare, err := BuildCommand("pig", Session{Role: "work"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := envValue(bare.Env, "EMBERLOOP_SESSION_ID"); ok {
		t.Errorf("a session with no id has no transcript to tail: %q", bare.Env)
	}
	// Claude's transcript is named by its session id; without one there is
	// nothing to tail, and claude takes no session dir.
	claude, err := BuildCommand("claude", Session{Role: "review", ID: "sess-9", Dir: "/tmp/x"})
	if err != nil {
		t.Fatal(err)
	}
	// The tail identity must also reach emberloop-mcp through the MCP config:
	// claude starts its MCP servers from that config's env map, not from
	// its own process env, so Command.Env alone would leave the tailer off.
	cfgRaw, err := os.ReadFile(mcpConfigPath(Session{Role: "review", ID: "sess-9", Dir: "/tmp/x"}))
	if err != nil {
		t.Fatalf("claude MCP config: %v", err)
	}
	var cfg struct {
		Servers map[string]struct {
			Env map[string]string `json:"env"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(cfgRaw, &cfg); err != nil {
		t.Fatalf("claude MCP config is not JSON: %v", err)
	}
	mcpEnv := cfg.Servers["emberloop"].Env
	for _, want := range []string{"EMBERLOOP_HARNESS=claude", "EMBERLOOP_SESSION_ID=sess-9"} {
		k, v, _ := strings.Cut(want, "=")
		if mcpEnv[k] != v {
			t.Errorf("claude MCP config env %q = %q, want %q", k, mcpEnv[k], v)
		}
	}
	for _, want := range []string{"EMBERLOOP_HARNESS=claude", "EMBERLOOP_SESSION_ID=sess-9"} {
		k, v, _ := strings.Cut(want, "=")
		if got, ok := envValue(claude.Env, k); !ok || got != v {
			t.Errorf("claude env %q = %q, want %q", k, got, v)
		}
	}
	if _, ok := envValue(claude.Env, "EMBERLOOP_SESSION_DIR"); ok {
		t.Errorf("claude takes no session dir: %q", claude.Env)
	}
	noid, err := BuildCommand("claude", Session{Role: "review", Label: "01-x"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := envValue(noid.Env, "EMBERLOOP_SESSION_ID"); ok {
		t.Errorf("claude with no session id has no transcript to tail: %q", noid.Env)
	}
}

func TestUsageTailFromEnvStaysOffWithoutASession(t *testing.T) {
	t.Setenv("EMBERLOOP_HARNESS", "pig")
	t.Setenv("EMBERLOOP_SESSION_ID", "")
	t.Setenv("EMBERLOOP_AGENT_ID", "w1")
	if _, ok := UsageTailFromEnv(); ok {
		t.Error("no session id means no transcript: the tailer stays off")
	}
	t.Setenv("EMBERLOOP_SESSION_ID", "s1")
	t.Setenv("EMBERLOOP_AGENT_ID", "")
	if _, ok := UsageTailFromEnv(); ok {
		t.Error("no agent id means no log owner: the tailer stays off")
	}
	t.Setenv("EMBERLOOP_AGENT_ID", "w1")
	t.Setenv("EMBERLOOP_HARNESS", "nope")
	if _, ok := UsageTailFromEnv(); ok {
		t.Error("unknown harness means no parser: the tailer stays off")
	}
}

func TestUsageTailLoopFollowsATranscript(t *testing.T) {
	isolateState(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("EMBERLOOP_HARNESS", "pig")
	t.Setenv("EMBERLOOP_SESSION_ID", "tail-1")
	t.Setenv("EMBERLOOP_AGENT_ID", "agent-1")
	cfg, ok := UsageTailFromEnv()
	if !ok {
		t.Fatal("tail config must resolve from the session environment")
	}
	// Nothing written yet: the first passes are quiet.
	if err := TailUsageFile(cfg); err != nil {
		t.Fatalf("quiet tail: %v", err)
	}
	root := filepath.Join(home, ".pig", "agent", "sessions")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	transcript := filepath.Join(root, "tail-1.jsonl")
	oldEvery := usageTailEvery
	usageTailEvery = 5 * time.Millisecond
	defer func() { usageTailEvery = oldEvery }()
	stop := make(chan struct{})
	defer close(stop)
	go RunUsageTail(cfg, stop)
	writeLines(t, transcript, pigLine("live-1", 7))
	deadline := time.Now().Add(5 * time.Second)
	for {
		if got := readUsageLog(t, "agent-1"); len(got) == 1 && got[0].Input == 7 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("loop never tailed the transcript: %+v", readUsageLog(t, "agent-1"))
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestTailUsageHonoursEmberloopStateDir(t *testing.T) {
	isolateState(t)
	dir := t.TempDir()
	t.Setenv("EMBERLOOP_STATE_DIR", dir)
	transcript := filepath.Join(t.TempDir(), "sess.jsonl")
	writeLines(t, transcript, pigLine("m1", 5))
	if n, err := TailUsage(transcript, "w1", "pig"); n != 1 || err != nil {
		t.Fatalf("tail = %d, %v; want 1, nil", n, err)
	}
	got := readUsageLog(t, "w1")
	if len(got) != 1 || got[0].Input != 5 {
		t.Fatalf("database under EMBERLOOP_STATE_DIR holds %+v, want the tailed request", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "usage", "w1.jsonl")); !os.IsNotExist(err) {
		t.Errorf("no per-agent JSONL may be written: stat err = %v", err)
	}
}
