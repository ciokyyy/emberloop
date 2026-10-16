package web

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"example.com/emberloop-web/memory"
)

// seedEndedSession stores evs as Session id's events.jsonl and in-memory
// history with an ended meta, so the miner treats it as settled. Seqs and
// timestamps are filled when zero.
func seedEndedSession(t *testing.T, d *Daemon, id, owner string, evs []Event) {
	t.Helper()
	for i := range evs {
		evs[i].SessionID = id
		if evs[i].Seq == 0 {
			evs[i].Seq = int64(i + 1)
		}
		if evs[i].CreatedAt.IsZero() {
			evs[i].CreatedAt = time.Now().UTC()
		}
	}
	d.mu.Lock()
	d.sessions[id] = &session{
		meta:   SessionMeta{ID: id, Owner: owner, Status: "ended", CreatedAt: time.Now().UTC()},
		events: append([]Event(nil), evs...),
		notify: make(chan struct{}),
		last:   time.Now().UTC(),
	}
	d.mu.Unlock()
	dir := d.sessionDir(id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	for _, e := range evs {
		b, err := json.Marshal(e)
		if err != nil {
			t.Fatal(err)
		}
		sb.Write(b)
		sb.WriteByte('\n')
	}
	if err := os.WriteFile(filepath.Join(dir, "events.jsonl"), []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

func userPrompt(seq int64, user, text string) Event {
	return Event{Kind: "prompt", Sender: senderUser, User: user, Text: text, Seq: seq}
}

func bankQuotes(t *testing.T, d *Daemon, user, query string) []memory.Observation {
	t.Helper()
	bank, err := d.profileBankFor(user)
	if err != nil {
		t.Fatal(err)
	}
	obs, err := bank.SearchObservations(t.Context(), query, 20)
	if err != nil {
		t.Fatal(err)
	}
	return obs
}

func TestMineSessionSkipsNonUserEvents(t *testing.T) {
	t.Parallel()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	evs := []Event{
		{Kind: "tool_call_started", Text: `{"name":"write","input":"x"}`, Seq: 1},
		userPrompt(2, "alice", "No, use tabs here"),
		{Kind: "prompt", Sender: senderSystem, Text: "grouped result", Seq: 3},
		{Kind: "prompt", Sender: senderSystem, Handoff: true, Text: "# Handoff", Seq: 4},
		{Kind: "injection", Sender: senderSystem, Text: InjectionEvent{Rules: []InjectedRule{{ID: "r", Version: 1}}}.text(), Seq: 5},
		{Kind: "assistant_chunk", Text: "ok", Seq: 6},
	}
	seedEndedSession(t, d, "s1", "alice", evs)
	res, err := d.MineSession("s1")
	if err != nil {
		t.Fatal(err)
	}
	if res.Scanned != len(evs) || res.Kept != 1 || res.Stored != 1 || res.Duplicates != 0 {
		t.Fatalf("result = %+v, want 6 scanned, 1 kept, 1 stored", res)
	}
	obs := bankQuotes(t, d, "alice", "tabs")
	if len(obs) != 1 || obs[0].Quote != "No, use tabs here" || obs[0].Author != "alice" {
		t.Fatalf("observations = %+v, want the one user correction", obs)
	}
	if err := memory.CheckGrounding(obs[0].EventID, obs[0].Quote, memory.GroundEvent{Found: true, Sender: "user", Text: "No, use tabs here"}); err != nil {
		t.Fatalf("stored observation is not grounded: %v", err)
	}
}

func TestMineSessionGoldenFilters(t *testing.T) {
	t.Parallel()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	cases := []struct {
		name   string
		text   string
		stored int
	}{
		{"guardrail", "don't add tests unless asked", 1},
		{"deny", "no problem, thanks", 0},
		{"pivot", "Perfect! Now let's add pagination", 0},
		{"praise", "Perfect!", 0},
		{"cjk", "いや、そうじゃなくてタブを使って", 1},
	}
	for i, c := range cases {
		id := "golden-" + string(rune('a'+i))
		seedEndedSession(t, d, id, "alice", []Event{userPrompt(1, "alice", c.text)})
		res, err := d.MineSession(id)
		if err != nil {
			t.Fatal(err)
		}
		if res.Stored != c.stored {
			t.Errorf("%s: stored = %d, want %d (%+v)", c.name, res.Stored, c.stored, res)
		}
	}
}

func TestMineSessionRoutesToSenderBank(t *testing.T) {
	t.Parallel()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	evs := []Event{
		userPrompt(1, "alice", "No, use tabs here"),
		{Kind: "prompt", Mode: "steer", Sender: senderUser, User: "bob", Text: "don't use mocks for this package", Seq: 2},
	}
	seedEndedSession(t, d, "s1", "alice", evs)
	if _, err := d.MineSession("s1"); err != nil {
		t.Fatal(err)
	}
	alice := bankQuotes(t, d, "alice", "tabs")
	if len(alice) != 1 || alice[0].Author != "alice" {
		t.Fatalf("alice bank = %+v, want her prompt only", alice)
	}
	if leaked := bankQuotes(t, d, "alice", "mocks"); len(leaked) != 0 {
		t.Fatalf("alice bank holds bob's steer: %+v", leaked)
	}
	bob := bankQuotes(t, d, "bob", "mocks")
	if len(bob) != 1 || bob[0].Author != "bob" {
		t.Fatalf("bob bank = %+v, want his steer only", bob)
	}
}

func TestMineSessionContradictsNotMerged(t *testing.T) {
	t.Parallel()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	a := "No, always return wrapped errors in this repo"
	b := "No, never return wrapped errors in this repo"
	seedEndedSession(t, d, "s1", "alice", []Event{userPrompt(1, "alice", a), userPrompt(2, "alice", b)})
	res, err := d.MineSession("s1")
	if err != nil {
		t.Fatal(err)
	}
	if res.Stored != 2 {
		t.Fatalf("result = %+v, want 2 stored, nothing merged", res)
	}
	obs := bankQuotes(t, d, "alice", "wrapped errors")
	if len(obs) != 2 {
		t.Fatalf("observations = %d, want 2", len(obs))
	}
	if got := memory.Relation(obs[0].Quote, obs[1].Quote); got != memory.RelContradicts {
		t.Fatalf("relation = %q, want contradicts", got)
	}
	// The later observation records the relation and the related row, so
	// the queue stage reads it without recomputing from quotes.
	var first, second memory.Observation
	for _, o := range obs {
		if strings.Contains(o.Quote, "always return") {
			first = o
		} else {
			second = o
		}
	}
	var doc struct {
		Relation string `json:"relation"`
		Related  int64  `json:"related_observation_id"`
	}
	if err := json.Unmarshal([]byte(second.DeciderJSON), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Relation != memory.RelContradicts || doc.Related != first.ID {
		t.Fatalf("stored relation = %+v, want contradicts row %d", doc, first.ID)
	}
	if strings.Contains(first.DeciderJSON, `"relation"`) {
		t.Fatalf("novel observation carries a relation: %s", first.DeciderJSON)
	}
}

func TestMineSessionContextComesFromBefore(t *testing.T) {
	t.Parallel()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	evs := []Event{
		{Kind: "tool_call_started", Text: `{"name":"first-tool","input":"a"}`, Seq: 1},
		userPrompt(2, "alice", "that's wrong, the retry belongs in the caller"),
		{Kind: "tool_call_started", Text: `{"name":"later-tool","input":"b"}`, Seq: 3},
	}
	seedEndedSession(t, d, "s1", "alice", evs)
	if _, err := d.MineSession("s1"); err != nil {
		t.Fatal(err)
	}
	got := bankQuotes(t, d, "alice", "retry")
	if len(got) != 1 {
		t.Fatalf("observations = %+v, want 1", got)
	}
	var ctx struct {
		Context string `json:"context"`
	}
	if err := json.Unmarshal([]byte(got[0].DeciderJSON), &ctx); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ctx.Context, "first-tool") {
		t.Fatalf("context = %q, want the preceding tool call", ctx.Context)
	}
	if strings.Contains(ctx.Context, "later-tool") {
		t.Fatalf("context = %q, want nothing from after the correction", ctx.Context)
	}
}

func TestMineSessionRedactsBeforeStorage(t *testing.T) {
	t.Parallel()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	text := "remember: the password is hunter2-2026, always use tabs"
	seedEndedSession(t, d, "s1", "alice", []Event{userPrompt(1, "alice", text)})
	if _, err := d.MineSession("s1"); err != nil {
		t.Fatal(err)
	}
	obs := bankQuotes(t, d, "alice", "tabs")
	if len(obs) != 1 {
		t.Fatalf("observations = %+v, want 1", obs)
	}
	if strings.Contains(obs[0].Quote, "hunter2") {
		t.Fatalf("secret stored raw: %q", obs[0].Quote)
	}
	if !strings.Contains(obs[0].Quote, "[redacted:password]") {
		t.Fatalf("redaction marker missing: %q", obs[0].Quote)
	}
	if err := memory.CheckGrounding(obs[0].EventID, obs[0].Quote, memory.GroundEvent{
		Found: true, Sender: "user", Text: memory.Redact(text),
	}); err != nil {
		t.Fatalf("redacted observation is not grounded: %v", err)
	}
}

func TestMineSessionStoresCorrectedContext(t *testing.T) {
	t.Parallel()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	evs := []Event{
		{Kind: "tool_call_started", Text: `{"name":"write","input":"main.go"}`, Seq: 1},
		userPrompt(2, "alice", "that's wrong, the retry belongs in the caller"),
	}
	seedEndedSession(t, d, "s1", "alice", evs)
	if _, err := d.MineSession("s1"); err != nil {
		t.Fatal(err)
	}
	obs := bankQuotes(t, d, "alice", "retry")
	if len(obs) != 1 {
		t.Fatalf("observations = %+v, want 1", obs)
	}
	var doc struct {
		Context string `json:"context"`
	}
	if err := json.Unmarshal([]byte(obs[0].DeciderJSON), &doc); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(doc.Context, "write") {
		t.Fatalf("context = %q, want the preceding tool call", doc.Context)
	}
}

func TestMineSessionCursorResetAndTornLine(t *testing.T) {
	t.Parallel()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	id := "s1"
	e1 := userPrompt(1, "alice", "No, use tabs here")
	e2 := userPrompt(2, "alice", "don't use mocks for this package")
	seedEndedSession(t, d, id, "alice", []Event{e1})
	// A torn last line waits: append the first half of e2 with no newline.
	logPath := filepath.Join(d.sessionDir(id), "events.jsonl")
	b2, _ := json.Marshal(e2)
	half := len(b2) / 2
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(b2[:half]); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	res, err := d.MineSession(id)
	if err != nil {
		t.Fatal(err)
	}
	if res.Scanned != 1 || res.Stored != 1 {
		t.Fatalf("torn read = %+v, want the 1 full line mined", res)
	}
	// Complete the torn line by appending its second half and a newline.
	f, err = os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(append(append([]byte(nil), b2[half:]...), '\n')); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	// Sync the in-memory history with the completed file.
	d.mu.Lock()
	d.sessions[id].events = []Event{e1, e2}
	d.mu.Unlock()
	res, err = d.MineSession(id)
	if err != nil {
		t.Fatal(err)
	}
	if res.Scanned != 1 || res.Stored != 1 || res.Reset {
		t.Fatalf("completed read = %+v, want the 1 finished line mined with no reset", res)
	}
	if n := len(bankQuotes(t, d, "alice", "tabs")) + len(bankQuotes(t, d, "alice", "mocks")); n != 2 {
		t.Fatalf("bank holds %d observations, want 2", n)
	}
	// A rewritten head resets the cursor but replays store nothing new:
	// same Session Events return without bumping.
	rewrite := string(b2) + "\n"
	first, _ := json.Marshal(e1)
	rewrite += string(first) + "\n"
	if err := os.WriteFile(logPath, []byte(rewrite), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err = d.MineSession(id)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Reset {
		t.Fatalf("rewrite read = %+v, want reset", res)
	}
	for _, q := range []string{"tabs", "mocks"} {
		for _, o := range bankQuotes(t, d, "alice", q) {
			if o.EvidenceCount != 1 {
				t.Fatalf("replay bumped evidence: %+v", o)
			}
		}
	}
}

func TestMineSessionOnlyWhenSettled(t *testing.T) {
	t.Parallel()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	evs := []Event{userPrompt(1, "alice", "No, use tabs here")}
	for i := range evs {
		evs[i].SessionID = "live"
		evs[i].CreatedAt = time.Now().UTC()
	}
	d.mu.Lock()
	d.sessions["live"] = &session{
		meta:   SessionMeta{ID: "live", Owner: "alice", Status: "idle", CreatedAt: time.Now().UTC()},
		events: append([]Event(nil), evs...),
		notify: make(chan struct{}),
		last:   time.Now().UTC(),
	}
	d.mu.Unlock()
	dir := d.sessionDir("live")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(evs[0])
	if err := os.WriteFile(filepath.Join(dir, "events.jsonl"), append(b, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := d.MineSession("live")
	if err != nil {
		t.Fatal(err)
	}
	if res.Scanned != 0 || res.Stored != 0 {
		t.Fatalf("live session mined = %+v, want nothing before it settles", res)
	}
	d.mu.Lock()
	m := d.sessions["live"].meta
	m.Status = "ended"
	d.sessions["live"].meta = m
	d.mu.Unlock()
	res, err = d.MineSession("live")
	if err != nil {
		t.Fatal(err)
	}
	if res.Stored != 1 {
		t.Fatalf("ended session mined = %+v, want 1 stored", res)
	}
}

func TestMineSessionBankErrorHoldsCursor(t *testing.T) {
	t.Parallel()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	seedEndedSession(t, d, "s1", "alice", []Event{userPrompt(1, "alice", "No, use tabs here")})
	// Break the author's bank: the store fails transiently, so the scan
	// must report the error and leave the cursor unsaved for a retry.
	bank, err := d.profileBankFor("alice")
	if err != nil {
		t.Fatal(err)
	}
	if err := bank.(*memory.Bank).Close(); err != nil {
		t.Fatal(err)
	}
	res, err := d.MineSession("s1")
	if err == nil {
		t.Fatalf("mine with a broken bank = %+v, want an error", res)
	}
	if res.Stored != 0 {
		t.Fatalf("mine with a broken bank stored %d, want 0", res.Stored)
	}
	if _, statErr := os.Stat(d.minerCursorPath("s1")); !os.IsNotExist(statErr) {
		t.Fatalf("cursor saved despite the store error: %v", statErr)
	}
}

// A subject key recurring across Sessions promotes on its own: mining the
// second Session enqueues one proposal with both quotes, with no manual
// promote call. A single Session's key enqueues nothing.
func TestMineSessionPromotesRecurringSubject(t *testing.T) {
	t.Parallel()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	a := "No, always return wrapped errors in this repo"
	b := "No, never return wrapped errors from handlers"
	seedEndedSession(t, d, "s1", "alice", []Event{userPrompt(1, "alice", a)})
	res, err := d.MineSession("s1")
	if err != nil {
		t.Fatal(err)
	}
	if res.Stored != 1 || res.Promoted != 0 {
		t.Fatalf("first mine = %+v, want 1 stored and no promotion from a single mention", res)
	}
	bank, err := d.profileBankFor("alice")
	if err != nil {
		t.Fatal(err)
	}
	items, err := bank.Queue(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if it.Kind == memory.QueueRule {
			t.Fatalf("single mention queued %+v, want nothing", it)
		}
	}
	seedEndedSession(t, d, "s2", "alice", []Event{userPrompt(1, "alice", b)})
	res, err = d.MineSession("s2")
	if err != nil {
		t.Fatal(err)
	}
	if res.Stored != 1 || res.Promoted != 0 {
		t.Fatalf("second mine = %+v, want stored observation without legacy subject-key promotion", res)
	}
	items, _ = bank.Queue(t.Context())
	for _, it := range items {
		if it.Kind == memory.QueueRule {
			t.Fatalf("legacy subject-key candidate entered queue: %+v", it)
		}
	}
	obs := bankQuotes(t, d, "alice", "wrapped errors")
	if len(obs) != 2 {
		t.Fatalf("observations = %d, want 2", len(obs))
	}
	for _, o := range obs {
		if o.SubjectKey == "" {
			t.Fatalf("observation %d has no legacy subject key", o.ID)
		}
	}
}

// The subject key comes from the confirming Decider when it proposes one:
// the scripted test seam names the key the miner stores.
func TestMineSessionSubjectKeyFromDecider(t *testing.T) {
	t.Parallel()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	d.decider = &memory.ScriptedDecider{Durable: 0.9, Kind: memory.KindPreference, SubjectKey: "Scripted.Subject.Key"}
	seedEndedSession(t, d, "s1", "alice", []Event{userPrompt(1, "alice", "No, use tabs here")})
	if _, err := d.MineSession("s1"); err != nil {
		t.Fatal(err)
	}
	obs := bankQuotes(t, d, "alice", "tabs")
	if len(obs) != 1 {
		t.Fatalf("observations = %+v, want 1", obs)
	}
	if obs[0].SubjectKey != "scripted.subject.key" {
		t.Fatalf("subject key = %q, want the Decider proposal normalized", obs[0].SubjectKey)
	}
}

// The subject key never carries secrets: the miner keys from the redacted
// quote, so a secret in the topic tokens lands redacted in the observation,
// the decider_json and any promoted queue payload.
func TestMineSessionRedactsSubjectKey(t *testing.T) {
	t.Parallel()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	a := "No, always store it where the admin password is hunter2-2026 in the vault"
	b := "No, never store it where the admin password is hunter2-2026 in the config"
	seedEndedSession(t, d, "s1", "alice", []Event{userPrompt(1, "alice", a)})
	if _, err := d.MineSession("s1"); err != nil {
		t.Fatal(err)
	}
	obs := bankQuotes(t, d, "alice", "vault")
	if len(obs) != 1 {
		t.Fatalf("observations = %+v, want 1", obs)
	}
	if strings.Contains(obs[0].SubjectKey, "hunter2") || strings.Contains(obs[0].DeciderJSON, "hunter2") {
		t.Fatalf("secret leaked into key material: %+v", obs[0])
	}
	if obs[0].SubjectKey == "" {
		t.Fatalf("redacted quote proposed no key: %+v", obs[0])
	}
	seedEndedSession(t, d, "s2", "alice", []Event{userPrompt(1, "alice", b)})
	res, err := d.MineSession("s2")
	if err != nil {
		t.Fatal(err)
	}
	if res.Promoted != 0 {
		t.Fatalf("second mine = %+v, want no subject-key recurrence promotion", res)
	}
	bank, err := d.profileBankFor("alice")
	if err != nil {
		t.Fatal(err)
	}
	items, _ := bank.Queue(t.Context())
	for _, it := range items {
		if it.Kind == memory.QueueRule && strings.Contains(it.Payload, "hunter2") {
			t.Fatalf("secret leaked into the queue payload: %s", it.Payload)
		}
	}
}
