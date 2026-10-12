package web

import "testing"

// The shared classifier labels one provider failure as quota, credit, auth,
// session-limit or other. The texts are forms the pig SDK puts into its
// errors; the negatives are prose that must stay other.
func TestClassifyFailure(t *testing.T) {
	t.Parallel()
	cases := []struct {
		text string
		want failureClass
	}{
		{"429 quota exceeded for this key", failureQuota},
		{"insufficient_quota", failureQuota},
		{"quota exhausted, try again tomorrow", failureQuota},
		{"402 status code (no body)", failureCredit},
		{"pig one-off: 402 Payment Required", failureCredit},
		{"no credits left on this key", failureCredit},
		{"billing hard limit reached", failureCredit},
		{"anthropic: HTTP 401: authentication_error: invalid x-api-key", failureAuth},
		{"401 Unauthorized", failureAuth},
		{"session limit reached for this account", failureSessionLimit},
		{"too many sessions for this key", failureSessionLimit},
		{"limit-capped: start a new session", failureSessionLimit},
		{"Mistral API error (429): Too Many Requests", failureOther},
		{"pig one-off: 503 Service Unavailable", failureOther},
		{"Connection error.", failureOther},
		{"OpenAI API error (403): forbidden", failureOther},
		{`400: {"message":"invalid request"}`, failureOther},
		{"pig one-off stopped at the 5m0s limit", failureOther},
		{"bad request: malformed input", failureOther},
		{"context 500 tokens", failureOther},
		{"max output 401 tokens", failureOther},
		{"insufficient room in the context window", failureOther},
		{"credit card form field is invalid", failureOther},
		{"connection pool size 5", failureOther},
		{"retry in 500ms", failureOther},
	}
	for _, c := range cases {
		if got := classifyFailure(errString(c.text)); got != c.want {
			t.Errorf("%q: class %q, want %q", c.text, got, c.want)
		}
	}
	if got := classifyFailure(nil); got != failureOther {
		t.Errorf("nil: class %q, want other", got)
	}
}

// Only the four advancing classes move the Run chain; anything else surfaces.
func TestFailureAdvancesChain(t *testing.T) {
	t.Parallel()
	for _, c := range []failureClass{failureQuota, failureCredit, failureAuth, failureSessionLimit} {
		if !c.advancesChain() {
			t.Errorf("%q: advancesChain = false, want true", c)
		}
	}
	if failureOther.advancesChain() {
		t.Error("other: advancesChain = true, want false")
	}
}

// The compress block path shares the classifier: every failure the old block
// matcher caught advances the chain, and every transient or other failure
// does not.
func TestCompressBlockMatchesChainAdvancing(t *testing.T) {
	t.Parallel()
	block := []string{
		"402 status code (no body)",
		"anthropic: HTTP 401: authentication_error: invalid x-api-key",
		"429 quota exceeded for this key",
		"insufficient_quota",
		"no credits left on this key",
		"pig one-off: 402 Payment Required",
	}
	for _, text := range block {
		err := errString(text)
		if classifyCompressError(err) != failBlock {
			t.Errorf("%q: compress class is not block", text)
		}
		if c := classifyFailure(err); !c.advancesChain() {
			t.Errorf("%q: chain class %q does not advance", text, c)
		}
	}
	noAdvance := []string{
		"Mistral API error (429): Too Many Requests",
		"anthropic: HTTP 529: overloaded_error: busy",
		"got status: UNAVAILABLE. {}",
		"pig one-off: 503 Service Unavailable",
		"Connection error.",
		"connection reset by peer",
		"fetch failed",
		"Request timed out.",
		"OpenAI API error (403): forbidden",
		`400: {"message":"invalid request"}`,
		"pig one-off stopped at the 5m0s limit",
		"bad request: malformed input",
		"context 500 tokens",
		"max output 401 tokens",
		"insufficient room in the context window",
		"credit card form field is invalid",
		"connection pool size 5",
		"retry in 500ms",
	}
	for _, text := range noAdvance {
		if c := classifyFailure(errString(text)); c.advancesChain() {
			t.Errorf("%q: chain class %q advances, want other", text, c)
		}
	}
}
