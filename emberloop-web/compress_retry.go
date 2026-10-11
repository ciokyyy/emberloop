package web

import (
	"net/http"
	"regexp"
	"time"
)

// Compress failures sort by their text. The pig SDK puts the HTTP status or
// the provider error type into provider failure text, and names network
// failures with fixed phrases ("fetch failed", "Connection error.",
// "Request timed out."). A credit or auth failure blocks the Session. A
// transient failure retries under compressBackoff. Any other failure falls
// through the fallback chain in summarize.go.

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

var (
	// blockFailure matches credit and auth failures: 402, 401 and quota.
	blockFailure = regexp.MustCompile(`(?i)\b(401|402)\b|payment required|insufficient|credit|quota|billing|unauthori[sz]ed|authenticat|api[ _-]?key`)
	// transientFailure matches network, 5xx and rate-limit failures.
	transientFailure = regexp.MustCompile(`(?i)\b(429|5\d\d)\b|rate[ _-]?limit|too many requests|overloaded|timed out|timeout|connection|fetch failed|terminated|network|\beof\b|no such host|temporar|unavailable|try again`)
)

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

// backoffWait is the sleep seam for compress retries: the Daemon's seam when
// set, else time.Sleep.
func (d *Daemon) backoffWait() func(time.Duration) {
	if d.compressSleep != nil {
		return d.compressSleep
	}
	return time.Sleep
}

// blockCompress stops a compress on a credit or auth failure. The old handle
// and its resume token stay as they are. The Session holds its messages until
// Retry succeeds or the block is dismissed.
func (d *Daemon) blockCompress(id string, ev CompressBlockedEvent) {
	ev.Action = "retry"
	reason := ev.Error
	if reason == "" {
		reason = "compress blocked"
	}
	d.mu.Lock()
	if cur, ok := d.sessions[id]; ok {
		cur.meta.CompressBlock = reason
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
