package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"example.com/emberloop-web/memory"
)

// Memory page: the User's own bank behind five routes. Every route resolves
// the signed-in User and opens that User's bank only, so a User never reads
// another User's Rules or observations; admins get no extra read access. The
// machine token is no User and answers 401 like /api/users.
//
//   - GET /api/rules?q=&limit=: list (empty q, newest first) or FTS5 search
//     the User's Rules, live versions only.
//   - GET /api/observations?q=&limit=: the same for observations.
//   - GET /api/memory/ledger: every ledger row, oldest first.
//   - GET /api/memory/status: the engine status chip plus the stats the page
//     shows (queue budget and latency, ledger size, last row, last nightly).
//     A failing engine answers 503 with available false.
//   - POST /api/memory/purge {"selection","confirm":"PURGE"}: the human purge.
//     The typed confirmation is required; the result reports what it removed.

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
type memoryStatus struct {
	Available   bool               `json:"available"`
	Queue       *memory.QueueStats `json:"queue,omitempty"`
	LedgerSize  int                `json:"ledgerSize"`
	LastLedger  *ledgerView        `json:"lastLedger,omitempty"`
	LastNightly *ledgerView        `json:"lastNightly,omitempty"`
	Error       string             `json:"error,omitempty"`
}

// purgeRequest is POST /api/memory/purge. Selection IDs name the records the
// human chose on the page; Confirm must be the typed word PURGE.
type purgeRequest struct {
	Observations []int64  `json:"observations"`
	Rules        []string `json:"rules"`
	Queue        []int64  `json:"queue"`
	Ledger       []int64  `json:"ledger"`
	Confirm      string   `json:"confirm"`
}

// purgeResultView reports what the purge removed, in UI keys.
type purgeResultView struct {
	Observations int `json:"observations"`
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
		Rules:        body.Rules,
		Queue:        body.Queue,
		Ledger:       body.Ledger,
	}
	if len(sel.Observations)+len(sel.Rules)+len(sel.Queue)+len(sel.Ledger) == 0 {
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
		Rules:        res.Rules,
		Queue:        res.Queue,
		Ledger:       res.Ledger,
		Scrubbed:     res.Scrubbed,
		Snapshots:    res.Snapshots,
	})
}
