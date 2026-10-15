package memory

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// Fake is an in-memory Memory for API tests. It keeps the Bank contract:
// the same validation, append-only Rule versions, live filter, purge scrub
// and snapshot policy. Matching uses the same Tokens and SplitText rules as
// the FTS5 index, but without FTS5, so ranking is a term count.
type Fake struct {
	mu         sync.Mutex
	now        func() time.Time
	nextObs    int64
	nextQueue  int64
	nextMem    int64
	nextEp     int64
	nextLedger int64
	obs        []Observation
	mems       []MemoryItem
	eps        []Episode
	rules      []Rule
	queue      []QueueItem
	ledger     []LedgerRow
	profiles   []ProfileVersion
	snaps      []time.Time
}

var _ Memory = (*Fake)(nil)

// NewFake returns an empty in-memory bank.
func NewFake(opts Options) *Fake {
	return &Fake{now: opts.now}
}

func (f *Fake) AddObservation(_ context.Context, o Observation) (int64, error) {
	o.SubjectKey = NormalizeSubjectKey(o.SubjectKey)
	if err := validObservation(o); err != nil {
		return 0, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextObs++
	o.ID = f.nextObs
	o.EvidenceCount = max(o.EvidenceCount, 1)
	o.CreatedAt = f.now()
	f.obs = append(f.obs, o)
	return o.ID, nil
}
func (f *Fake) BumpObservationEvidence(_ context.Context, id int64) (Observation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.obs {
		if f.obs[i].ID != id {
			continue
		}
		f.obs[i].EvidenceCount++
		return f.obs[i], nil
	}
	return Observation{}, fmt.Errorf("%w: observation %d", ErrNotFound, id)
}

// ObservationByEvent returns the observation stored for one Session Event,
// or ErrNotFound.
func (f *Fake) ObservationByEvent(_ context.Context, sessionID, eventID string) (Observation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, o := range f.obs {
		if o.SessionID == sessionID && o.EventID == eventID {
			return o, nil
		}
	}
	return Observation{}, fmt.Errorf("%w: observation %s event %s", ErrNotFound, sessionID, eventID)
}

func (f *Fake) ObservationByID(_ context.Context, id int64) (Observation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, o := range f.obs {
		if o.ID == id {
			return o, nil
		}
	}
	return Observation{}, fmt.Errorf("%w: observation %d", ErrNotFound, id)
}

func (f *Fake) SearchObservations(_ context.Context, text string, limit int) ([]Observation, error) {
	terms := Tokens(text)
	if len(terms) == 0 {
		return nil, nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	type hit struct {
		o     Observation
		score int
	}
	var hits []hit
	for _, o := range f.obs {
		if n := matchCount(terms, o.Quote); n > 0 {
			hits = append(hits, hit{o, n})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
	var out []Observation
	for _, h := range hits[:min(len(hits), clampLimit(limit))] {
		out = append(out, h.o)
	}
	return out, nil
}

func (f *Fake) ObservationsBySubjectKey(_ context.Context, subjectKey string) ([]Observation, error) {
	key := NormalizeSubjectKey(subjectKey)
	if key == "" {
		return nil, nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []Observation
	for _, o := range f.obs {
		if o.SubjectKey == key {
			out = append(out, o)
		}
	}
	return out, nil
}

func (f *Fake) ListObservations(_ context.Context, limit int) ([]Observation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := clampList(limit)
	if len(f.obs) == 0 {
		return nil, nil
	}
	out := append([]Observation(nil), f.obs...)
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	if len(out) > n {
		out = out[:n]
	}
	return out, nil
}

func (f *Fake) AddRule(_ context.Context, r Rule) (Rule, error) {
	if r.RuleID == "" {
		r.RuleID = newID()
	}
	r.Version = 1
	if r.Status == "" {
		r.Status = StatusActive
	}
	r.SubjectKey = NormalizeSubjectKey(r.SubjectKey)
	r.ApprovedAt = f.now()
	r.PromotedAt = time.Time{}
	r.PromotedBy = ""
	if err := validRule(r); err != nil {
		return Rule{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if latestVersion(f.rules, r.RuleID) > 0 {
		return Rule{}, fmt.Errorf("%w: rule %s exists; revise it", ErrInvalid, r.RuleID)
	}
	f.rules = append(f.rules, copyRule(r))
	return r, nil
}

func (f *Fake) ReviseRule(_ context.Context, next Rule) (Rule, error) {
	if next.RuleID == "" {
		return Rule{}, fmt.Errorf("%w: rule id empty", ErrInvalid)
	}
	if next.Status == "" {
		next.Status = StatusActive
	}
	next.SubjectKey = NormalizeSubjectKey(next.SubjectKey)
	next.ApprovedAt = f.now()
	next.PromotedAt = time.Time{}
	next.PromotedBy = ""
	if err := validRule(next); err != nil {
		return Rule{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	latest := latestVersion(f.rules, next.RuleID)
	if latest == 0 {
		return Rule{}, fmt.Errorf("%w: rule %s", ErrNotFound, next.RuleID)
	}
	for i := range f.rules {
		if f.rules[i].RuleID == next.RuleID && f.rules[i].Version == latest {
			f.rules[i].Status = StatusSuperseded
		}
	}
	next.Version = latest + 1
	f.rules = append(f.rules, copyRule(next))
	return next, nil
}

func (f *Fake) SetRuleStatus(_ context.Context, ruleID, status string) error {
	if !validStatus(status) {
		return fmt.Errorf("%w: status %q", ErrInvalid, status)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	latest := latestVersion(f.rules, ruleID)
	if latest == 0 {
		return fmt.Errorf("%w: rule %s", ErrNotFound, ruleID)
	}
	for i := range f.rules {
		if f.rules[i].RuleID == ruleID && f.rules[i].Version == latest {
			f.rules[i].Status = status
		}
	}
	return nil
}

func (f *Fake) RuleVersions(_ context.Context, ruleID string) ([]Rule, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []Rule
	for _, r := range f.rules {
		if r.RuleID == ruleID {
			out = append(out, copyRule(r))
		}
	}
	return out, nil
}

func (f *Fake) Recall(_ context.Context, text string, limit int) ([]Rule, error) {
	terms := Tokens(text)
	if len(terms) == 0 {
		return nil, nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	type hit struct {
		r     Rule
		score int
	}
	var hits []hit
	for _, r := range f.rules {
		if !live(r.Status) {
			continue
		}
		if n := matchCount(terms, r.Text); n > 0 {
			hits = append(hits, hit{copyRule(r), n})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
	var out []Rule
	for _, h := range hits[:min(len(hits), clampLimit(limit))] {
		out = append(out, h.r)
	}
	return out, nil
}

func (f *Fake) LiveRules(_ context.Context) ([]Rule, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	latest := make(map[string]Rule)
	for _, r := range f.rules {
		if cur, ok := latest[r.RuleID]; !ok || r.Version > cur.Version {
			latest[r.RuleID] = r
		}
	}
	var out []Rule
	for _, r := range latest {
		if live(r.Status) {
			out = append(out, copyRule(r))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RuleID < out[j].RuleID })
	return out, nil
}

func (f *Fake) RulesBySubjectKey(_ context.Context, subjectKey string) ([]Rule, error) {
	key := NormalizeSubjectKey(subjectKey)
	if key == "" {
		return nil, nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []Rule
	for _, r := range f.rules {
		if r.SubjectKey == key {
			out = append(out, copyRule(r))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].RuleID != out[j].RuleID {
			return out[i].RuleID < out[j].RuleID
		}
		return out[i].Version < out[j].Version
	})
	return out, nil
}

func (f *Fake) ListRules(_ context.Context, limit int) ([]Rule, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := clampList(limit)
	latest := make(map[string]Rule)
	for _, r := range f.rules {
		if cur, ok := latest[r.RuleID]; !ok || r.Version > cur.Version {
			latest[r.RuleID] = copyRule(r)
		}
	}
	var out []Rule
	for _, r := range latest {
		if !live(r.Status) {
			continue
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].ApprovedAt.Equal(out[j].ApprovedAt) {
			return out[i].ApprovedAt.After(out[j].ApprovedAt)
		}
		return out[i].RuleID > out[j].RuleID
	})
	if len(out) > n {
		out = out[:n]
	}
	if out == nil {
		return nil, nil
	}
	return out, nil
}

func (f *Fake) Enqueue(_ context.Context, item QueueItem) (int64, error) {
	if err := validQueueItem(item); err != nil {
		return 0, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextQueue++
	f.queue = append(f.queue, QueueItem{
		ID:        f.nextQueue,
		Kind:      item.Kind,
		Payload:   item.Payload,
		Status:    QueuePending,
		CreatedAt: f.now(),
	})
	return f.nextQueue, nil
}

func (f *Fake) Decide(_ context.Context, id int64, status, decidedBy string) error {
	if status != QueueApproved && status != QueueRejected {
		return fmt.Errorf("%w: decision %q", ErrInvalid, status)
	}
	if decidedBy == "" {
		return fmt.Errorf("%w: decider empty", ErrInvalid)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.queue {
		if f.queue[i].ID != id {
			continue
		}
		if f.queue[i].Status != QueuePending {
			return fmt.Errorf("%w: queue item %d already decided", ErrInvalid, id)
		}
		f.queue[i].Status = status
		f.queue[i].DecidedBy = decidedBy
		f.queue[i].DecidedAt = f.now()
		return nil
	}
	return fmt.Errorf("%w: queue item %d", ErrNotFound, id)
}

func (f *Fake) AnnotateQueueRelation(_ context.Context, id int64, relation, relatedRuleID string) error {
	switch relation {
	case RelDuplicate, RelRefines, RelContradicts:
		if strings.TrimSpace(relatedRuleID) == "" {
			return fmt.Errorf("%w: relation %q needs a related rule", ErrInvalid, relation)
		}
	case "":
		if strings.TrimSpace(relatedRuleID) != "" {
			return fmt.Errorf("%w: empty relation with related rule %q", ErrInvalid, relatedRuleID)
		}
	default:
		return fmt.Errorf("%w: relation %q", ErrInvalid, relation)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.queue {
		if f.queue[i].ID != id {
			continue
		}
		if f.queue[i].Kind != QueueRule {
			return fmt.Errorf("%w: queue item %d is %q, not a rule", ErrInvalid, id, f.queue[i].Kind)
		}
		if f.queue[i].Status != QueuePending {
			return fmt.Errorf("%w: queue item %d already decided", ErrInvalid, id)
		}
		proposal, err := DecodeRememberProposal(f.queue[i].Payload)
		if err != nil {
			return err
		}
		if proposal.Relation == relation && proposal.RelatedRuleID == relatedRuleID {
			return nil
		}
		proposal.Relation, proposal.RelatedRuleID = relation, relatedRuleID
		raw := EncodeRuleProposal(proposal)
		if !validJSON(raw) {
			return fmt.Errorf("%w: queue payload is not JSON", ErrInvalid)
		}
		f.queue[i].Payload = raw
		return nil
	}
	return fmt.Errorf("%w: queue item %d", ErrNotFound, id)
}

func (f *Fake) ApproveRule(_ context.Context, itemID int64, decidedBy string, rule Rule, supersedeIDs []string) (Rule, error) {
	if decidedBy == "" {
		return Rule{}, fmt.Errorf("%w: decider empty", ErrInvalid)
	}
	if rule.RuleID == "" {
		rule.RuleID = newID()
	}
	rule.Version = 1
	if rule.Status == "" {
		rule.Status = StatusActive
	}
	rule.SubjectKey = NormalizeSubjectKey(rule.SubjectKey)
	if rule.ApprovedBy == "" {
		rule.ApprovedBy = decidedBy
	}
	rule.ApprovedAt = f.now()
	rule.PromotedAt = time.Time{}
	rule.PromotedBy = ""
	if err := validRule(rule); err != nil {
		return Rule{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	idx := -1
	for i := range f.queue {
		if f.queue[i].ID == itemID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return Rule{}, fmt.Errorf("%w: queue item %d", ErrNotFound, itemID)
	}
	if f.queue[idx].Kind != QueueRule {
		return Rule{}, fmt.Errorf("%w: queue item %d is %q, not a rule", ErrInvalid, itemID, f.queue[idx].Kind)
	}
	if f.queue[idx].Status != QueuePending {
		return Rule{}, fmt.Errorf("%w: queue item %d already decided", ErrInvalid, itemID)
	}
	if latestVersion(f.rules, rule.RuleID) > 0 {
		return Rule{}, fmt.Errorf("%w: rule %s exists; revise it", ErrInvalid, rule.RuleID)
	}
	seen := make(map[string]bool)
	for _, sid := range supersedeIDs {
		sid = strings.TrimSpace(sid)
		if sid == "" || sid == rule.RuleID || seen[sid] {
			continue
		}
		seen[sid] = true
		if latest := latestVersion(f.rules, sid); latest > 0 {
			for i := range f.rules {
				if f.rules[i].RuleID == sid && f.rules[i].Version == latest && live(f.rules[i].Status) {
					f.rules[i].Status = StatusSuperseded
				}
			}
		}
	}
	f.rules = append(f.rules, copyRule(rule))
	f.queue[idx].Status = QueueApproved
	f.queue[idx].DecidedBy = decidedBy
	f.queue[idx].DecidedAt = f.now()
	return rule, nil
}

func (f *Fake) MarkRulePromoted(_ context.Context, ruleID, promotedBy string) error {
	if err := validatePromotedBy(ruleID, promotedBy); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	now := f.now()
	found := false
	for i := range f.rules {
		if f.rules[i].RuleID != ruleID {
			continue
		}
		found = true
		f.rules[i].PromotedAt = now
		f.rules[i].PromotedBy = promotedBy
	}
	if !found {
		return fmt.Errorf("%w: rule %s", ErrNotFound, ruleID)
	}
	return nil
}

func (f *Fake) Queue(_ context.Context) ([]QueueItem, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]QueueItem(nil), f.queue...), nil
}

func (f *Fake) GetProfile(_ context.Context) (ProfileVersion, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if n := len(f.profiles); n > 0 {
		return copyProfile(f.profiles[n-1]), nil
	}
	return ProfileVersion{}, nil
}

func (f *Fake) ProfileHistory(_ context.Context) ([]ProfileVersion, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]ProfileVersion, 0, len(f.profiles))
	for _, v := range f.profiles {
		out = append(out, copyProfile(v))
	}
	return out, nil
}

func (f *Fake) AppendProfileVersion(_ context.Context, patch ProfilePatch, approvedBy string) (ProfileVersion, error) {
	if strings.TrimSpace(approvedBy) == "" {
		return ProfileVersion{}, fmt.Errorf("%w: profile needs an approver", ErrInvalid)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	var current []ProfileLine
	latest := 0
	if n := len(f.profiles); n > 0 {
		current = f.profiles[n-1].Lines
		latest = f.profiles[n-1].Version
	}
	next, err := ApplyProfilePatch(current, patch)
	if err != nil {
		return ProfileVersion{}, err
	}
	if err := checkProfileCap(next); err != nil {
		return ProfileVersion{}, err
	}
	out := f.insertProfileVersionLocked(latest, next, profilePatchJSON(patch), approvedBy)
	return out, nil
}

func (f *Fake) RestoreProfileVersion(_ context.Context, version int, approvedBy string) (ProfileVersion, error) {
	if strings.TrimSpace(approvedBy) == "" {
		return ProfileVersion{}, fmt.Errorf("%w: profile needs an approver", ErrInvalid)
	}
	if version < 1 {
		return ProfileVersion{}, fmt.Errorf("%w: profile version %d", ErrInvalid, version)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.profiles) == 0 {
		return ProfileVersion{}, fmt.Errorf("%w: profile has no versions", ErrNotFound)
	}
	var src *ProfileVersion
	for i := range f.profiles {
		if f.profiles[i].Version == version {
			src = &f.profiles[i]
			break
		}
	}
	if src == nil {
		return ProfileVersion{}, fmt.Errorf("%w: profile version %d", ErrNotFound, version)
	}
	latest := f.profiles[len(f.profiles)-1].Version
	out := f.insertProfileVersionLocked(latest, src.Lines, fmt.Sprintf(`{"restore":%d}`, version), approvedBy)
	return out, nil
}

// insertProfileVersionLocked stores lines as the version after latest.
// Callers hold f.mu.
func (f *Fake) insertProfileVersionLocked(latest int, lines []ProfileLine, patchDoc, approvedBy string) ProfileVersion {
	out := ProfileVersion{
		Version:    latest + 1,
		Lines:      append([]ProfileLine(nil), lines...),
		Patch:      patchDoc,
		ApprovedBy: approvedBy,
		CreatedAt:  f.now(),
	}
	f.profiles = append(f.profiles, copyProfile(out))
	return out
}

func (f *Fake) RecordLedger(_ context.Context, row LedgerRow) (int64, error) {
	if row.Stage == "" || !validLedgerStatus(row.Status) {
		return 0, fmt.Errorf("%w: ledger stage %q status %q", ErrInvalid, row.Stage, row.Status)
	}
	if row.CountsJSON == "" {
		row.CountsJSON = "{}"
	}
	if !validJSON(row.CountsJSON) {
		return 0, fmt.Errorf("%w: ledger counts are not JSON", ErrInvalid)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextLedger++
	row.ID = f.nextLedger
	row.CreatedAt = f.now()
	f.ledger = append(f.ledger, row)
	return row.ID, nil
}

func (f *Fake) Ledger(_ context.Context) ([]LedgerRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]LedgerRow(nil), f.ledger...), nil
}

// Purge removes the selected records, then scrubs their text from queue,
// ledger and Profile rows. The Fake keeps no snapshot files, so it purges none.
func (f *Fake) Purge(_ context.Context, sel Selection) (PurgeResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var res PurgeResult
	obsIDs := setOf(sel.Observations)
	ruleIDs := setOf(sel.Rules)
	queueIDs := setOf(sel.Queue)
	ledgerIDs := setOf(sel.Ledger)

	var texts []string
	var obs []Observation
	for _, o := range f.obs {
		if obsIDs[o.ID] {
			texts = append(texts, o.Quote)
			res.Observations++
			continue
		}
		obs = append(obs, o)
	}
	var rules []Rule
	for _, r := range f.rules {
		if ruleIDs[r.RuleID] {
			texts = append(texts, r.Text)
			res.Rules++
			continue
		}
		rules = append(rules, r)
	}
	var queue []QueueItem
	for _, q := range f.queue {
		if queueIDs[q.ID] {
			res.Queue++
			continue
		}
		var changed bool
		q.Payload, changed = scrubJSONText(q.Payload, texts)
		res.Scrubbed += boolInt(changed)
		queue = append(queue, q)
	}
	var ledger []LedgerRow
	for _, l := range f.ledger {
		if ledgerIDs[l.ID] {
			res.Ledger++
			continue
		}
		var changed bool
		l.Detail, changed = scrubPlain(l.Detail, texts)
		res.Scrubbed += boolInt(changed)
		ledger = append(ledger, l)
	}
	for i := range f.profiles {
		changed := false
		for j := range f.profiles[i].Lines {
			next, c := scrubPlain(f.profiles[i].Lines[j].Text, texts)
			if c {
				f.profiles[i].Lines[j].Text = next
				changed = true
			}
		}
		if next, c := scrubJSONText(f.profiles[i].Patch, texts); c {
			f.profiles[i].Patch = next
			changed = true
		}
		res.Scrubbed += boolInt(changed)
	}
	f.obs, f.rules, f.queue, f.ledger = obs, rules, queue, ledger
	return res, nil
}

// Snapshot follows the Bank policy: a new snapshot only when the newest is
// SnapshotInterval old, and the newest SnapshotKeep are kept.
func (f *Fake) Snapshot(_ context.Context) (SnapshotResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := f.now()
	var newest time.Time
	if n := len(f.snaps); n > 0 {
		newest = f.snaps[n-1]
	}
	if !snapshotDue(newest, now) {
		return SnapshotResult{Kept: len(f.snaps)}, nil
	}
	f.snaps = append(f.snaps, now)
	pruned := 0
	if extra := len(f.snaps) - SnapshotKeep; extra > 0 {
		f.snaps = f.snaps[extra:]
		pruned = extra
	}
	return SnapshotResult{Taken: true, Kept: len(f.snaps), Pruned: pruned}, nil
}

// matchCount counts the search terms that text holds. Text and its split copy
// both count, as the two FTS5 columns do.
func matchCount(terms []string, text string) int {
	held := make(map[string]bool)
	for _, w := range words(text) {
		held[strings.ToLower(w)] = true
	}
	for _, w := range words(SplitText(text)) {
		held[strings.ToLower(w)] = true
	}
	n := 0
	for _, t := range terms {
		if held[t] {
			n++
		}
	}
	return n
}

// boolInt is 1 for true and 0 for false: it turns a changed flag into a row count.
func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func latestVersion(rules []Rule, ruleID string) int {
	latest := 0
	for _, r := range rules {
		if r.RuleID == ruleID && r.Version > latest {
			latest = r.Version
		}
	}
	return latest
}

func copyRule(r Rule) Rule {
	r.Anchors = append([]string(nil), r.Anchors...)
	r.SourceObservationIDs = append([]int64(nil), r.SourceObservationIDs...)
	return r
}

func copyProfile(v ProfileVersion) ProfileVersion {
	v.Lines = append([]ProfileLine(nil), v.Lines...)
	return v
}

func setOf[T comparable](vals []T) map[T]bool {
	set := make(map[T]bool, len(vals))
	for _, v := range vals {
		set[v] = true
	}
	return set
}
