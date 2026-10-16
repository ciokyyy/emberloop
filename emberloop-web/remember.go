package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"example.com/emberloop-web/memory"
)

// defaultRememberLimit is the per-Session cap on agent remember calls.
// EL_REMEMBER_LIMIT overrides it when it sets a positive number; tests use
// the override to refuse without proposing five Rules first.
const defaultRememberLimit = 5

// rememberCommandPrefix is the structural slash command that creates a Rule
// directly: /remember on the first line of a typed (sender=user) message.
// Pasted "remember:" text and tool output never match it.
const rememberCommandPrefix = "/remember"

// Sentinel errors for the remember tool's HTTP mapping. Handlers switch on
// these with errors.Is, so a reworded message never changes the status.
var (
	errRememberForbidden = errors.New("only top-level Sessions propose Rules")
	errRememberLimited   = errors.New("remember limit per session reached")
)

// rememberQueuePayload is the JSON of a rule queue item proposed through the
// remember tool: the proposed text plus who asked for it and why, with the
// grounding quote and Event. Origin marks remember proposals so the
// per-Session rate limit counts only them. SubjectKey groups the proposal
// with mined observations of the same subject for recurrence and
// supersession; it is normalized on write.
type rememberQueuePayload struct {
	Text       string `json:"text"`
	Reason     string `json:"reason"`
	Quote      string `json:"quote"`
	EventID    string `json:"eventId,omitempty"`
	SessionID  string `json:"sessionId,omitempty"`
	Author     string `json:"author,omitempty"`
	Origin     string `json:"origin,omitempty"`
	SubjectKey string `json:"subject_key,omitempty"`
	Kind       string `json:"kind,omitempty"`
}

// recallHit is one structured recall result: a User Rule, an observation or
// a Project note.
type recallHit struct {
	Source string `json:"source"`
	ID     string `json:"id"`
	Text   string `json:"text"`
	Flag   string `json:"flag,omitempty"`
}

// parseRememberCommand reports whether text carries a structural /remember
// action: the first line starts with /remember. It returns the Rule text
// after the prefix on that line and the remaining lines after it. Only the
// first line counts, so the same words inside a pasted block (a later line)
// or without the slash ("remember: ...") are not a command. Callers check
// the sender separately: only a typed sender=user message acts.
func parseRememberCommand(text string) (ruleText, rest string, isCommand bool) {
	first, after, _ := strings.Cut(text, "\n")
	line := strings.TrimSuffix(first, "\r")
	if line != rememberCommandPrefix && !strings.HasPrefix(line, rememberCommandPrefix+" ") && !strings.HasPrefix(line, rememberCommandPrefix+"\t") {
		return "", "", false
	}
	ruleText = strings.TrimSpace(strings.TrimPrefix(line, rememberCommandPrefix))
	return ruleText, after, true
}

// rememberLimit is the per-Session cap on agent remember calls in force.
func rememberLimit() int {
	if v, err := strconv.Atoi(strings.TrimSpace(os.Getenv("EL_REMEMBER_LIMIT"))); err == nil && v > 0 {
		return v
	}
	return defaultRememberLimit
}

// rememberAllowed reports whether the Session may propose Rules through the
// remember tool. Only top-level Sessions propose: scouts, spawned workers,
// Run Sessions and Reviewers get recall only.
func rememberAllowed(meta SessionMeta) bool {
	if meta.Scout || meta.RunWorker || meta.GroupID != "" || isReviewerSession(meta) {
		return false
	}
	return elRoleFor(meta) == "web"
}

// groundingEvent finds the newest sender=user prompt Event of the Session
// whose text holds quote, for the remember grounding check. It returns the
// Event's sequence number, its sender's User ID and its text. The sender
// matters: a quote from User B's steer inside A's Session grounds in B's
// bank, never A's. Callers hold no lock.
func (d *Daemon) groundingEvent(sessionID, quote string) (seq int64, user, text string, found bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	s, ok := d.sessions[sessionID]
	if !ok {
		return 0, "", "", false
	}
	for i := len(s.events) - 1; i >= 0; i-- {
		e := s.events[i]
		if e.Kind != "prompt" || e.Sender != senderUser {
			continue
		}
		if quote != "" && strings.Contains(e.Text, quote) {
			return e.Seq, e.User, e.Text, true
		}
	}
	return 0, "", "", false
}

// rememberCount counts the remember proposals of one Session in the bank.
// Only origin=remember items count, so future miner candidates never consume
// the agent limit.
func rememberCount(ctx context.Context, bank memory.Memory, sessionID string) (int, error) {
	items, err := bank.Queue(ctx)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, it := range items {
		if it.Kind != memory.QueueRule {
			continue
		}
		var p rememberQueuePayload
		if err := json.Unmarshal([]byte(it.Payload), &p); err != nil {
			continue
		}
		if p.Origin == "remember" && p.SessionID == sessionID {
			n++
		}
	}
	return n, nil
}

// proposeRemember grounds the quote in a User message of the Session, checks
// the per-Session limit, and enqueues a Rule candidate. It never writes a
// Rule. The candidate lands in the bank of the grounded Event's sender:
// usually the Session owner, but a quote from User B's steer inside A's
// Session lands in B's bank, never A's. Author records who asked ("" for
// agent calls over the machine token).
//
// The limit check and the enqueue hold d.rememberMu, so concurrent agent
// calls in one Session cannot pass the limit together and exceed it.
func (d *Daemon) proposeRemember(ctx context.Context, sessionID, text, reason, quote, author string) (int64, error) {
	d.mu.Lock()
	s, ok := d.sessions[sessionID]
	meta := SessionMeta{}
	if ok {
		meta = s.meta
	}
	d.mu.Unlock()
	if !ok {
		return 0, fmt.Errorf("%w: session %s", memory.ErrNotFound, sessionID)
	}
	if meta.Owner == "" {
		return 0, fmt.Errorf("%w: session %s", memory.ErrNotFound, sessionID)
	}
	if !rememberAllowed(meta) {
		return 0, errRememberForbidden
	}
	text = strings.TrimSpace(text)
	reason = strings.TrimSpace(reason)
	quote = strings.TrimSpace(quote)
	if text == "" {
		return 0, fmt.Errorf("%w: remember text is required", memory.ErrInvalid)
	}
	if reason == "" {
		return 0, fmt.Errorf("%w: remember reason is required", memory.ErrInvalid)
	}
	if utf8.RuneCountInString(text) > memory.MaxRuleChars {
		return 0, fmt.Errorf("%w: rule text %d chars, max %d", memory.ErrInvalid, utf8.RuneCountInString(text), memory.MaxRuleChars)
	}
	if quote == "" {
		quote = text
	}
	seq, eventUser, eventText, found := d.groundingEvent(sessionID, quote)
	if !found {
		return 0, fmt.Errorf("%w: quote is not a substring of any user message in this session", memory.ErrInvalid)
	}
	if err := memory.CheckGrounding(strconv.FormatInt(seq, 10), quote, memory.GroundEvent{Found: true, Sender: senderUser, Text: eventText}); err != nil {
		return 0, err
	}
	bankOwner := eventUser
	if bankOwner == "" {
		bankOwner = meta.Owner
	}
	d.rememberMu.Lock()
	defer d.rememberMu.Unlock()
	bank, err := d.profileBankFor(bankOwner)
	if err != nil {
		return 0, err
	}
	if n, err := rememberCount(ctx, bank, sessionID); err != nil {
		return 0, err
	} else if n >= rememberLimit() {
		return 0, fmt.Errorf("%w (%d per session)", errRememberLimited, rememberLimit())
	}
	raw, _ := json.Marshal(rememberQueuePayload{
		Text: memory.Redact(text), Reason: reason, Quote: memory.Redact(quote),
		EventID: strconv.FormatInt(seq, 10), SessionID: sessionID, Author: author, Origin: "remember",
		SubjectKey: memory.NormalizeSubjectKey(memory.ProposeSubjectKey(text)),
		Kind:       memory.KindPreference,
	})
	return bank.Enqueue(ctx, memory.QueueItem{Kind: memory.QueueRule, Payload: string(raw)})
}

// RememberFromSession is the pig in-process propose path: the same rules as
// the HTTP remember route, without the HTTP round-trip.
func (d *Daemon) RememberFromSession(sessionID, text, reason, quote string) (int64, error) {
	return d.proposeRemember(context.Background(), sessionID, text, reason, quote, "")
}

// handleSessionRemember serves the agent propose path:
// POST /api/sessions/{id}/remember {"text","reason","quote"?}. It always
// creates a queue item, never a Rule. Only the web role (top-level Sessions)
// may propose; scouts, workers, Reviewers and Run Sessions are refused, as is
// anything past the per-Session limit.
func (d *Daemon) handleSessionRemember(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	d.mu.Lock()
	_, ok := d.sessions[id]
	d.mu.Unlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	var body struct {
		Text   string `json:"text"`
		Reason string `json:"reason"`
		Quote  string `json:"quote"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	var author string
	if who, ok := d.requesterOf(r); ok && who.user != nil {
		author = who.user.ID
	}
	qid, err := d.proposeRemember(r.Context(), id, body.Text, body.Reason, body.Quote, author)
	if err != nil {
		switch {
		case errors.Is(err, errRememberForbidden):
			http.Error(w, err.Error(), http.StatusForbidden)
		case errors.Is(err, errRememberLimited):
			http.Error(w, err.Error(), http.StatusTooManyRequests)
		default:
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": qid})
}

// recallText searches the Session owner's bank and the worktree's Project
// notes for query and renders the fused hits, best match first. It never
// returns another User's bank: the bank is always the owner's. A notes or
// verification failure still returns the bank hits; only a bank failure
// errors.
func (d *Daemon) recallText(ctx context.Context, sessionID, query string) (string, []recallHit, error) {
	d.mu.Lock()
	s, ok := d.sessions[sessionID]
	meta := SessionMeta{}
	if ok {
		meta = s.meta
	}
	d.mu.Unlock()
	if !ok {
		return "", nil, fmt.Errorf("%w: session %s", memory.ErrNotFound, sessionID)
	}
	if meta.Owner == "" {
		return "", nil, fmt.Errorf("%w: session %s", memory.ErrNotFound, sessionID)
	}
	if strings.TrimSpace(query) == "" {
		return "", nil, fmt.Errorf("%w: recall query is required", memory.ErrInvalid)
	}
	bank, err := d.profileBankFor(meta.Owner)
	if err != nil {
		return "", nil, err
	}
	rules, err := bank.Recall(ctx, memory.Redact(query), memory.MaxRecall)
	if err != nil {
		return "", nil, err
	}
	verified := memory.VerifyRules(ctx, meta.WorkDir, rules)
	var liveRules []memory.VerifiedRule
	for _, v := range verified {
		if v.Result.Status == memory.VerifyDormant {
			continue
		}
		liveRules = append(liveRules, v)
	}
	var liveNotes []memory.VerifiedNote
	if notes, _, err := memory.LoadNotes(meta.WorkDir); err == nil && len(notes) > 0 {
		if idx, err := memory.OpenNotesIndex(notes); err == nil {
			defer idx.Close()
			liveNotes, _ = memory.RecallNotes(ctx, meta.WorkDir, idx, query, memory.MaxRecall)
		}
	}
	hits := memory.FuseRecall(liveRules, liveNotes)
	if len(hits) > 10 {
		hits = hits[:10]
	}
	obs, obsErr := bank.SearchObservations(ctx, memory.Redact(query), 10)
	memories, memErr := bank.RecallMemories(ctx, memory.Redact(query), d.memoryRecallWeights(), 10)
	if memErr != nil {
		memories = nil
	}
	episodes, episodeErr := bank.RecallEpisodes(ctx, memory.Redact(query), d.memoryRecallWeights(), 10)
	if episodeErr != nil {
		episodes = nil
	}
	text, results := fuseRecallToolResults(hits, obs, obsErr, memories, episodes)
	return text, results, nil
}

// fuseRecallToolResults combines each independently ranked source by RRF.
// It never compares BM25 values across the bank, notes, or Episode indexes.
func fuseRecallToolResults(rulesNotes []memory.MergedHit, observations []memory.Observation, obsErr error, memories []memory.MemoryHit, episodes []memory.EpisodeHit) (string, []recallHit) {
	type item struct {
		hit   recallHit
		line  string
		score float64
	}
	var ranked []item
	add := func(hit recallHit, line string, rank int) {
		ranked = append(ranked, item{hit, line, 1 / (float64(memory.RRFK) + float64(rank))})
	}
	for i, h := range rulesNotes {
		text, hits, _ := formatRecallResults([]memory.MergedHit{h}, nil, nil)
		if len(hits) > 0 {
			add(hits[0], text, i+1)
		}
	}
	if obsErr == nil {
		for i, o := range observations {
			add(recallHit{Source: "observation", ID: strconv.FormatInt(o.ID, 10), Text: o.Quote}, "observation: "+o.Quote, i+1)
		}
	}
	for i, m := range memories {
		add(recallHit{Source: "memory", ID: strconv.FormatInt(m.Memory.ID, 10), Text: m.Memory.Text}, "memory: "+m.Memory.Text, i+1)
	}
	for i, e := range episodes {
		add(recallHit{Source: "episode", ID: strconv.FormatInt(e.Episode.ID, 10), Text: e.Episode.Text}, "episode: "+e.Episode.Text, i+1)
	}
	sort.SliceStable(ranked, func(i, j int) bool { return ranked[i].score > ranked[j].score })
	if len(ranked) > 10 {
		ranked = ranked[:10]
	}
	results := make([]recallHit, 0, len(ranked))
	lines := make([]string, 0, len(ranked))
	for _, r := range ranked {
		results = append(results, r.hit)
		lines = append(lines, r.line)
	}
	return strings.Join(lines, "\n"), results
}

// formatRecallHit renders one fused hit as its structured result and its text
// line, with the verify flag in parentheses when the anchored code changed.
func formatRecallHit(source, id, text, flag string) (recallHit, string) {
	display := "rule: " + text
	if source == "note" {
		display = "note " + id + ": " + text
	}
	line := display
	if flag != "" {
		line += " (" + flag + ")"
	}
	return recallHit{Source: source, ID: id, Text: text, Flag: flag}, line
}

// formatRecallResults renders fused Rule/note hits plus observation hits.
// Observations are candidates, never injected, but searchable through recall
// like Rules. An observation search failure still returns the Rule and note
// hits; only a bank failure (which never reaches here) errors.
func formatRecallResults(hits []memory.MergedHit, obs []memory.Observation, obsErr error) (string, []recallHit, error) {
	var out []recallHit
	var lines []string
	for _, h := range hits {
		switch h.Source {
		case memory.MergeNote:
			flag := ""
			if h.NoteResult.NeedsFlag() {
				flag = h.NoteResult.FlagNote()
			}
			hit, line := formatRecallHit("note", h.Note.ID, h.Note.Text, flag)
			out, lines = append(out, hit), append(lines, line)
		default:
			flag := ""
			if h.RuleResult.NeedsFlag() {
				flag = h.RuleResult.FlagNote()
			}
			hit, line := formatRecallHit("rule", h.Rule.RuleID, h.Rule.Text, flag)
			out, lines = append(out, hit), append(lines, line)
		}
	}
	if obsErr == nil {
		for _, o := range obs {
			out = append(out, recallHit{Source: "observation", ID: strconv.FormatInt(o.ID, 10), Text: o.Quote})
			lines = append(lines, "observation: "+o.Quote)
		}
	}
	return strings.Join(lines, "\n"), out, nil
}

// RecallForSession is the pig in-process search path: the Session owner's
// bank plus Project notes, without the HTTP round-trip.
func (d *Daemon) RecallForSession(sessionID, query string) (string, error) {
	text, _, err := d.recallText(context.Background(), sessionID, query)
	return text, err
}

// handleSessionRecall serves the agent search path:
// GET /api/sessions/{id}/recall?query=.... Every role may search, including
// scouts, workers, Reviewers and Run Sessions; the bank is always the Session
// owner's, so no other User's Rules or observations ever show.
func (d *Daemon) handleSessionRecall(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != "GET" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	d.mu.Lock()
	_, ok := d.sessions[id]
	d.mu.Unlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	text, hits, err := d.recallText(r.Context(), id, r.URL.Query().Get("query"))
	if err != nil {
		switch {
		case errors.Is(err, memory.ErrNotFound):
			http.Error(w, err.Error(), http.StatusNotFound)
		case errors.Is(err, memory.ErrInvalid):
			http.Error(w, err.Error(), http.StatusBadRequest)
		default:
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	if hits == nil {
		hits = []recallHit{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": hits, "text": text})
}

// gitHead returns the HEAD commit of the worktree at workDir, or "" when the
// directory is not a git checkout. It gives a direct /remember Rule its
// observed commit for provenance without failing the action.
func gitHead(workDir string) string {
	workDir = strings.TrimSpace(workDir)
	if workDir == "" {
		return ""
	}
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = workDir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// applyRememberCommand creates a Rule directly from a structural /remember
// action on a typed sender=user message. The bank is always the sender's
// (the Event sender, not the Session owner), per the private-by-default rule;
// a steer from User B inside A's Session lands in B's bank. The quote is the
// Rule text itself, which is a substring of the message by construction, so
// the action is self-grounding. It returns the new Rule ID.
//
// The Rule kind is fact, the neutral default: the author stated the text as
// something to keep, and no signal picks a narrower kind. The approval queue
// (ticket 22) may reclassify candidates it mines.
func (d *Daemon) applyRememberCommand(sessionID, senderID, fullText, ruleText string, eventSeq int64) (string, error) {
	ruleText = strings.TrimSpace(ruleText)
	if ruleText == "" {
		return "", fmt.Errorf("%w: remember text is required", memory.ErrInvalid)
	}
	if utf8.RuneCountInString(ruleText) > memory.MaxRuleChars {
		return "", fmt.Errorf("%w: rule text %d chars, max %d", memory.ErrInvalid, utf8.RuneCountInString(ruleText), memory.MaxRuleChars)
	}
	if err := memory.CheckGrounding(strconv.FormatInt(eventSeq, 10), ruleText, memory.GroundEvent{Found: true, Sender: senderUser, Text: fullText}); err != nil {
		return "", err
	}
	bank, err := d.profileBankFor(senderID)
	if err != nil {
		return "", err
	}
	d.mu.Lock()
	var workDir string
	if s, ok := d.sessions[sessionID]; ok {
		workDir = s.meta.WorkDir
	}
	d.mu.Unlock()
	rule, err := bank.AddRule(context.Background(), memory.Rule{
		Text: memory.Redact(ruleText), Kind: memory.KindFact, Status: memory.StatusActive,
		ObservedCommit: gitHead(workDir), ApprovedBy: senderID,
	})
	if err != nil {
		return "", err
	}
	return rule.RuleID, nil
}

// rememberCommandResult carries a handled /remember action: the stored prompt
// Event, the new Rule, and the leftover message that still routes normally.
type rememberCommandResult struct {
	event  Event
	ruleID string
	rest   string
}

// validateRememberText refuses an empty or over-long /remember text before
// anything is logged, so a rejected command leaves no prompt Event behind.
func validateRememberText(ruleText string) error {
	ruleText = strings.TrimSpace(ruleText)
	if ruleText == "" {
		return fmt.Errorf("%w: remember text is required", memory.ErrInvalid)
	}
	if utf8.RuneCountInString(ruleText) > memory.MaxRuleChars {
		return fmt.Errorf("%w: rule text %d chars, max %d", memory.ErrInvalid, utf8.RuneCountInString(ruleText), memory.MaxRuleChars)
	}
	return nil
}

// handleRememberCommand stores the typed message as its prompt Event and
// creates its Rule directly. Callers checked the sender (user) and the first
// line (the /remember prefix) already, and validated the text, so the log
// holds exactly the commands that act. The full text stays in the log as the
// grounding Event; only rest routes on as the turn, so the agent never sees
// the slash line.
func (d *Daemon) handleRememberCommand(id, mode, text string, images []Image, imageIDs []string, from promptSender) (rememberCommandResult, int, string) {
	ruleText, rest, _ := parseRememberCommand(text)
	if err := validateRememberText(ruleText); err != nil {
		return rememberCommandResult{}, http.StatusBadRequest, err.Error()
	}
	e := d.appendPromptEvent(id, text, modeLabel(mode), len(images), imageIDs, from)
	ruleID, err := d.applyRememberCommand(id, from.user, text, ruleText, e.Seq)
	if err != nil {
		return rememberCommandResult{}, http.StatusBadRequest, err.Error()
	}
	return rememberCommandResult{event: e, ruleID: ruleID, rest: strings.TrimSpace(rest)}, 0, ""
}
