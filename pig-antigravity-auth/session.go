package antigravity

import (
	"crypto/rand"
	"fmt"
	"strconv"
	"sync"
	"time"
)

// Request identity the agy CLI attaches to every agent request: a stable
// conversation and trajectory per pig session, a step counter, and labels.

const (
	agyCLIVersion    = "1.1.24"
	agyCLIChangeList = "974782877"
	sessionTTL       = 24 * time.Hour
	maxSessions      = 256
)

type agySession struct {
	conversationID   string
	trajectoryID     string
	numericSessionID string
	lastExecutionID  string
	usedClaude       bool
	usedNonGemini    bool
	lastAccess       time.Time
	lastRequest      int64
}

type sessionStore struct {
	mu      sync.Mutex
	entries map[string]*agySession
	now     func() time.Time
}

func newSessionStore() *sessionStore {
	return &sessionStore{entries: map[string]*agySession{}, now: time.Now}
}

// fnv1a64Signed is the FNV-1a 64-bit hash of s read as a signed integer.
func fnv1a64Signed(s string) string {
	h := uint64(0xcbf29ce484222325)
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 0x100000001b3
	}
	return strconv.FormatInt(int64(h), 10)
}

func uuid4() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// begin returns the session for key and a request timestamp that is strictly
// increasing within it. The session is shared: callers hold it only while
// building one request.
func (s *sessionStore) begin(key string) (*agySession, int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	s.prune(now, key)
	e := s.entries[key]
	if e == nil {
		e = &agySession{conversationID: uuid4(), trajectoryID: uuid4(), numericSessionID: fnv1a64Signed("")}
		s.entries[key] = e
	}
	e.lastAccess = now
	ts := now.UnixMilli()
	if ts <= e.lastRequest {
		ts = e.lastRequest + 1
	}
	e.lastRequest = ts
	return e, ts
}

// complete marks the end of an execution: the next request carries it as
// last_execution_id.
func (s *sessionStore) complete(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.entries[key]; e != nil {
		e.lastExecutionID = uuid4()
	}
}

func (s *sessionStore) prune(now time.Time, keep string) {
	for k, e := range s.entries {
		if k != keep && now.Sub(e.lastAccess) > sessionTTL {
			delete(s.entries, k)
		}
	}
	for len(s.entries) >= maxSessions {
		oldest, at := "", time.Time{}
		for k, e := range s.entries {
			if k == keep {
				continue
			}
			if oldest == "" || e.lastAccess.Before(at) {
				oldest, at = k, e.lastAccess
			}
		}
		if oldest == "" {
			return
		}
		delete(s.entries, oldest)
	}
}

// requestSteps counts steps the way the agy CLI does: one per content plus one
// per function response.
func requestSteps(contents []any) int {
	n := len(contents)
	for _, c := range contents {
		parts, _ := asMap(c)["parts"].([]any)
		for _, p := range parts {
			if _, ok := asMap(p)["functionResponse"]; ok {
				n++
			}
		}
	}
	if n < 1 {
		return 1
	}
	return n
}

// agentMetadata is the request id, numeric session id and labels of one request.
func agentMetadata(s *agySession, contents []any, wire string, ts int64) (requestID string, labels map[string]string) {
	last := requestSteps(contents)
	if s.lastExecutionID != "" {
		last++
	}
	lower := toLower(wire)
	isClaude := hasPrefix(lower, "claude-")
	s.usedClaude = s.usedClaude || isClaude
	s.usedNonGemini = s.usedNonGemini || isClaude || hasPrefix(lower, "gpt-")
	labels = map[string]string{}
	if s.lastExecutionID != "" {
		labels["last_execution_id"] = s.lastExecutionID
	}
	labels["last_step_index"] = strconv.Itoa(last)
	if e, ok := agyModelEnum[lower]; ok {
		labels["model_enum"] = e
	}
	labels["trajectory_id"] = s.trajectoryID
	labels["used_claude"] = strconv.FormatBool(s.usedClaude)
	labels["used_claude_conservative"] = strconv.FormatBool(s.usedClaude)
	labels["used_non_gemini_model"] = strconv.FormatBool(s.usedNonGemini)
	return fmt.Sprintf("agent/%s/%d/%s/%d", s.conversationID, ts, s.trajectoryID, last+1), labels
}
