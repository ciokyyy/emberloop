// Package memory is the Daemon's memory engine: one SQLite bank per User
// holding observations, Rules, the decision queue, Profile versions and the
// nightly ledger, with FTS5 recall, append-only Rules, a human purge and
// VACUUM INTO snapshots.
//
// Memory is the seam the Daemon calls. Bank is the SQLite implementation and
// Fake the in-memory one for API tests; both pass the same contract suite.
package memory

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// Rule statuses. Recall returns only live Rules: not superseded, not dormant.
const (
	StatusActive     = "active"
	StatusParked     = "parked"
	StatusDormant    = "dormant"
	StatusSuperseded = "superseded"
	StatusForgotten  = "forgotten"
)

// Kinds of Rule and observation.
const (
	KindPreference  = "preference"
	KindDecision    = "decision"
	KindAntipattern = "antipattern"
	KindFact        = "fact"
	KindProcedure   = "procedure"
	KindInstruction = "instruction"
)

// Queue kinds and statuses. Rejected items stay: they are the Decider labels.
const (
	QueueRule         = "rule"
	QueueProfilePatch = "profile_patch"
	QueuePromotion    = "promotion"

	QueuePending  = "pending"
	QueueApproved = "approved"
	QueueRejected = "rejected"
)

// Ledger statuses, one row per nightly stage run.
const (
	LedgerSuccess = "SUCCESS"
	LedgerNoData  = "NO_DATA"
	LedgerFailed  = "FAILED"
)

const (
	// MaxRuleChars caps Rule text, counted in characters.
	MaxRuleChars = 300
	// MaxRecall caps the result size of one search.
	MaxRecall = 20
	// MaxList caps one Memory page list. Search stays at MaxRecall;
	// an empty query lists the newest records instead.
	MaxList = 100
	// SnapshotKeep is how many snapshots a bank keeps.
	SnapshotKeep = 10
	// SnapshotInterval is the minimum age of the newest snapshot before another is taken.
	SnapshotInterval = 6 * time.Hour
	// PurgedText replaces purged text in queue, ledger and Profile fields.
	PurgedText = "[purged]"
)

var (
	ErrNotFound = errors.New("memory: not found")
	ErrInvalid  = errors.New("memory: invalid")
	ErrClosed   = errors.New("memory: closed")
)

// Memory is one User's bank. Writes never delete: edits are new Rule
// versions and new Profile versions, and only Purge removes rows.
type Memory interface {
	AddObservation(ctx context.Context, o Observation) (int64, error)
	// BumpObservationEvidence increments one observation's evidence_count for
	// a duplicate hit and returns the updated row. The miner calls it; nothing
	// is ever deleted or merged.
	BumpObservationEvidence(ctx context.Context, id int64) (Observation, error)
	// ObservationByEvent returns the observation stored for one Session
	// Event, or ErrNotFound. The miner uses it to recognize a replayed Event
	// exactly, whatever else its quote matches.
	ObservationByEvent(ctx context.Context, sessionID, eventID string) (Observation, error)
	// ObservationByID returns the observation with the bank ID, or ErrNotFound.
	// Approval resolves source-observation authors and dates through it.
	ObservationByID(ctx context.Context, id int64) (Observation, error)
	SearchObservations(ctx context.Context, text string, limit int) ([]Observation, error)
	// ObservationsBySubjectKey returns every observation with the normalized
	// subject key, oldest first. The queue promotion stage reads it to test
	// recurrence across Sessions.
	ObservationsBySubjectKey(ctx context.Context, subjectKey string) ([]Observation, error)
	// ListObservations returns the newest observations, newest first, at most
	// MaxList. An empty bank gives an empty list, not an error. The Memory
	// page lists from it; search uses SearchObservations.
	ListObservations(ctx context.Context, limit int) ([]Observation, error)
	AddRule(ctx context.Context, r Rule) (Rule, error)
	ReviseRule(ctx context.Context, next Rule) (Rule, error)
	SetRuleStatus(ctx context.Context, ruleID, status string) error
	RuleVersions(ctx context.Context, ruleID string) ([]Rule, error)
	Recall(ctx context.Context, text string, limit int) ([]Rule, error)
	// LiveRules returns the latest version of every live Rule (not superseded,
	// not dormant), ordered by rule ID. The nightly anchor check refreshes
	// dormant status over it; parked Rules stay live for manual recall.
	LiveRules(ctx context.Context) ([]Rule, error)
	// RulesBySubjectKey returns every Rule version with the normalized
	// subject key, ordered by rule ID then version. Callers keep the latest
	// live version per Rule for supersession and disagreement checks.
	RulesBySubjectKey(ctx context.Context, subjectKey string) ([]Rule, error)
	// ListRules returns the latest live version of every Rule, newest first,
	// at most MaxList. Superseded and dormant versions stay out, like Recall.
	// The Memory page lists from it; search uses Recall.
	ListRules(ctx context.Context, limit int) ([]Rule, error)

	// Memories (auto-extracted).
	AddMemory(ctx context.Context, m MemoryItem) (int64, error)
	InsertMemory(ctx context.Context, m MemoryItem) (int64, error)
	GetMemory(ctx context.Context, id int64) (MemoryItem, error)
	MemoryByID(ctx context.Context, id int64) (MemoryItem, error)
	ListMemories(ctx context.Context, kind, status string, limit int) ([]MemoryItem, error)
	RecallMemoryItems(ctx context.Context, text string, limit int, includeSuperseded bool) ([]MemoryItem, error)
	SearchMemories(ctx context.Context, text string, kind, status string, limit int) ([]MemoryItem, error)
	SetMemoryStatus(ctx context.Context, id int64, status string) error
	ReviseMemory(ctx context.Context, oldID int64, next MemoryItem) (int64, error)
	PinMemory(ctx context.Context, id int64, pinned int) error
	SetMemoryPinned(ctx context.Context, id int64, pinned bool) error
	BumpMemory(ctx context.Context, id int64) (MemoryItem, error)
	BumpMemoryMention(ctx context.Context, id int64) (MemoryItem, error)
	DedupAndStoreMemory(ctx context.Context, m MemoryItem) (MemoryStoreResult, error)
	SetMemoryEmbedding(ctx context.Context, id int64, embedding []byte, model string) error
	RecallMemories(ctx context.Context, text string, weights RecallWeights, limit int) ([]MemoryHit, error)
	RecallEpisodes(ctx context.Context, text string, weights RecallWeights, limit int) ([]EpisodeHit, error)

	// Episodes.
	AddEpisode(ctx context.Context, e Episode) (int64, error)
	InsertEpisode(ctx context.Context, e Episode) (int64, error)
	GetEpisode(ctx context.Context, id int64) (Episode, error)
	EpisodeByID(ctx context.Context, id int64) (Episode, error)
	ListEpisodes(ctx context.Context, limit int) ([]Episode, error)
	SetEpisodeEmbedding(ctx context.Context, id int64, embedding []byte, model string) error
	SetEpisodeConsolidated(ctx context.Context, id int64, at time.Time) error

	Enqueue(ctx context.Context, item QueueItem) (int64, error)
	Decide(ctx context.Context, id int64, status, decidedBy string) error
	// AnnotateQueueRelation records contradiction pairing on a pending
	// kind=rule queue item: the relation (duplicate, refines or contradicts)
	// to the stored Rule the candidate overlaps, so the approver sees the
	// conflict without recomputing it. Only pending items are annotated;
	// decided items stay as labels. An empty relation clears an annotation.
	AnnotateQueueRelation(ctx context.Context, id int64, relation, relatedRuleID string) error
	Queue(ctx context.Context) ([]QueueItem, error)
	// ApproveRule approves one pending kind=rule queue item and creates its
	// Rule in one transaction: the item must be pending, the rule text is
	// refused over MaxRuleChars (never clipped), the named live Rules become
	// superseded and the item becomes approved. Any failure rolls everything
	// back, so a failed approval leaves no Rule, no supersession and a still
	// pending item. Missing supersede targets are skipped (a human purge may
	// have removed them); every other failure aborts.
	ApproveRule(ctx context.Context, itemID int64, decidedBy string, rule Rule, supersedeIDs []string) (Rule, error)
	// MarkRulePromoted stamps every version of ruleID as promoted by promotedBy.
	// Status and provenance (text, kind, anchors, sources, approver) stay
	// untouched; only the promoted columns change, which the rules guard
	// allows alongside status. Unknown IDs are ErrNotFound.
	MarkRulePromoted(ctx context.Context, ruleID, promotedBy string) error

	// GetProfile returns the latest Profile version, or Version 0 with no
	// lines when the User never saved one.
	GetProfile(ctx context.Context) (ProfileVersion, error)
	// ProfileHistory returns every Profile version, oldest first.
	ProfileHistory(ctx context.Context) ([]ProfileVersion, error)
	// AppendProfileVersion validates patch against the latest version,
	// refuses an over-2 KB render with a condense message, and stores the
	// next version. A retired line stays stored and hidden.
	AppendProfileVersion(ctx context.Context, patch ProfilePatch, approvedBy string) (ProfileVersion, error)
	// RestoreProfileVersion copies an older version's lines as the next
	// version, so any version can be restored without deleting history.
	RestoreProfileVersion(ctx context.Context, version int, approvedBy string) (ProfileVersion, error)

	RecordLedger(ctx context.Context, row LedgerRow) (int64, error)
	Ledger(ctx context.Context) ([]LedgerRow, error)

	Purge(ctx context.Context, sel Selection) (PurgeResult, error)
	Snapshot(ctx context.Context) (SnapshotResult, error)
}

// Observation is a grounded candidate. It is searchable and never injected.
// CreatedAt is set by the bank; the input value is ignored.
type Observation struct {
	ID             int64
	SessionID      string
	EventID        string
	Author         string // User ID of the Event sender
	Quote          string
	Kind           string
	SubjectKey     string
	EvidenceCount  int // at least 1; zero means 1
	DeciderJSON    string
	ObservedCommit string
	CreatedAt      time.Time
}

// Rule is one version of an approved, injectable note. ApprovedAt is set by
// the bank. PromotedAt/By stamp a Rule copied to project knowledge (the
// nearest AGENTS.md or a project note file) by a human-approved promotion;
// zero means never promoted. Stamping never touches status or provenance.
type Rule struct {
	RuleID               string
	Version              int
	Text                 string
	Kind                 string
	SubjectKey           string
	Anchors              []string
	ObservedCommit       string
	Status               string
	SourceObservationIDs []int64
	ApprovedBy           string
	ApprovedAt           time.Time
	PromotedAt           time.Time
	PromotedBy           string
}

// QueueItem is a pending decision: a Rule, a Profile patch or a promotion.
// Payload is JSON. CreatedAt and DecidedAt are set by the bank.
type QueueItem struct {
	ID        int64
	Kind      string
	Payload   string
	Status    string
	DecidedBy string
	DecidedAt time.Time
	CreatedAt time.Time
}

// LedgerRow records one nightly stage run. CountsJSON defaults to "{}".
type LedgerRow struct {
	ID         int64
	RunID      string
	Stage      string
	Status     string
	CountsJSON string
	Detail     string
	CreatedAt  time.Time
}

// Selection names the records a human purge removes. Rules removes every
// version of each rule ID.
type Selection struct {
	Observations []int64
	Memories     []int64
	Episodes     []int64
	Rules        []string
	Queue        []int64
	Ledger       []int64
}

// PurgeResult counts rows removed and rows where purged text was cleared.
type PurgeResult struct {
	Observations int
	Memories     int
	Episodes     int
	Rules        int
	Queue        int
	Ledger       int
	Scrubbed     int // rows changed to drop purged text (queue, ledger, Profile)
	Snapshots    int // snapshot files purged
}

// SnapshotResult says whether a snapshot was taken. Kept and Pruned describe
// the snapshot set after the call.
type SnapshotResult struct {
	Taken  bool
	Path   string // empty when skipped or when the Fake takes it
	Kept   int
	Pruned int
}

// Memory is one automatically extracted durable item.
type MemoryItem struct {
	ID             int64
	SessionID      string
	EventID        string
	Author         string
	Kind           string
	Text           string
	Quote          string
	Importance     float64
	Veracity       string
	MentionCount   int
	Status         string
	SupersededBy   int64
	Pinned         int
	Embedding      []byte
	EmbeddingModel string
	ExtractorJSON  string
	ObservedCommit string
	CreatedAt      time.Time
	LastSeenAt     time.Time
}

// Episode stores the redacted text of one user turn.
type Episode struct {
	ID             int64
	SessionID      string
	EventRange     string
	Author         string
	Text           string
	Importance     float64
	Embedding      []byte
	EmbeddingModel string
	ConsolidatedAt time.Time
	CreatedAt      time.Time
}

type Options struct {
	// Now is the clock. Nil reads time.Now. Tests inject a fixed clock.
	Now            func() time.Time
	Embedder       Embedder
	EmbeddingModel string
}

func (o Options) now() time.Time {
	if o.Now == nil {
		return time.Now().UTC()
	}
	return o.Now().UTC()
}

const (
	VeracityStated   = "stated"
	VeracityInferred = "inferred"
)

var userIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)

func validUserID(id string) bool { return userIDPattern.MatchString(id) }

func validKind(k string) bool {
	switch k {
	case KindPreference, KindDecision, KindAntipattern, KindFact, KindProcedure:
		return true
	}
	return false
}

func validStatus(s string) bool {
	switch s {
	case StatusActive, StatusParked, StatusDormant, StatusSuperseded:
		return true
	}
	return false
}

func validQueueKind(k string) bool {
	switch k {
	case QueueRule, QueueProfilePatch, QueuePromotion:
		return true
	}
	return false
}

func validLedgerStatus(s string) bool {
	switch s {
	case LedgerSuccess, LedgerNoData, LedgerFailed:
		return true
	}
	return false
}

func validRule(r Rule) error {
	switch {
	case r.Text == "":
		return fmt.Errorf("%w: rule text empty", ErrInvalid)
	case utf8.RuneCountInString(r.Text) > MaxRuleChars:
		return fmt.Errorf("%w: rule text %d chars, max %d", ErrInvalid, utf8.RuneCountInString(r.Text), MaxRuleChars)
	case !validKind(r.Kind):
		return fmt.Errorf("%w: rule kind %q", ErrInvalid, r.Kind)
	case !validStatus(r.Status):
		return fmt.Errorf("%w: rule status %q", ErrInvalid, r.Status)
	case r.ApprovedBy == "":
		return fmt.Errorf("%w: rule has no approver", ErrInvalid)
	case r.Kind == KindPreference && len(r.Anchors) > 0:
		return fmt.Errorf("%w: preference rules carry no anchors", ErrInvalid)
	}
	// Anchors agree with cleanAnchor: anything cleanAnchor drops (whitespace,
	// empty) is refused here. The spec caps only Rule text; anchor count and
	// length are extraction caps (MaxAnchors, MaxAnchorRunes), not write limits.
	for _, a := range r.Anchors {
		switch {
		case a == "":
			return fmt.Errorf("%w: empty anchor", ErrInvalid)
		case strings.ContainsAny(a, " \t\r\n\f\v"):
			return fmt.Errorf("%w: anchor %q has whitespace", ErrInvalid, a)
		}
	}
	return nil
}

func validObservation(o Observation) error {
	switch {
	case o.SessionID == "" || o.EventID == "" || o.Author == "":
		return fmt.Errorf("%w: observation needs session, event and author", ErrInvalid)
	case o.Quote == "":
		return fmt.Errorf("%w: observation quote empty", ErrInvalid)
	case !validKind(o.Kind):
		return fmt.Errorf("%w: observation kind %q", ErrInvalid, o.Kind)
	case o.EvidenceCount < 0:
		return fmt.Errorf("%w: evidence count %d", ErrInvalid, o.EvidenceCount)
	}
	return nil
}

func validQueueItem(q QueueItem) error {
	switch {
	case !validQueueKind(q.Kind):
		return fmt.Errorf("%w: queue kind %q", ErrInvalid, q.Kind)
	case !json.Valid([]byte(q.Payload)):
		return fmt.Errorf("%w: queue payload is not JSON", ErrInvalid)
	}
	return nil
}

// live reports whether a Rule with this status is searchable.
func live(status string) bool {
	return status != StatusSuperseded && status != StatusDormant
}

// snapshotDue reports whether a snapshot may be taken at now. newest is the
// time of the newest snapshot; zero when there is none.
func snapshotDue(newest, now time.Time) bool {
	return newest.IsZero() || now.Sub(newest) >= SnapshotInterval
}

// newID returns a random 128-bit hex ID for a Rule.
func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("memory: random id: %v", err))
	}
	return hex.EncodeToString(b[:])
}

// timeText stores a time as RFC 3339 with nanoseconds, UTC.
func timeText(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func parseTimeText(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// clampLimit caps a search limit to MaxRecall; zero and negative mean the cap.
func clampLimit(n int) int {
	if n <= 0 || n > MaxRecall {
		return MaxRecall
	}
	return n
}

// clampList caps a Memory page list limit to MaxList; zero and negative mean
// the cap. Lists show more than one search page.
func clampList(n int) int {
	if n <= 0 || n > MaxList {
		return MaxList
	}
	return n
}

func validMemory(m MemoryItem) error {
	switch {
	case m.SessionID == "" || m.EventID == "" || m.Author == "":
		return fmt.Errorf("%w: memory needs session, event and author", ErrInvalid)
	case m.Text == "":
		return fmt.Errorf("%w: memory text empty", ErrInvalid)
	case utf8.RuneCountInString(m.Text) > 300:
		return fmt.Errorf("%w: memory text too long", ErrInvalid)
	case m.Quote == "":
		return fmt.Errorf("%w: memory quote empty", ErrInvalid)
	case !validMemoryKind(m.Kind):
		return fmt.Errorf("%w: memory kind %q", ErrInvalid, m.Kind)
	case !validMemoryStatus(m.Status) && m.Status != "":
		return fmt.Errorf("%w: memory status %q", ErrInvalid, m.Status)
	case m.Veracity != "" && m.Veracity != VeracityStated && m.Veracity != VeracityInferred:
		return fmt.Errorf("%w: memory veracity %q", ErrInvalid, m.Veracity)
	}
	return nil
}

func validMemoryKind(k string) bool {
	switch k {
	case KindPreference, KindInstruction, KindDecision, KindFact:
		return true
	}
	return false
}

func validMemoryStatus(s string) bool {
	if s == "" {
		return true
	}
	switch s {
	case StatusActive, StatusParked, StatusSuperseded, StatusForgotten:
		return true
	}
	return false
}

func validEpisode(e Episode) error {
	if e.SessionID == "" || e.Author == "" || e.Text == "" {
		return fmt.Errorf("%w: episode needs session, author and text", ErrInvalid)
	}
	if len(e.Text) > 4096 {
		return fmt.Errorf("%w: episode text too long", ErrInvalid)
	}
	return nil
}

// validJSON reports whether s is well-formed JSON.
func validJSON(s string) bool { return json.Valid([]byte(s)) }
