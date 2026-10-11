package web

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"time"
)

// Compress failures sort by their text. The pig SDK puts a provider HTTP
// status in its error text: at the start, after a colon or an open paren, or
// after "HTTP" or "status". It names network failures with fixed phrases. A
// credit or auth failure blocks the Session. A transient failure retries under
// compressBackoff. Any other failure falls through the fallback chain in
// summarize.go.

// compressBackoff is the wait before each retry of a transient failure: three
// retries, so at most four tries per call.
var compressBackoff = []time.Duration{2 * time.Second, 8 * time.Second, 30 * time.Second}

// compressFailure is the class of one failed compress call.
type compressFailure int

const (
	failOther compressFailure = iota
	failTransient
	failBlock
)

// statusAt is where a provider HTTP status sits in SDK error text.
const statusAt = `(?:^|[:(]\s*|\bHTTP\s*|\bstatus(?:\s+code)?\s*)`

// blockFailure matches credit and auth failures. A status code needs a reason
// or a boundary after it, so a bare number in prose never matches.
var blockFailure = regexp.MustCompile(`(?i)` + statusAt + `(?:401|402)(?:\)|:|\s*$|\s+status\b|\s+(?:unauthorized|payment|invalid))` +
	`|payment required|insufficient[ _-](?:credits?|balance|quota|funds)|(?:no|out of|low) credits?\b|credit balance` +
	`|\bquota\b|billing[ _](?:hard[ _])?limit|billing (?:required|error)|unauthori[sz]ed|authentication|api[ _-]?key`)

// transientFailure matches network, 5xx and rate-limit failures.
var transientFailure = regexp.MustCompile(`(?i)` + statusAt + `(?:429|5\d\d)(?:\)|:|\s*$|\s+status\b|\s+(?:too many|overloaded|internal|service|bad gateway|gateway|unavailable|requests|error))` +
	`|rate[ _-]?limit|too many requests|overloaded|timed out|i/o timeout|connection (?:error|reset|refused|closed)` +
	`|fetch failed|unexpected eof|no such host|network (?:error|is unreachable)|(?:service|server) unavailable|status: unavailable` +
	`|temporar(?:y|ily) (?:unavailable|failure|error|overloaded)|(?:^|: )terminated$`)

// classifyCompressError sorts one failure by its text. Block wins over
// transient: a 429 that names the quota is a credit failure.
func classifyCompressError(err error) compressFailure {
	msg := err.Error()
	switch {
	case blockFailure.MatchString(msg):
		return failBlock
	case transientFailure.MatchString(msg):
		return failTransient
	}
	return failOther
}

// retryTransient runs op. A transient failure waits compressBackoff[n] and
// runs op again, up to len(compressBackoff) retries. It returns the retry
// count and the last error.
func retryTransient(op func() error, wait func(time.Duration)) (int, error) {
	retries := 0
	for {
		err := op()
		if err == nil || classifyCompressError(err) != failTransient || retries >= len(compressBackoff) {
			return retries, err
		}
		wait(compressBackoff[retries])
		retries++
	}
}

// oneShotDeadline is the error of a one-off that failed at its deadline, or nil.
// A deadline is not transient: retrying it would stall the Session for every try.
func oneShotDeadline(ctx context.Context, timeout time.Duration, failed bool) error {
	if !failed || ctx.Err() != context.DeadlineExceeded {
		return nil
	}
	return fmt.Errorf("pig one-off stopped at the %s limit", timeout)
}

// backoffWait is the sleep seam for compress retries: the Daemon's seam when
// set, else time.Sleep.
func (d *Daemon) backoffWait() func(time.Duration) {
	if d.compressSleep != nil {
		return d.compressSleep
	}
	return time.Sleep
}

// blockCompress stops a compress on a credit or auth failure. The old handle
// and its resume token stay as they are. A compress that waits on a Group is
// dropped: a Retry asks again. The Session holds its messages until Retry
// succeeds or the block is dismissed.
func (d *Daemon) blockCompress(id string, ev CompressBlockedEvent) {
	ev.Action = "retry"
	reason := ev.Error
	if reason == "" {
		reason = "compress blocked"
	}
	d.mu.Lock()
	if cur, ok := d.sessions[id]; ok {
		cur.meta.CompressBlock = reason
		cur.compressWait = false
		cur.compressInstr = ""
		d.persistMeta(cur.meta)
	}
	d.mu.Unlock()
	d.appendEvent(id, "compress_blocked", ev.text(), "")
}

// handleCompressDismiss is POST /api/sessions/{id}/compress/dismiss. It clears
// a compress block without a compress: the old handle stays, and the messages
// the block held go on.
func (d *Daemon) handleCompressDismiss(w http.ResponseWriter, r *http.Request, id string) {
	code, out := d.dismissCompress(id)
	if code == http.StatusNotFound {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, code, out)
}

// dismissCompress clears the block of id and drains the held messages. A
// released Session resumes first, as a send does.
func (d *Daemon) dismissCompress(id string) (int, map[string]any) {
	d.mu.Lock()
	s, ok := d.sessions[id]
	if !ok {
		d.mu.Unlock()
		return http.StatusNotFound, nil
	}
	live := s.live
	d.mu.Unlock()
	if !live {
		if code, msg := d.resumeReleased(id); code != 0 {
			return code, map[string]any{"error": msg}
		}
	}
	d.mu.Lock()
	s, ok = d.sessions[id]
	if !ok {
		d.mu.Unlock()
		return http.StatusNotFound, nil
	}
	if s.meta.CompressBlock == "" {
		d.mu.Unlock()
		return http.StatusConflict, map[string]any{"error": "no compress is blocked"}
	}
	if s.busy || s.active > 0 {
		d.mu.Unlock()
		return http.StatusConflict, map[string]any{"error": "session is working; dismiss when it is idle"}
	}
	s.meta.CompressBlock = ""
	d.persistMeta(s.meta)
	// Claim the Session as a turn does, so finishTurn settles it and drains
	// the held messages in order.
	s.busy = true
	s.active++
	d.mu.Unlock()
	d.finishTurn(id)
	return http.StatusOK, map[string]any{"dismissed": true}
}

// retryingSummarizer runs each summarizer call under the transient retry policy
// and counts the retries it made. writeHandoff and its folds call it in place of
// the summarizer.
type retryingSummarizer struct {
	next    handoffSummarizer
	wait    func(time.Duration)
	retries int
}

// Window is the input budget of the next summarizer for model.
func (r *retryingSummarizer) Window(model string) int { return r.next.Window(model) }

// Summarize is one summarizer call, retried while its failure is transient.
func (r *retryingSummarizer) Summarize(model, system, user string) (string, error) {
	var reply string
	n, err := retryTransient(func() error {
		var e error
		reply, e = r.next.Summarize(model, system, user)
		return e
	}, r.wait)
	r.retries += n
	return reply, err
}

// blockedResult is the outcome of a credit or auth failure: no Handoff text,
// and the compress holds.
func blockedResult(err error) handoffResult {
	return handoffResult{blocked: true, err: err.Error()}
}
