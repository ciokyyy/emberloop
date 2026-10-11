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
)

// Kinds of Rule and observation.
const (
	KindPreference  = "preference"
	KindDecision    = "decision"
	KindAntipattern = "antipattern"
	KindFact        = "fact"
	KindProcedure   = "procedure"
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
// versions, and only Purge removes rows.
type Memory interface {
	AddObservation(ctx context.Context, o Observation) (int64, error)
	SearchObservations(ctx context.Context, text string, limit int) ([]Observation, error)

	AddRule(ctx context.Context, r Rule) (Rule, error)
	ReviseRule(ctx context.Context, next Rule) (Rule, error)
	SetRuleStatus(ctx context.Context, ruleID, status string) error
	RuleVersions(ctx context.Context, ruleID string) ([]Rule, error)
	Recall(ctx context.Context, text string, limit int) ([]Rule, error)

	Enqueue(ctx context.Context, item QueueItem) (int64, error)
	Decide(ctx context.Context, id int64, status, decidedBy string) error
	Queue(ctx context.Context) ([]QueueItem, error)

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
// the bank.
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
	Rules        []string
	Queue        []int64
	Ledger       []int64
}

// PurgeResult counts rows removed and rows where purged text was cleared.
type PurgeResult struct {
	Observations int
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

// Options configures an Engine or a Fake.
type Options struct {
	// Now is the clock. Nil reads time.Now. Tests inject a fixed clock.
	Now func() time.Time
}

func (o Options) now() time.Time {
	if o.Now == nil {
		return time.Now().UTC()
	}
	return o.Now().UTC()
}

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

// validJSON reports whether s is well-formed JSON.
func validJSON(s string) bool { return json.Valid([]byte(s)) }
