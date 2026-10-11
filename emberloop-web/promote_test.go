package web

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"example.com/emberloop-web/memory"
)

// promoteGitEnv isolates git the way runs_driver_test does.
func promoteGitEnv(t *testing.T) {
	t.Helper()
	t.Setenv("GIT_AUTHOR_NAME", "web-test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "web-test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("HOME", tempDir(t))
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
}

func promoteGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// initPromoteRepo creates a main-branch repo with the given files committed.
func initPromoteRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	promoteGitEnv(t)
	repo := tempDir(t)
	promoteGit(t, repo, "init", "-b", "main", ".")
	for name, content := range files {
		writeFile(t, repo, name, content)
	}
	promoteGit(t, repo, "add", "-A")
	promoteGit(t, repo, "commit", "-m", "base")
	return repo
}

// promoteDaemon starts a Daemon on the fake Backend with one Session rooted
// at repo, returning the Daemon, base URL, Session ID and owner.
func promoteDaemon(t *testing.T, repo string) (*Daemon, string, string, string) {
	t.Helper()
	t.Setenv("PIG_TEST_FAUX", "1")
	useSettings(t, `{"compress_model":"test-faux/faux-1"}`)
	d := NewDaemon(tempDir(t), NewFakeBackend())
	d.summarizer = &scriptedSummarizer{replies: []string{goodSummary}}
	srv := httptest.NewServer(authedHandler(t, d))
	t.Cleanup(srv.Close)
	id := createSession(t, srv.URL, "fake", "fake-model", repo)
	meta, _ := sessionMeta(t, srv.URL, id)
	return d, srv.URL, id, meta.Owner
}

func addPromoteRule(t *testing.T, d *Daemon, owner string, obsIDs []int64) memory.Rule {
	t.Helper()
	bank, err := d.profileBankFor(owner)
	if err != nil {
		t.Fatal(err)
	}
	rule, err := bank.AddRule(context.Background(), memory.Rule{
		Text: "always wrap errors in this repo", Kind: memory.KindPreference,
		SubjectKey: "wrap.errors", ApprovedBy: owner,
		SourceObservationIDs: obsIDs,
	})
	if err != nil {
		t.Fatal(err)
	}
	return rule
}

func addPromoteObservation(t *testing.T, d *Daemon, owner, sessionID string) int64 {
	t.Helper()
	bank, err := d.profileBankFor(owner)
	if err != nil {
		t.Fatal(err)
	}
	id, err := bank.AddObservation(context.Background(), memory.Observation{
		SessionID: sessionID, EventID: "e1", Author: owner,
		Quote: "always wrap errors in this repo", Kind: memory.KindPreference,
		SubjectKey: "wrap.errors",
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func postPromotion(t *testing.T, base, ruleID, sessionID, target string) (int, map[string]any) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"rule_id": ruleID, "session_id": sessionID, "target": target})
	resp, err := http.Post(base+"/api/promotions", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func postSessionPromote(t *testing.T, base, sessionID, ruleID, target string) (int, map[string]any) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"rule_id": ruleID, "target": target})
	resp, err := http.Post(base+"/api/sessions/"+sessionID+"/promote", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func getPromotionQueue(t *testing.T, base string) []promotionView {
	t.Helper()
	resp, err := http.Get(base + "/api/promotions/queue")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/promotions/queue = %d, want 200", resp.StatusCode)
	}
	var out []promotionView
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode promotions queue: %v", err)
	}
	return out
}

func decidePromotion(t *testing.T, base string, id int64, body any) (int, map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(body)
	resp, err := http.Post(base+"/api/promotions/queue/"+strconv.FormatInt(id, 10)+"/decide", "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

// Promoting a Rule produces a proposed diff against the nearest AGENTS.md
// shown for approval.
func TestPromoteProducesDiffAgainstNearestAgentsMD(t *testing.T) {
	repo := initPromoteRepo(t, map[string]string{
		"AGENTS.md":         "# AGENTS.md — demo\n\nContract.\n",
		"sub/AGENTS.md":     "# AGENTS.md — sub\n\nSub contract.\n",
		"sub/nested/app.go": "package app\n",
	})
	d, base, _, owner := promoteDaemon(t, repo)
	// A Session deep in sub/ must propose against sub/AGENTS.md, not the root.
	nested := filepath.Join(repo, "sub", "nested")
	nestedSession := createSession(t, base, "fake", "fake-model", nested)
	rule := addPromoteRule(t, d, owner, nil)
	code, out := postPromotion(t, base, rule.RuleID, nestedSession, "agents_md")
	if code != http.StatusCreated {
		t.Fatalf("propose = %d %v, want 201", code, out)
	}
	if out["target_path"] != "sub/AGENTS.md" {
		t.Fatalf("target_path = %v, want sub/AGENTS.md", out["target_path"])
	}
	diff, _ := out["diff"].(string)
	if !strings.Contains(diff, "sub/AGENTS.md") || !strings.Contains(diff, rule.Text) {
		t.Fatalf("diff = %q, want the nearest path and the rule text", diff)
	}
	queue := getPromotionQueue(t, base)
	if len(queue) != 1 || queue[0].Diff == "" {
		t.Fatalf("queue = %+v, want the pending promotion with its diff", queue)
	}
	// The chat action proposes the same Rule from the Session itself.
	code, chat := postSessionPromote(t, base, nestedSession, rule.RuleID, "")
	if code != http.StatusOK {
		t.Fatalf("chat promote = %d %v, want 200 with the existing item", code, chat)
	}
	if getPromotionQueue(t, base)[0].ID != queue[0].ID {
		t.Fatalf("chat action enqueued a duplicate promotion")
	}
}

// Approval writes the file in the Session's worktree and leaves it
// uncommitted; the Rule is marked promoted with its provenance intact.
func TestPromoteApproveWritesUncommittedAndMarksRule(t *testing.T) {
	repo := initPromoteRepo(t, map[string]string{
		"AGENTS.md": "# AGENTS.md — demo\n\nContract.\n",
	})
	headBefore := promoteGit(t, repo, "rev-parse", "HEAD")
	d, base, sessionID, owner := promoteDaemon(t, repo)
	obs := addPromoteObservation(t, d, owner, sessionID)
	rule := addPromoteRule(t, d, owner, []int64{obs})
	code, out := postPromotion(t, base, rule.RuleID, sessionID, "")
	if code != http.StatusCreated {
		t.Fatalf("propose = %d %v, want 201", code, out)
	}
	queue := getPromotionQueue(t, base)
	if len(queue) != 1 {
		t.Fatalf("queue = %d, want 1", len(queue))
	}
	code, approved := decidePromotion(t, base, queue[0].ID, map[string]any{"status": "approved"})
	if code != http.StatusOK {
		t.Fatalf("approve = %d %v, want 200", code, approved)
	}
	raw, err := os.ReadFile(filepath.Join(repo, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), rule.Text) {
		t.Fatalf("AGENTS.md = %q, want the promoted line", raw)
	}
	if headAfter := promoteGit(t, repo, "rev-parse", "HEAD"); headAfter != headBefore {
		t.Fatalf("HEAD moved %s -> %s, want the Daemon to leave the edit uncommitted", headBefore, headAfter)
	}
	if status := promoteGit(t, repo, "status", "--porcelain"); !strings.Contains(status, "AGENTS.md") {
		t.Fatalf("git status = %q, want the edit left dirty in the worktree", status)
	}
	bank, err := d.profileBankFor(owner)
	if err != nil {
		t.Fatal(err)
	}
	versions, err := bank.RuleVersions(context.Background(), rule.RuleID)
	if err != nil || len(versions) != 1 {
		t.Fatal(err)
	}
	got := versions[0]
	if got.PromotedBy != owner || got.PromotedAt.IsZero() {
		t.Fatalf("rule not marked promoted: %+v", got)
	}
	if got.Status != memory.StatusActive {
		t.Fatalf("status = %q, want active left untouched by the stamp", got.Status)
	}
	if len(got.SourceObservationIDs) != 1 || got.SourceObservationIDs[0] != obs || got.ApprovedBy != owner || got.Text != rule.Text {
		t.Fatalf("provenance changed: %+v", got)
	}
	items, err := bank.Queue(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if it.Kind == memory.QueuePromotion && it.Status == memory.QueueApproved {
			return
		}
	}
	t.Fatalf("no approved promotion item in %+v", items)
}

// A promotion approved through the normal rule-queue route performs the same
// write: the worktree file gains the line, the Rule is stamped, and the item
// is approved. A rejection through that route keeps the label and writes
// nothing.
func TestPromoteDecidesThroughRulesQueueRoute(t *testing.T) {
	repo := initPromoteRepo(t, map[string]string{"AGENTS.md": "# AGENTS.md\n"})
	d, base, sessionID, owner := promoteDaemon(t, repo)
	rule := addPromoteRule(t, d, owner, nil)
	if code, _ := postPromotion(t, base, rule.RuleID, sessionID, ""); code != http.StatusCreated {
		t.Fatalf("propose = %d, want 201", code)
	}
	qid := getPromotionQueue(t, base)[0].ID
	raw, _ := json.Marshal(map[string]any{"status": "approved"})
	resp, err := http.Post(base+"/api/rules/queue/"+strconv.FormatInt(qid, 10)+"/decide", "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("rules-route approve = %d, want 200", resp.StatusCode)
	}
	content, err := os.ReadFile(filepath.Join(repo, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), rule.Text) {
		t.Fatalf("AGENTS.md = %q, want the promoted line written through the rules route", content)
	}
	bank, err := d.profileBankFor(owner)
	if err != nil {
		t.Fatal(err)
	}
	versions, err := bank.RuleVersions(context.Background(), rule.RuleID)
	if err != nil || len(versions) != 1 || versions[0].PromotedBy != owner {
		t.Fatalf("rule not stamped through the rules route: %+v %v", versions, err)
	}
	// Rejection through the rules route keeps the label and writes nothing.
	rule2, err := bank.AddRule(context.Background(), memory.Rule{
		Text: "tabs for Go files", Kind: memory.KindPreference, ApprovedBy: owner,
	})
	if err != nil {
		t.Fatal(err)
	}
	if code, _ := postPromotion(t, base, rule2.RuleID, sessionID, ""); code != http.StatusCreated {
		t.Fatalf("second propose = %d, want 201", code)
	}
	var pending int64
	for _, it := range getPromotionQueue(t, base) {
		if it.Status == "pending" {
			pending = it.ID
		}
	}
	if pending == 0 {
		t.Fatalf("no pending promotion for the rejection half")
	}
	raw, _ = json.Marshal(map[string]any{"status": "rejected"})
	resp2, err := http.Post(base+"/api/rules/queue/"+strconv.FormatInt(pending, 10)+"/decide", "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("rules-route reject = %d, want 200", resp2.StatusCode)
	}
	if content, _ := os.ReadFile(filepath.Join(repo, "AGENTS.md")); strings.Contains(string(content), rule2.Text) {
		t.Fatalf("rejected promotion wrote the file through the rules route")
	}
}

// Approval is idempotent: approving twice never duplicates the AGENTS.md line.
func TestPromoteApproveIsIdempotent(t *testing.T) {
	repo := initPromoteRepo(t, map[string]string{"AGENTS.md": "# AGENTS.md\n"})
	d, base, sessionID, owner := promoteDaemon(t, repo)
	rule := addPromoteRule(t, d, owner, nil)
	if code, _ := postPromotion(t, base, rule.RuleID, sessionID, ""); code != http.StatusCreated {
		t.Fatalf("propose = %d, want 201", code)
	}
	qid := getPromotionQueue(t, base)[0].ID
	if code, _ := decidePromotion(t, base, qid, map[string]any{"status": "approved"}); code != http.StatusOK {
		t.Fatalf("first approve = %d, want 200", code)
	}
	// A second promotion for the same Rule proposes the already-applied line
	// with an empty diff and approves without duplicating it.
	if code, _ := postPromotion(t, base, rule.RuleID, sessionID, ""); code != http.StatusOK && code != http.StatusCreated {
		t.Fatalf("re-propose = %d, want 200 or 201", code)
	}
	raw, err := os.ReadFile(filepath.Join(repo, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(raw), rule.Text); n != 1 {
		t.Fatalf("AGENTS.md holds the line %d times, want exactly once", n)
	}
}

// A note target adds a frontmatter file under .emberloop/notes/ that parses
// and recalls like other project notes.
func TestPromoteNoteTargetAddsParseableFile(t *testing.T) {
	repo := initPromoteRepo(t, map[string]string{"AGENTS.md": "# AGENTS.md\n"})
	d, base, sessionID, owner := promoteDaemon(t, repo)
	rule := addPromoteRule(t, d, owner, nil)
	code, out := postPromotion(t, base, rule.RuleID, sessionID, "note")
	if code != http.StatusCreated {
		t.Fatalf("propose note = %d %v, want 201", code, out)
	}
	if out["target_path"] != ".emberloop/notes/"+strings.ToLower(rule.RuleID)+".md" {
		t.Fatalf("target_path = %v, want the note file", out["target_path"])
	}
	qid := getPromotionQueue(t, base)[0].ID
	if code, _ := decidePromotion(t, base, qid, map[string]any{"status": "approved"}); code != http.StatusOK {
		t.Fatalf("approve note = %d, want 200", code)
	}
	notePath := filepath.Join(repo, ".emberloop", "notes", strings.ToLower(rule.RuleID)+".md")
	raw, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatalf("note file missing: %v", err)
	}
	n, err := memory.ParseNote(notePath, raw)
	if err != nil {
		t.Fatalf("note does not parse: %v\n%s", err, raw)
	}
	if n.Text != rule.Text || n.Author != owner {
		t.Fatalf("note = %+v, want the rule text and author", n)
	}
	if status := promoteGit(t, repo, "status", "--porcelain"); !strings.Contains(status, ".emberloop") {
		t.Fatalf("git status = %q, want the note left uncommitted", status)
	}
}

// Rejection keeps the item as a label, writes nothing and leaves the Rule
// unmarked.
func TestPromoteRejectKeepsLabelAndWritesNothing(t *testing.T) {
	repo := initPromoteRepo(t, map[string]string{"AGENTS.md": "# AGENTS.md\n"})
	d, base, sessionID, owner := promoteDaemon(t, repo)
	rule := addPromoteRule(t, d, owner, nil)
	if code, _ := postPromotion(t, base, rule.RuleID, sessionID, ""); code != http.StatusCreated {
		t.Fatalf("propose = %d, want 201", code)
	}
	qid := getPromotionQueue(t, base)[0].ID
	if code, _ := decidePromotion(t, base, qid, map[string]any{"status": "rejected"}); code != http.StatusOK {
		t.Fatalf("reject = %d, want 200", code)
	}
	if raw, _ := os.ReadFile(filepath.Join(repo, "AGENTS.md")); strings.Contains(string(raw), rule.Text) {
		t.Fatalf("rejected promotion wrote the file")
	}
	bank, _ := d.profileBankFor(owner)
	got, _ := bank.RuleVersions(context.Background(), rule.RuleID)
	if !got[0].PromotedAt.IsZero() || got[0].PromotedBy != "" {
		t.Fatalf("rejected rule marked promoted: %+v", got[0])
	}
}

// Promotion needs a human: the machine token answers 401.
func TestPromoteRefusesMachineToken(t *testing.T) {
	_ = initPromoteRepo(t, map[string]string{"AGENTS.md": "# root\n"})
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()
	body, _ := json.Marshal(map[string]string{"rule_id": "r1", "session_id": "s1"})
	req, _ := http.NewRequest("POST", srv.URL+"/api/promotions", bytes.NewReader(body))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("machine promote = %d, want 401", resp.StatusCode)
	}
}

// Unknown rules and Sessions fail without queueing anything. A missing
// session with no provenance to resolve fails the same way.
func TestPromoteBadInputsQueueNothing(t *testing.T) {
	repo := initPromoteRepo(t, map[string]string{"AGENTS.md": "# root\n"})
	d, base, sessionID, owner := promoteDaemon(t, repo)
	if code, _ := postPromotion(t, base, "missing-rule", sessionID, ""); code != http.StatusNotFound {
		t.Fatalf("missing rule = %d, want 404", code)
	}
	if code, _ := postPromotion(t, base, "missing-rule", "missing-session", ""); code == http.StatusCreated {
		t.Fatalf("missing session queued a promotion")
	}
	if code, _ := postPromotion(t, base, "missing-rule", sessionID, "bogus"); code != http.StatusBadRequest {
		t.Fatalf("bad target = %d, want 400", code)
	}
	// A rule with no source sessions and no session_id names no worktree.
	rule := addPromoteRule(t, d, owner, nil)
	raw, _ := json.Marshal(map[string]string{"rule_id": rule.RuleID})
	resp, err := http.Post(base+"/api/promotions", "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	buf, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusNotFound || !strings.Contains(strings.ToLower(string(buf)), "session") {
		t.Fatalf("sourceless rule without session = %d %q, want 404 naming session_id", resp.StatusCode, buf)
	}
	if got := getPromotionQueue(t, base); len(got) != 0 {
		t.Fatalf("queue = %+v, want nothing queued for bad inputs", got)
	}
}

// Without a session_id the worktree resolves from the Rule's provenance: the
// newest source-observation Session still known to the Daemon.
func TestPromoteResolvesSessionFromProvenance(t *testing.T) {
	repo := initPromoteRepo(t, map[string]string{"AGENTS.md": "# root\n"})
	d, base, sessionID, owner := promoteDaemon(t, repo)
	obs := addPromoteObservation(t, d, owner, sessionID)
	rule := addPromoteRule(t, d, owner, []int64{obs})
	code, out := postPromotion(t, base, rule.RuleID, "", "")
	if code != http.StatusCreated {
		t.Fatalf("propose without session = %d %v, want 201 via provenance", code, out)
	}
	if out["target_path"] != "AGENTS.md" {
		t.Fatalf("target_path = %v, want the repo root AGENTS.md", out["target_path"])
	}
	if diff, _ := out["diff"].(string); !strings.Contains(diff, rule.Text) {
		t.Fatalf("diff = %q, want the rule text", diff)
	}
}
