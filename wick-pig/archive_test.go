package wick

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"example.com/wick/tools"
)

// Seam 1 (ticket 05): blob store — the system prompt, tool definitions and
// harness flags are stored once by content hash. Same bytes twice → one blob.
func TestBlobStoreDedupsIdenticalContent(t *testing.T) {
	isolateState(t)
	first, err := StoreBlob([]byte("system prompt v1"))
	if err != nil {
		t.Fatalf("StoreBlob: %v", err)
	}
	if !strings.HasPrefix(first, "sha256:") || len(first) != len("sha256:")+64 {
		t.Fatalf("blob hash must be sha256:<hex>, got %q", first)
	}
	second, err := StoreBlob([]byte("system prompt v1"))
	if err != nil {
		t.Fatalf("StoreBlob again: %v", err)
	}
	if second != first {
		t.Errorf("same content must hash the same: %q vs %q", first, second)
	}
	if blobs := countFiles(t, blobDir()); blobs != 1 {
		t.Errorf("same content twice must leave one blob, found %d", blobs)
	}
	other, err := StoreBlob([]byte("different prompt"))
	if err != nil {
		t.Fatalf("StoreBlob: %v", err)
	}
	if other == first {
		t.Errorf("different content must hash differently")
	}
	if blobs := countFiles(t, blobDir()); blobs != 2 {
		t.Errorf("two contents must leave two blobs, found %d", blobs)
	}
	got, err := ReadBlob(first)
	if err != nil {
		t.Fatalf("ReadBlob: %v", err)
	}
	if string(got) != "system prompt v1" {
		t.Errorf("ReadBlob = %q, want the stored bytes", got)
	}
	if _, err := ReadBlob("sha256:" + strings.Repeat("0", 64)); !os.IsNotExist(err) {
		t.Errorf("missing blob must be a not-exist error, got %v", err)
	}
}

// Seam 2 (ticket 05): session snapshot — at session start the system prompt,
// the tool definitions served for the role and the harness flags are stored
// once by hash, and the session's archive references the hashes. Two sessions
// sharing a prompt and tool set add one blob, not two.
func TestSessionSnapshotSharesBlobsAcrossSessions(t *testing.T) {
	isolateState(t)
	prompt := SystemPrompt()
	if prompt == "" {
		t.Fatal("SystemPrompt must expose the prompt every session carries")
	}
	toolsJSON, err := ToolDefsJSON(mustRoleTools(t, "review"))
	if err != nil {
		t.Fatalf("ToolDefsJSON: %v", err)
	}
	flags := FlagsJSON("pig", "m1", "high")
	first, err := SnapshotSession(SessionSnapshot{
		Run: "run-records", Agent: "agent-1", Harness: "pig", Role: "review",
		SystemPrompt: prompt, ToolsJSON: toolsJSON, FlagsJSON: flags,
		TranscriptSource: "/sessions/agent-1.jsonl",
	})
	if err != nil {
		t.Fatalf("SnapshotSession: %v", err)
	}
	if first.SystemPrompt == "" || first.Tools == "" || first.Flags == "" {
		t.Fatalf("snapshot must reference all three blobs: %+v", first)
	}
	second, err := SnapshotSession(SessionSnapshot{
		Run: "run-records", Agent: "agent-2", Harness: "pig", Role: "review",
		SystemPrompt: prompt, ToolsJSON: toolsJSON, FlagsJSON: flags,
		TranscriptSource: "/sessions/agent-2.jsonl",
	})
	if err != nil {
		t.Fatalf("SnapshotSession: %v", err)
	}
	if second.SystemPrompt != first.SystemPrompt || second.Tools != first.Tools || second.Flags != first.Flags {
		t.Errorf("same prompt/tools/flags must share blobs: %+v vs %+v", first, second)
	}
	if blobs := countFiles(t, blobDir()); blobs != 3 {
		t.Errorf("two sessions sharing everything must leave 3 blobs, found %d", blobs)
	}
	// Each session's archive references its hashes on disk.
	for _, rec := range []SessionRecord{first, second} {
		raw, err := os.ReadFile(SessionFilePath(rec.Run, rec.Agent))
		if err != nil {
			t.Fatalf("ReadFile session: %v", err)
		}
		var stored SessionRecord
		if err := json.Unmarshal(raw, &stored); err != nil {
			t.Fatalf("session.json is not JSON: %v", err)
		}
		if stored.SystemPrompt != rec.SystemPrompt || stored.Tools != rec.Tools || stored.Flags != rec.Flags {
			t.Errorf("session.json must reference the blobs: %s", raw)
		}
		if stored.Agent != rec.Agent || stored.Run != rec.Run || stored.Harness != "pig" {
			t.Errorf("session.json must name the session: %s", raw)
		}
	}
	// A changed tool set adds exactly one blob; the prompt blob is still shared.
	changed, err := ToolDefsJSON(mustRoleTools(t, "work"))
	if err != nil {
		t.Fatalf("ToolDefsJSON: %v", err)
	}
	third, err := SnapshotSession(SessionSnapshot{
		Run: "run-records", Agent: "agent-3", Harness: "pig", Role: "work",
		SystemPrompt: prompt, ToolsJSON: changed, FlagsJSON: flags,
	})
	if err != nil {
		t.Fatalf("SnapshotSession: %v", err)
	}
	if third.SystemPrompt != first.SystemPrompt {
		t.Errorf("the prompt blob must still be shared: %q vs %q", third.SystemPrompt, first.SystemPrompt)
	}
	if blobs := countFiles(t, blobDir()); blobs != 4 {
		t.Errorf("one changed tool set must add one blob, found %d", blobs)
	}
}

func mustRoleTools(t *testing.T, role string) []tools.Tool {
	t.Helper()
	served, err := RoleTools(role)
	if err != nil {
		t.Fatalf("RoleTools(%q): %v", role, err)
	}
	return served
}

func countFiles(t *testing.T, dir string) int {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	return len(entries)
}

// Seam 3 (ticket 05): transcript copy — the archive tracks a growing harness
// transcript across calls and resumes from its stored offset after a restart,
// so the copy outlives harness cleanup.
func TestTranscriptCopyTracksAGrowingTranscript(t *testing.T) {
	isolateState(t)
	src := writeArchiveSrc(t, "src.jsonl", `{"type":"message","n":1}`+"\n")
	if _, err := CopyTranscript(src, "run-records", "agent-9"); err != nil {
		t.Fatalf("CopyTranscript: %v", err)
	}
	assertFileLines(t, TranscriptCopyPath("run-records", "agent-9"), 1)
	// The harness writes more; the next copy appends only the new bytes.
	appendArchiveLines(t, src, `{"type":"message","n":2}`+"\n", `{"type":"message","n":3}`+"\n")
	if _, err := CopyTranscript(src, "run-records", "agent-9"); err != nil {
		t.Fatalf("CopyTranscript: %v", err)
	}
	assertFileLines(t, TranscriptCopyPath("run-records", "agent-9"), 3)
	// A restart resumes from the stored offset: no line is copied twice.
	off, err := TranscriptOffset("run-records", "agent-9")
	if err != nil {
		t.Fatalf("TranscriptOffset: %v", err)
	}
	info, err := os.Stat(src)
	if err != nil {
		t.Fatal(err)
	}
	if off != info.Size() {
		t.Errorf("offset = %d, want the source size %d", off, info.Size())
	}
	if _, err := CopyTranscript(src, "run-records", "agent-9"); err != nil {
		t.Fatalf("CopyTranscript: %v", err)
	}
	assertFileLines(t, TranscriptCopyPath("run-records", "agent-9"), 3)
	// The source vanishing (harness cleanup) keeps the copy, without error.
	if err := os.Remove(src); err != nil {
		t.Fatal(err)
	}
	if _, err := CopyTranscript(src, "run-records", "agent-9"); err != nil {
		t.Fatalf("a cleaned-up transcript must keep the copy: %v", err)
	}
	assertFileLines(t, TranscriptCopyPath("run-records", "agent-9"), 3)
}

func writeArchiveSrc(t *testing.T, name, text string) string {
	t.Helper()
	path := t.TempDir() + "/" + name
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func appendArchiveLines(t *testing.T, path string, lines ...string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for _, l := range lines {
		if _, err := f.WriteString(l); err != nil {
			t.Fatal(err)
		}
	}
}

func assertFileLines(t *testing.T, path string, want int) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile %s: %v", path, err)
	}
	got := 0
	for _, l := range strings.Split(string(raw), "\n") {
		if strings.TrimSpace(l) != "" {
			got++
		}
	}
	if got != want {
		t.Errorf("%s holds %d lines, want %d:\n%s", path, got, want, raw)
	}
}

// Seam 4 (ticket 05): session wiring — every launched session tells its
// wick-mcp who it is (session id, harness, model, effort), so the session's
// own wick process can snapshot the blobs and tail the transcript.
func TestEverySessionTellsItsWickProcessWhoItIs(t *testing.T) {
	isolateState(t)
	for _, harness := range HarnessNames() {
		cmd, err := BuildCommand(harness, Session{
			Role: "work", ID: "sess-1", Label: "01-x", Model: "m1", Effort: "high",
			Dir: "/sessions",
		})
		if err != nil {
			t.Fatal(err)
		}
		env := map[string]string{}
		for _, e := range cmd.Env {
			k, v, _ := strings.Cut(e, "=")
			env[k] = v
		}
		for k, want := range map[string]string{
			"WICK_SESSION_ID": "sess-1",
			"WICK_HARNESS":    harness,
			"WICK_MODEL":      "m1",
			"WICK_EFFORT":     "high",
		} {
			if env[k] != want {
				t.Errorf("%s: %s = %q, want %q in %q", harness, k, env[k], want, cmd.Env)
			}
		}
		// Only pig takes a session dir: its transcript lives under it, so
		// only pig sessions name it. Claude takes none.
		if dir, ok := env["WICK_SESSION_DIR"]; ok != (harness == "pig") || (ok && dir != "/sessions") {
			t.Errorf("%s: WICK_SESSION_DIR = %q, present = %v", harness, dir, ok)
		}
		// The MCP config claude serves its wick-mcp must carry the same
		// identity: the child reads env, not the launch line.
		if harness == "claude" {
			cfg, err := readLaunchConfig(cmd)
			if err != nil {
				t.Fatal(err)
			}
			for k, want := range map[string]string{
				"WICK_SESSION_ID": "sess-1",
				"WICK_HARNESS":    "claude",
			} {
				if cfg[k] != want {
					t.Errorf("claude MCP env %s = %q, want %q", k, cfg[k], want)
				}
			}
		}
	}
}

// Seam 5 (ticket 05): the session's own wick process snapshots the blobs at
// start and tails its harness transcript — exercised here through the same
// env-driven helpers wick-mcp runs, against a fake pig session dir.
func TestArchiverSnapshotsAndTailsItsOwnTranscript(t *testing.T) {
	isolateState(t)
	sessDir := t.TempDir()
	t.Setenv("WICK_AGENT_ID", "01-archive")
	t.Setenv("WICK_SESSION_ID", "sess-archive")
	t.Setenv("WICK_SESSION_DIR", sessDir)
	t.Setenv("WICK_HARNESS", "pig")
	t.Setenv("WICK_ROLE", "review")
	t.Setenv("WICK_MODEL", "m1")
	t.Setenv("WICK_EFFORT", "high")
	t.Setenv("WICK_SPEC", "/repo/.scratch/run-records/spec.md")
	served, err := RoleTools("review")
	if err != nil {
		t.Fatal(err)
	}
	if err := SnapshotFromEnv(served); err != nil {
		t.Fatalf("SnapshotFromEnv: %v", err)
	}
	rec := readSession(t, "run-records", "01-archive")
	if rec.Harness != "pig" || rec.Role != "review" {
		t.Errorf("snapshot must name the session: %+v", rec)
	}
	for _, h := range []string{rec.SystemPrompt, rec.Tools, rec.Flags} {
		if _, err := ReadBlob(h); err != nil {
			t.Errorf("snapshot hash %q must resolve in the blob store: %v", h, err)
		}
	}
	// The harness writes its transcript after the session starts; copies tail it.
	src := filepath.Join(sessDir, "t_sess-archive.jsonl")
	if _, err := CopyFromEnv(); err != nil {
		t.Fatalf("CopyFromEnv before any transcript: %v", err)
	}
	if err := os.WriteFile(src, []byte(`{"n":1}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := CopyFromEnv(); err != nil {
		t.Fatalf("CopyFromEnv: %v", err)
	}
	assertFileLines(t, TranscriptCopyPath("run-records", "01-archive"), 1)
	appendArchiveLines(t, src, `{"n":2}`+"\n")
	if _, err := CopyFromEnv(); err != nil {
		t.Fatalf("CopyFromEnv: %v", err)
	}
	assertFileLines(t, TranscriptCopyPath("run-records", "01-archive"), 2)
}

// readLaunchConfig decodes the MCP config a claude launch wrote for its
// wick-mcp: the env the child reads.
func readLaunchConfig(cmd Command) (map[string]string, error) {
	path := ""
	for i, a := range cmd.Args {
		if a == "--mcp-config" && i+1 < len(cmd.Args) {
			path = cmd.Args[i+1]
		}
	}
	if path == "" {
		return nil, errNoConfig
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg struct {
		Servers map[string]struct {
			Env map[string]string `json:"env"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, err
	}
	return cfg.Servers["wick"].Env, nil
}

var errNoConfig = errConfig("no --mcp-config in the launch")

type errConfig string

func (e errConfig) Error() string { return string(e) }

func readSession(t *testing.T, run, agent string) SessionRecord {
	t.Helper()
	raw, err := os.ReadFile(SessionFilePath(run, agent))
	if err != nil {
		t.Fatalf("ReadFile session: %v", err)
	}
	var rec SessionRecord
	if err := json.Unmarshal(raw, &rec); err != nil {
		t.Fatalf("session.json is not JSON: %v", err)
	}
	return rec
}

// A corrupt offset file is an error, never a silently accepted position:
// trailing garbage, words and negatives must all fail.
func TestCorruptTranscriptOffsetIsAnError(t *testing.T) {
	isolateState(t)
	src := writeArchiveSrc(t, "src.jsonl", "{\"n\":1}\n")
	if _, err := CopyTranscript(src, "run-records", "agent-corrupt"); err != nil {
		t.Fatalf("CopyTranscript: %v", err)
	}
	for _, bad := range []string{"12abc\n", "nope\n", "-3\n", "1 2 3\n", "\n"} {
		if err := os.WriteFile(TranscriptOffsetPath("run-records", "agent-corrupt"), []byte(bad), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := TranscriptOffset("run-records", "agent-corrupt"); err == nil {
			t.Errorf("offset %q must be corrupt, got no error", bad)
		}
		if _, err := CopyTranscript(src, "run-records", "agent-corrupt"); err == nil {
			t.Errorf("copy with offset %q must fail, got no error", bad)
		}
	}
}

// A crash between the copy's append and its offset write must not duplicate
// bytes: the next copy cuts the torn tail back instead of re-copying it.
func TestTranscriptCopyRepairsATornTail(t *testing.T) {
	isolateState(t)
	src := writeArchiveSrc(t, "src.jsonl", "{\"n\":1}\n")
	if _, err := CopyTranscript(src, "run-records", "agent-torn"); err != nil {
		t.Fatalf("CopyTranscript: %v", err)
	}
	// Simulate the crash: bytes appended to the copy but never recorded.
	appendArchiveLines(t, TranscriptCopyPath("run-records", "agent-torn"), "{\"n\":1}\n")
	if _, err := CopyTranscript(src, "run-records", "agent-torn"); err != nil {
		t.Fatalf("CopyTranscript: %v", err)
	}
	assertFileLines(t, TranscriptCopyPath("run-records", "agent-torn"), 1)
	// And the repair holds when the source grows afterwards.
	appendArchiveLines(t, src, "{\"n\":2}\n")
	if _, err := CopyTranscript(src, "run-records", "agent-torn"); err != nil {
		t.Fatalf("CopyTranscript: %v", err)
	}
	assertFileLines(t, TranscriptCopyPath("run-records", "agent-torn"), 2)
}

// A reviewer, scout or read-only agent spawned inside a run archives under
// that run: its session inherits the spawner's spec, so its transcript copy
// and session record land under archive/<run>, never archive/adhoc.
func TestSpawnedAgentsInheritTheRunSpec(t *testing.T) {
	const spec = "/repo/.scratch/run-records/spec.md"
	t.Setenv("WICK_SPEC", spec)
	// Scouts inherit the spec through their launch env.
	spawnTestState(t)
	g := newFakeGrid()
	t.Setenv("HERDR_TAB_ID", g.tabs[0].tab)
	ex := &scoutExec{g: g, writeOn: 1}
	spawnExec = ex.run
	spawn, _ := spawnToolForRole(t, "implement")
	if _, err := spawn("/repo", map[string]any{"agents": []any{
		map[string]any{"type": "scout", "prompt": "where is the auth check?"},
	}}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if len(ex.lines) != 1 {
		t.Fatalf("launched %d scouts, want 1", len(ex.lines))
	}
	if got := envIn(ex.lines[0], "WICK_SPEC"); got != spec {
		t.Errorf("scout WICK_SPEC = %q, want %q", got, spec)
	}
	// Read-only sessions carry the caller's spec.
	if sess := readOnlySession("id-1", t.TempDir(), "", ""); sess.Spec != spec {
		t.Errorf("read-only session spec = %q, want %q", sess.Spec, spec)
	}
	// Reviewers inherit the spec through their launch env.
	_, tree := reviewRepo(t)
	f := newFakeReviewers(answers(verdictPass, verdictPass))
	if _, err := reviewSpawn(t, f, tree, bothReviewers()...); err != nil {
		t.Fatal(err)
	}
	for _, typ := range []string{reviewerStandards, reviewerSpec} {
		if got := envIn(f.paneOf(t, typ).line, "WICK_SPEC"); got != spec {
			t.Errorf("%s reviewer WICK_SPEC = %q, want %q", typ, got, spec)
		}
	}
}

// A copy that lost recorded bytes is rebuilt from the source start, not
// resumed mid-file: the archive never holds a hole.
func TestTranscriptCopyRebuildsAShortenedCopy(t *testing.T) {
	isolateState(t)
	src := writeArchiveSrc(t, "src.jsonl", "{\"n\":1}\n{\"n\":2}\n")
	if _, err := CopyTranscript(src, "run-records", "agent-short"); err != nil {
		t.Fatalf("CopyTranscript: %v", err)
	}
	// Simulate external loss: the copy keeps only its first line.
	raw, err := os.ReadFile(TranscriptCopyPath("run-records", "agent-short"))
	if err != nil {
		t.Fatal(err)
	}
	first, _, _ := strings.Cut(string(raw), "\n")
	if err := os.WriteFile(TranscriptCopyPath("run-records", "agent-short"), []byte(first+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	appendArchiveLines(t, src, "{\"n\":3}\n")
	if _, err := CopyTranscript(src, "run-records", "agent-short"); err != nil {
		t.Fatalf("CopyTranscript: %v", err)
	}
	want, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(TranscriptCopyPath("run-records", "agent-short"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("rebuilt copy = %q, want the full source %q", got, want)
	}
}

// The first copy that finds the harness transcript records where it lives,
// so session.json names the source the stats indexer reads.
func TestFirstCopyRecordsTheTranscriptSource(t *testing.T) {
	isolateState(t)
	sessDir := t.TempDir()
	t.Setenv("WICK_AGENT_ID", "01-source")
	t.Setenv("WICK_SESSION_ID", "sess-source")
	t.Setenv("WICK_SESSION_DIR", sessDir)
	t.Setenv("WICK_HARNESS", "pig")
	t.Setenv("WICK_ROLE", "review")
	t.Setenv("WICK_SPEC", "/repo/.scratch/run-records/spec.md")
	served, err := RoleTools("review")
	if err != nil {
		t.Fatal(err)
	}
	if err := SnapshotFromEnv(served); err != nil {
		t.Fatalf("SnapshotFromEnv: %v", err)
	}
	// The snapshot runs before the first turn: no source yet.
	if rec := readSession(t, "run-records", "01-source"); rec.TranscriptSource != "" {
		t.Fatalf("snapshot before any transcript must record no source, got %q", rec.TranscriptSource)
	}
	src := filepath.Join(sessDir, "t_sess-source.jsonl")
	if err := os.WriteFile(src, []byte("{\"n\":1}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := CopyFromEnv(); err != nil {
		t.Fatalf("CopyFromEnv: %v", err)
	}
	if rec := readSession(t, "run-records", "01-source"); rec.TranscriptSource != src {
		t.Errorf("first copy must record the source %q, got %q", src, rec.TranscriptSource)
	}
	// A corrupt session record surfaces instead of copying blindly.
	if err := os.WriteFile(SessionFilePath("run-records", "01-source"), []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := CopyFromEnv(); err == nil {
		t.Errorf("a corrupt session record must fail the copy, got no error")
	}
}

// A harness command carries the first message exactly once: a doubled prompt
// would send the brief to the agent twice.
func TestHarnessCommandCarriesThePromptExactlyOnce(t *testing.T) {
	isolateState(t)
	for _, harness := range HarnessNames() {
		cmd, err := BuildCommand(harness, Session{Role: "work", ID: "sess-1", Prompt: "the brief"})
		if err != nil {
			t.Fatal(err)
		}
		got := 0
		for _, a := range cmd.Args {
			if a == "the brief" {
				got++
			}
		}
		if got != 1 {
			t.Errorf("%s: prompt appears %d times in args %q, want once", harness, got, cmd.Args)
		}
	}
}

// ArchiveSize covers the archive and blob store for the dashboard's Settings
// tab: an empty state reads zero, and every file under both directories
// counts.
func TestArchiveSizeCoversArchiveAndBlobs(t *testing.T) {
	isolateState(t)
	if got := ArchiveSize(); got != 0 {
		t.Fatalf("empty state ArchiveSize = %d, want 0", got)
	}
	writeSized := func(path string, n int) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(strings.Repeat("x", n)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeSized(filepath.Join(archiveRoot(), "spec-a", "w1", "transcript.jsonl"), 100)
	writeSized(filepath.Join(archiveRoot(), "spec-a", "w1", "session.json"), 50)
	writeSized(filepath.Join(blobDir(), "abc123"), 25)
	if got, want := ArchiveSize(), int64(175); got != want {
		t.Fatalf("ArchiveSize = %d, want %d", got, want)
	}
}
