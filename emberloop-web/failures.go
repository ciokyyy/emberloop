package web

import (
	"errors"
	"regexp"
)

// Provider failures sort by their text. The pig SDK puts a provider HTTP
// status in its error text: at the start, after a colon or an open paren, or
// after "HTTP" or "status". It names network failures with fixed phrases.
// This one classifier serves both the compress block path
// (classifyCompressError) and the Run fallback chain (chainFor in chains.go):
// quota, credit, auth and session-limit failures advance the chain, any other
// failure surfaces as today.

// failureClass is one provider failure kind.
type failureClass string

const (
	failureOther        failureClass = "other"
	failureQuota        failureClass = "quota"
	failureCredit       failureClass = "credit"
	failureAuth         failureClass = "auth"
	failureSessionLimit failureClass = "session-limit"
)

// advancesChain reports whether the class moves the Run chain to its next
// entry: quota, credit, auth and session-limit failures do, anything else
// does not.
func (c failureClass) advancesChain() bool { return c != failureOther }

// statusAt is where a provider HTTP status sits in SDK error text.
const statusAt = `(?:^|[:(]\s*|\bHTTP\s*|\bstatus(?:\s+code)?\s*)`

// quotaFailure matches exhausted allowances. The second branch is the SDK's
// insufficient_quota form, which \bquota\b alone never catches: the
// underscore joins the words, so no boundary sits before the q.
var quotaFailure = regexp.MustCompile(`(?i)` + `\bquota\b|insufficient[ _-]quota`)

// creditFailure matches spent accounts: 402 and the billing words. Quota
// stays with quotaFailure; together with authFailure and sessionFailure this
// covers what the block matcher caught before the split.
var creditFailure = regexp.MustCompile(`(?i)` + statusAt + `402(?:\)|:|\s*$|\s+status\b|\s+(?:unauthorized|payment|invalid))` +
	`|payment required|insufficient[ _-](?:credits?|balance|funds)|(?:no|out of|low) credits?\b|credit balance` +
	`|billing[ _](?:hard[ _])?limit|billing (?:required|error)`)

// authFailure matches rejected credentials: 401 and the key words. A bare
// "invalid" never matches on its own, so prose like a form validation error
// stays other.
var authFailure = regexp.MustCompile(`(?i)` + statusAt + `401(?:\)|:|\s*$|\s+status\b|\s+(?:unauthorized|payment|invalid))` +
	`|unauthori[sz]ed|authentication|api[ _-]?key`)

// sessionFailure matches a capped provider session. It needs the session
// word (or the capped phrase) beside the limit, so a bare "limit" in prose
// or a one-off deadline never matches.
var sessionFailure = regexp.MustCompile(`(?i)` +
	`session[ _-]?limit|session[ _-]?capped|capped[ _-]?session|limit[ _-]?capped` +
	`|too many sessions|concurrent sessions|maximum sessions|session[ _-]?quota`)

// transientFailure matches network, 5xx and rate-limit failures.
var transientFailure = regexp.MustCompile(`(?i)` + statusAt + `(?:429|5\d\d)(?:\)|:|\s*$|\s+status\b|\s+(?:too many|overloaded|internal|service|bad gateway|gateway|unavailable|requests|error))` +
	`|rate[ _-]?limit|too many requests|overloaded|timed out|i/o timeout|connection (?:error|reset|refused|closed)` +
	`|fetch failed|unexpected eof|no such host|network (?:error|is unreachable)|(?:service|server) unavailable|status: unavailable` +
	`|temporar(?:y|ily) (?:unavailable|failure|error|overloaded)|(?:^|: )terminated$`)

// classifyFailure sorts one provider failure by its text. The chain-advancing
// classes win over anything else: a 429 that names the quota is quota, not a
// transient rate limit.
func classifyFailure(err error) failureClass {
	if err == nil {
		return failureOther
	}
	msg := err.Error()
	switch {
	case quotaFailure.MatchString(msg):
		return failureQuota
	case creditFailure.MatchString(msg):
		return failureCredit
	case authFailure.MatchString(msg):
		return failureAuth
	case sessionFailure.MatchString(msg):
		return failureSessionLimit
	}
	return failureOther
}

// compressFailure is the class of one failed compress call.
type compressFailure int

const (
	failOther compressFailure = iota
	failTransient
	failBlock
)

// classifyCompressError sorts one failure by its text. A chain-advancing
// failure blocks the Session — session-limit included, by intent: a capped
// session holds for an explicit Retry rather than burning transient retries;
// a transient one retries under compressBackoff; any other failure falls
// through the fallback chain in summarize.go.
func classifyCompressError(err error) compressFailure {
	if err == nil {
		return failOther
	}
	if classifyFailure(err) != failureOther {
		return failBlock
	}
	if transientFailure.MatchString(err.Error()) {
		return failTransient
	}
	return failOther
}

// sessionLimitFailureInTurn reports whether the latest Session turn ended
// with a provider session-limit error.
func sessionLimitFailureInTurn(events []Event) bool {
	for i := len(events) - 1; i >= 0; i-- {
		e := events[i]
		if e.Kind == "prompt" {
			return false
		}
		if e.Kind == "error" {
			return classifyFailure(errors.New(e.Text)) == failureSessionLimit
		}
	}
	return false
}

// sessionLimitCapped reports whether the failed Session's persisted state
// classifies its latest turn as a provider session-limit failure.
func sessionLimitCapped(s *session) bool {
	return s != nil && s.meta.Status == "failed" && s.meta.Capped
}
