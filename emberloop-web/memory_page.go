package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"example.com/emberloop-web/memory"
)

// Memory page endpoints are scoped to the signed-in User's bank. Memories are
// newest-first and filterable; action routes soft-forget, park, pin, propose a
// pending Rule, or create a superseding version. Legacy observations can be
// promoted and Episodes are read-only. Purge remains the only deletion path.

// ruleView is one Rule on the Memory page. Keys are lowercase for the UI;
// the engine structs keep their Go names. Times are always set by the bank,
// so they carry no omitempty: encoding/json never omits a struct value.
type ruleView struct {
	RuleID               string    `json:"ruleId"`
	Version              int       `json:"version"`
	Text                 string    `json:"text"`
	Kind                 string    `json:"kind"`
	SubjectKey           string    `json:"subjectKey"`
	Status               string    `json:"status"`
	Anchors              []string  `json:"anchors,omitempty"`
	ObservedCommit       string    `json:"observedCommit,omitempty"`
	ApprovedBy           string    `json:"approvedBy,omitempty"`
	ApprovedAt           time.Time `json:"approvedAt"`
	SourceObservationIDs []int64   `json:"sourceObservationIds,omitempty"`
}

func toRuleView(r memory.Rule) ruleView {
	return ruleView{
		RuleID:               r.RuleID,
		Version:              r.Version,
		Text:                 r.Text,
		Kind:                 r.Kind,
		SubjectKey:           r.SubjectKey,
		Status:               r.Status,
		Anchors:              append([]string(nil), r.Anchors...),
		ObservedCommit:       r.ObservedCommit,
		ApprovedBy:           r.ApprovedBy,
		ApprovedAt:           r.ApprovedAt,
		SourceObservationIDs: append([]int64(nil), r.SourceObservationIDs...),
	}
}

// observationView is one observation on the Memory page.
type observationView struct {
	ID             int64     `json:"id"`
	SessionID      string    `json:"sessionId"`
	EventID        string    `json:"eventId"`
	Author         string    `json:"author,omitempty"`
	Quote          string    `json:"quote"`
	Kind           string    `json:"kind"`
	SubjectKey     string    `json:"subjectKey"`
	EvidenceCount  int       `json:"evidenceCount"`
	ObservedCommit string    `json:"observedCommit,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
}

func toObservationView(o memory.Observation) observationView {
	return observationView{
		ID:             o.ID,
		SessionID:      o.SessionID,
		EventID:        o.EventID,
		Author:         o.Author,
		Quote:          o.Quote,
		Kind:           o.Kind,
		SubjectKey:     o.SubjectKey,
		EvidenceCount:  o.EvidenceCount,
		ObservedCommit: o.ObservedCommit,
		CreatedAt:      o.CreatedAt,
	}
}

// ledgerView is one ledger row on the Memory page.
type ledgerView struct {
	ID         int64     `json:"id"`
	RunID      string    `json:"runId"`
	Stage      string    `json:"stage"`
	Status     string    `json:"status"`
	CountsJSON string    `json:"countsJson"`
	Detail     string    `json:"detail"`
	CreatedAt  time.Time `json:"createdAt"`
}

func toLedgerView(l memory.LedgerRow) ledgerView {
	return ledgerView{
		ID:         l.ID,
		RunID:      l.RunID,
		Stage:      l.Stage,
		Status:     l.Status,
		CountsJSON: l.CountsJSON,
		Detail:     l.Detail,
		CreatedAt:  l.CreatedAt,
	}
}

// memoryStatus is GET /api/memory/status: the chip plus the stats. LedgerSize
// has no omitempty so an empty ledger sends 0 instead of dropping the field.
type memoryMetrics struct {
	MemoriesPerDay   int     `json:"memoriesPerDay"`
	Forgotten7Day    float64 `json:"forgotten7Day"`
	PromotedShare    float64 `json:"promotedShare"`
	InjectionHitRate float64 `json:"injectionHitRate"`
	Contradictions   int     `json:"contradictions"`
	ExtractorCostUSD float64 `json:"extractorCostUsd"`
	EmbedderCostUSD  float64 `json:"embedderCostUsd"`
	CostAlert        bool    `json:"costAlert"`
}

type memoryStatus struct {
	Available   bool               `json:"available"`
	Queue       *memory.QueueStats `json:"queue,omitempty"`
	LedgerSize  int                `json:"ledgerSize"`
	LastLedger  *ledgerView        `json:"lastLedger,omitempty"`
	LastNightly *ledgerView        `json:"lastNightly,omitempty"`
	Metrics     memoryMetrics      `json:"metrics"`
	Error       string             `json:"error,omitempty"`
}

// purgeRequest is POST /api/memory/purge. Selection IDs name the records the
// human chose on the page; Confirm must be the typed word PURGE.
type purgeRequest struct {
	Observations []int64  `json:"observations"`
	Memories     []int64  `json:"memories"`
	Episodes     []int64  `json:"episodes"`
	Rules        []string `json:"rules"`
	Queue        []int64  `json:"queue"`
	Ledger       []int64  `json:"ledger"`
	Confirm      string   `json:"confirm"`
}

// purgeResultView reports what the purge removed, in UI keys.
type purgeResultView struct {
	Observations int `json:"observations"`
	Memories     int `json:"memories"`
	Episodes     int `json:"episodes"`
	Rules        int `json:"rules"`
	Queue        int `json:"queue"`
	Ledger       int `json:"ledger"`
	Scrubbed     int `json:"scrubbed"`
	Snapshots    int `json:"snapshots"`
}

// memoryUserOf resolves the signed-in User behind a Memory page call. The
// machine token is no User: it answers 401 like /api/users.
func (d *Daemon) memoryUserOf(w http.ResponseWriter, r *http.Request) (User, bool) {
	who, ok := d.requesterOf(r)
	if !ok || who.user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return User{}, false
	}
	return *who.user, true
}

// memoryBank resolves the signed-in User behind r, enforces method, and opens
// the caller's bank. Authentication and method failures are answered here. A
// bank-open failure is answered with failCode: 500 on the list, ledger and
// purge routes, 503 on the status route (plain text; the page treats any
// status-route failure as the Unavailable chip).
func (d *Daemon) memoryBank(w http.ResponseWriter, r *http.Request, method string, failCode int) (memory.Memory, bool) {
	user, ok := d.memoryUserOf(w, r)
	if !ok {
		return nil, false
	}
	if r.Method != method {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return nil, false
	}
	bank, err := d.profileBankFor(user.ID)
	if err != nil {
		http.Error(w, err.Error(), failCode)
		return nil, false
	}
	return bank, true
}

// memoryQuery reads the page search text: q first, query as fallback, trimmed.
func memoryQuery(r *http.Request) string {
	if q := strings.TrimSpace(r.URL.Query().Get("q")); q != "" {
		return q
	}
	return strings.TrimSpace(r.URL.Query().Get("query"))
}

// memoryLimit parses the list limit: empty means the default, garbage means
// the default, and the engine clamps the rest.
func memoryLimit(r *http.Request, def int) int {
	raw := strings.TrimSpace(r.URL.Query().Get("limit"))
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

// handleRulesList serves GET /api/rules: the User's Rules, live versions
// only. An empty q lists the newest first; a q searches through Recall. The
// bank is always the caller's, so no other User's Rules ever show.
func (d *Daemon) handleRulesList(w http.ResponseWriter, r *http.Request) {
	bank, ok := d.memoryBank(w, r, http.MethodGet, http.StatusInternalServerError)
	if !ok {
		return
	}
	var rules []memory.Rule
	var err error
	if q := memoryQuery(r); q == "" {
		rules, err = bank.ListRules(r.Context(), memoryLimit(r, 50))
	} else {
		rules, err = bank.Recall(r.Context(), memory.Redact(q), memoryLimit(r, memory.MaxRecall))
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out := make([]ruleView, 0, len(rules))
	for _, rl := range rules {
		out = append(out, toRuleView(rl))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleObservationsList serves GET /api/observations: the User's grounded
// candidates, never injected. An empty q lists the newest first; a q searches
// through SearchObservations. The bank is always the caller's.
func (d *Daemon) handleObservationsList(w http.ResponseWriter, r *http.Request) {
	bank, ok := d.memoryBank(w, r, http.MethodGet, http.StatusInternalServerError)
	if !ok {
		return
	}
	var obs []memory.Observation
	var err error
	if q := memoryQuery(r); q == "" {
		obs, err = bank.ListObservations(r.Context(), memoryLimit(r, 50))
	} else {
		obs, err = bank.SearchObservations(r.Context(), memory.Redact(q), memoryLimit(r, memory.MaxRecall))
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out := make([]observationView, 0, len(obs))
	for _, o := range obs {
		out = append(out, toObservationView(o))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleMemoryLedger serves GET /api/memory/ledger: the User's ledger rows,
// oldest first. Queue mutations and nightly stages share it; the page splits
// the last nightly run from the tail.
func (d *Daemon) handleMemoryLedger(w http.ResponseWriter, r *http.Request) {
	bank, ok := d.memoryBank(w, r, http.MethodGet, http.StatusInternalServerError)
	if !ok {
		return
	}
	rows, err := bank.Ledger(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out := make([]ledgerView, 0, len(rows))
	for _, l := range rows {
		out = append(out, toLedgerView(l))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleMemoryStatus serves GET /api/memory/status: the engine chip and the
// stats. Queue gives size and approval latency; ledger gives the last row and
// the last nightly run (the newest row whose run is not the queue's). Any
// engine or bank failure answers 503 with available false, which the page
// shows as the Unavailable chip and never blocks the turn.
func summarizeMemoryMetrics(rows []memory.LedgerRow, now time.Time) memoryMetrics {
	var m memoryMetrics
	var stored, stored7d, forgotten7d, promoted, injections, creates int
	for _, row := range rows {
		counts := map[string]float64{}
		_ = json.Unmarshal([]byte(row.CountsJSON), &counts)
		switch row.Stage {
		case "ingest":
			stored += int(counts["stored"])
			if row.CreatedAt.After(now.Add(-7 * 24 * time.Hour)) {
				stored7d += int(counts["stored"])
			}
			if row.CreatedAt.After(now.Add(-24 * time.Hour)) {
				m.MemoriesPerDay += int(counts["stored"])
				m.ExtractorCostUSD += ledgerCost(row.Detail, "cost_usd")
			}
		case "memory_forgotten":
			if row.CreatedAt.After(now.Add(-7 * 24 * time.Hour)) {
				forgotten7d += int(counts["count"])
			}
		case "memory_promoted":
			promoted += int(counts["count"])
		case "injection":
			injections += int(counts["memories"])
			creates += int(counts["creates"])
		case "memory_contradicted":
			m.Contradictions += int(counts["count"])
		case "embedding":
			if row.CreatedAt.After(now.Add(-24 * time.Hour)) {
				m.EmbedderCostUSD += ledgerCost(row.Detail, "cost_usd")
			}
		}
	}
	if stored > 0 {
		m.PromotedShare = float64(promoted) / float64(stored)
	}
	if stored7d > 0 {
		m.Forgotten7Day = float64(forgotten7d) / float64(stored7d)
	}
	if creates > 0 {
		m.InjectionHitRate = float64(injections) / float64(creates)
	}
	m.CostAlert = m.ExtractorCostUSD+m.EmbedderCostUSD > .05
	return m
}

func ledgerCost(detail, key string) float64 {
	for _, part := range strings.Fields(detail) {
		if k, v, ok := strings.Cut(part, "="); ok && k == key {
			n, _ := strconv.ParseFloat(v, 64)
			return n
		}
	}
	return 0
}

func (d *Daemon) handleMemoryStatus(w http.ResponseWriter, r *http.Request) {
	bank, ok := d.memoryBank(w, r, http.MethodGet, http.StatusServiceUnavailable)
	if !ok {
		return
	}
	items, err := bank.Queue(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, memoryStatus{Available: false, Error: err.Error()})
		return
	}
	rows, err := bank.Ledger(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, memoryStatus{Available: false, Error: err.Error()})
		return
	}
	st := memory.DescribeQueue(items, d.now())
	out := memoryStatus{Available: true, Queue: &st, LedgerSize: len(rows)}
	out.Metrics = summarizeMemoryMetrics(rows, d.now())
	if len(rows) > 0 {
		last := toLedgerView(rows[len(rows)-1])
		out.LastLedger = &last
		for i := len(rows) - 1; i >= 0; i-- {
			if rows[i].RunID != queueLedgerRunID {
				nightly := toLedgerView(rows[i])
				out.LastNightly = &nightly
				break
			}
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// handleMemoryPurge serves POST /api/memory/purge: the human purge. It
// deletes the chosen records in the caller's bank and reports what it
// removed. The typed confirmation (confirm PURGE) is required: without it the
// purge is refused and nothing is deleted. An empty selection is refused the
// same way, so a stray POST never vacuums the bank.
func (d *Daemon) handleMemoryPurge(w http.ResponseWriter, r *http.Request) {
	bank, ok := d.memoryBank(w, r, http.MethodPost, http.StatusInternalServerError)
	if !ok {
		return
	}
	var body purgeRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(body.Confirm) != "PURGE" {
		http.Error(w, `type PURGE to confirm`, http.StatusBadRequest)
		return
	}
	sel := memory.Selection{
		Observations: body.Observations,
		Memories:     body.Memories,
		Episodes:     body.Episodes,
		Rules:        body.Rules,
		Queue:        body.Queue,
		Ledger:       body.Ledger,
	}
	if len(sel.Observations)+len(sel.Memories)+len(sel.Episodes)+len(sel.Rules)+len(sel.Queue)+len(sel.Ledger) == 0 {
		http.Error(w, "nothing selected", http.StatusBadRequest)
		return
	}
	res, err := bank.Purge(r.Context(), sel)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, purgeResultView{
		Observations: res.Observations,
		Memories:     res.Memories,
		Episodes:     res.Episodes,
		Rules:        res.Rules,
		Queue:        res.Queue,
		Ledger:       res.Ledger,
		Scrubbed:     res.Scrubbed,
		Snapshots:    res.Snapshots,
	})
}

func (d *Daemon) handleMemoryCount(w http.ResponseWriter, r *http.Request) {
	bank, ok := d.memoryBank(w, r, http.MethodGet, http.StatusInternalServerError)
	if !ok {
		return
	}
	var since time.Time
	if raw := strings.TrimSpace(r.URL.Query().Get("since")); raw != "" {
		ms, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || ms < 0 {
			http.Error(w, "invalid since", http.StatusBadRequest)
			return
		}
		since = time.UnixMilli(ms).UTC()
	}
	count, err := bank.CountMemoriesSince(r.Context(), since)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"count": count})
}

type memoryItemView struct {
	ID           int64     `json:"id"`
	SessionID    string    `json:"sessionId"`
	EventID      string    `json:"eventId"`
	Author       string    `json:"author"`
	Kind         string    `json:"kind"`
	Text         string    `json:"text"`
	Quote        string    `json:"quote"`
	Importance   float64   `json:"importance"`
	Veracity     string    `json:"veracity"`
	MentionCount int       `json:"mentionCount"`
	Status       string    `json:"status"`
	SupersededBy int64     `json:"supersededBy,omitempty"`
	Pinned       bool      `json:"pinned"`
	CreatedAt    time.Time `json:"createdAt"`
}

func toMemoryItemView(m memory.MemoryItem) memoryItemView {
	return memoryItemView{m.ID, m.SessionID, m.EventID, m.Author, m.Kind, m.Text, m.Quote, m.Importance, m.Veracity, m.MentionCount, m.Status, m.SupersededBy, m.Pinned != 0, m.CreatedAt}
}

func (d *Daemon) handleMemories(w http.ResponseWriter, r *http.Request) {
	bank, ok := d.memoryBank(w, r, http.MethodGet, http.StatusInternalServerError)
	if !ok {
		return
	}
	rows, err := bank.SearchMemories(r.Context(), memory.Redact(memoryQuery(r)), r.URL.Query().Get("kind"), r.URL.Query().Get("status"), memoryLimit(r, 50))
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	out := make([]memoryItemView, 0, len(rows))
	for _, m := range rows {
		out = append(out, toMemoryItemView(m))
	}
	writeJSON(w, 200, out)
}

func (d *Daemon) handleMemoryAction(w http.ResponseWriter, r *http.Request) {
	bank, ok := d.memoryBank(w, r, http.MethodPost, http.StatusInternalServerError)
	if !ok {
		return
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/memory/memories/"), "/"), "/")
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid memory id", 400)
		return
	}
	var body struct {
		Pinned *bool  `json:"pinned"`
		Text   string `json:"text"`
	}
	if err = json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad json", 400)
		return
	}
	switch parts[1] {
	case "forget":
		err = bank.SetMemoryStatus(r.Context(), id, memory.StatusForgotten)
	case "park":
		err = bank.SetMemoryStatus(r.Context(), id, memory.StatusParked)
	case "pin":
		if body.Pinned == nil {
			http.Error(w, "pinned is required", 400)
			return
		}
		err = bank.SetMemoryPinned(r.Context(), id, *body.Pinned)
	case "promote":
		m, e := bank.GetMemory(r.Context(), id)
		if e != nil {
			err = e
			break
		}
		payload := memory.EncodeRuleProposal(memory.RuleProposal{
			Text: m.Text, Kind: m.Kind, SubjectKey: memory.NormalizeSubjectKey(memory.ProposeSubjectKey(m.Text)),
			Quotes: []string{m.Quote}, Sessions: []string{m.SessionID}, Author: m.Author,
			Origin: "memory", Reason: "Promoted from Memory #" + strconv.FormatInt(m.ID, 10) + " (importance " + strconv.FormatFloat(m.Importance, 'f', -1, 64) + ")",
			SourceMemoryIDs: []int64{m.ID}, Importance: m.Importance,
		})
		_, err = bank.Enqueue(r.Context(), memory.QueueItem{Kind: memory.QueueRule, Payload: payload, Status: memory.QueuePending})
	case "edit":
		m, e := bank.GetMemory(r.Context(), id)
		if e != nil {
			err = e
			break
		}
		if strings.TrimSpace(body.Text) == "" {
			http.Error(w, "text is required", 400)
			return
		}
		m.Text = strings.TrimSpace(body.Text)
		_, err = bank.ReviseMemory(r.Context(), id, m)
	default:
		http.NotFound(w, r)
		return
	}
	if err != nil {
		code := 500
		if errors.Is(err, memory.ErrNotFound) {
			code = 404
		}
		if errors.Is(err, memory.ErrInvalid) {
			code = 400
		}
		http.Error(w, err.Error(), code)
		return
	}
	if parts[1] == "forget" {
		_, _ = bank.RecordLedger(r.Context(), memory.LedgerRow{Stage: "memory_forgotten", Status: memory.LedgerSuccess, CountsJSON: `{"count":1}`})
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (d *Daemon) handleObservationPromote(w http.ResponseWriter, r *http.Request) {
	bank, ok := d.memoryBank(w, r, http.MethodPost, http.StatusInternalServerError)
	if !ok {
		return
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/memory/observations/"), "/"), "/")
	if len(parts) != 2 || parts[1] != "promote" {
		http.NotFound(w, r)
		return
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid observation id", 400)
	}
	o, err := bank.ObservationByID(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	legacyText, legacyKind := legacyObservationMeaning(o)
	newID, err := bank.AddMemory(r.Context(), memory.MemoryItem{SessionID: o.SessionID, EventID: o.EventID, Author: o.Author, Kind: legacyKind, Text: legacyText, Quote: o.Quote, Importance: .7, Veracity: memory.VeracityStated})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int64{"id": newID})
}

func legacyObservationMeaning(o memory.Observation) (string, string) {
	var prior struct {
		Text          string `json:"text"`
		CandidateText string `json:"candidate_text"`
		Statement     string `json:"statement"`
		Kind          string `json:"kind"`
	}
	_ = json.Unmarshal([]byte(o.DeciderJSON), &prior)
	text := firstNonBlank(prior.Text, prior.CandidateText, prior.Statement, o.Quote)
	kind := memoryMemoryKind(firstNonBlank(prior.Kind, o.Kind))
	return text, kind
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func memoryMemoryKind(kind string) string {
	switch kind {
	case memory.KindPreference, memory.KindInstruction, memory.KindDecision, memory.KindFact:
		return kind
	default:
		return memory.KindFact
	}
}

func (d *Daemon) handleEpisodes(w http.ResponseWriter, r *http.Request) {
	bank, ok := d.memoryBank(w, r, http.MethodGet, http.StatusInternalServerError)
	if !ok {
		return
	}
	rows, err := bank.ListEpisodes(r.Context(), memoryLimit(r, 50))
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	out := make([]episodeView, 0, len(rows))
	for _, ep := range rows {
		out = append(out, episodeView{ID: ep.ID, SessionID: ep.SessionID, EventRange: ep.EventRange, Text: ep.Text, Importance: ep.Importance, CreatedAt: ep.CreatedAt})
	}
	writeJSON(w, http.StatusOK, out)
}

type episodeView struct {
	ID         int64     `json:"id"`
	SessionID  string    `json:"sessionId"`
	EventRange string    `json:"eventRange"`
	Text       string    `json:"text"`
	Importance float64   `json:"importance"`
	CreatedAt  time.Time `json:"createdAt"`
}
