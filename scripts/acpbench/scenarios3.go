package main

import (
	"fmt"
	"strings"
	"time"
)

// Provider-fault contract (shared with fakellm2, which another worker is
// extending): the client sends a prompt containing directive text
// 'FAULT:<class>' with class in http429, http500, http529, midstream-error,
// stall-5s, stall-forever, truncate, reset, empty. fakellm2 honors the
// directive by failing that way; the probe classifies how the AGENT
// surfaces the failure to the ACP client:
//
//   - clean error stopReason/message within the turn timeout (good)
//   - silent empty end_turn (bad: silent-empty-turn)
//   - hang past the turn timeout (bad)
//   - crash: agent process gone (bad)
//   - retries then success (good; retry count recorded from stats)
//
// With --fault <name> the scenario runs that class; without it, it skips.
// The in-repo fake agent mirrors the contract so self-tests need no fakellm2.
//
// fakellm2 hint (documented for hand runs): point the agent at fakellm2 via
// --env (e.g. --env BASE_URL={{FAKE_LLM_URL}} --fake-llm-url <fakellm2-url>)
// and pass --fault <class>; fakellm2 answers the FAULT:<class> prompt with
// the matching HTTP failure / stall / truncation. Header/env hint: fakellm2
// also honors X-Acpbench-Fault: <class> and ACPBENCH_FAULT=<class> when the
// prompt directive is stripped by the agent.
func scProviderFault(b *benchCfg) ScenarioResult {
	name := "provider_fault"
	start := time.Now()
	if b.fault == "" {
		return failResult(name, start, VerdictSkipped,
			fmt.Errorf("needs --fault <class> (http429|http500|http529|midstream-error|stall-5s|stall-forever|truncate|reset|empty)"),
			map[string]any{"note": "no fault class requested"})
	}
	class := strings.ToLower(strings.TrimSpace(b.fault))
	col := &collector{}
	c, sid, err := b.startSession(col, PermAutoApprove)
	if err != nil {
		return failResult(name, start, VerdictProtocolError, err, map[string]any{"fault": class})
	}
	defer c.Close()
	prompt := fmt.Sprintf("Trigger provider fault FAULT:%s. Report exactly what the provider returned.", class)
	t0 := time.Now()
	col2 := &collector{}
	c.onUpdate = col2.onUpdate
	stop, perr := c.PromptBlocks(sid, []any{map[string]any{"type": "text", "text": prompt}}, promptTimeout)
	lat := ms(time.Since(t0))
	updates, sawTool, sawChunk, text, _, tools := col2.snapshot()
	details := map[string]any{
		"fault": class, "stopReason": stop, "latencyMs": lat,
		"updates": updates, "sawToolCall": sawTool, "sawChunk": sawChunk,
		"tools": tools, "textBytes": len(text), "text": truncText(text, 500),
		"error": errString(perr),
	}
	if perr != nil {
		if c.exited() {
			details["class"] = "crash"
			return failResult(name, start, VerdictCrash, perr, details)
		}
		if isTimeout(perr) {
			details["class"] = "hang"
			return failResult(name, start, VerdictTimeoutHang, perr, details)
		}
		// Clean error within the timeout: good.
		details["class"] = "clean-error"
		return ScenarioResult{Name: name, Verdict: VerdictOK, LatencyMs: ms(time.Since(start)), Details: details}
	}
	if stop == "cancelled" {
		details["class"] = "cancelled"
		return failResult(name, start, VerdictWrongResult, fmt.Errorf("fault turn cancelled"), details)
	}
	if stop != "end_turn" {
		if isErrorStop(stop, text) {
			details["class"] = "clean-error"
			return ScenarioResult{Name: name, Verdict: VerdictOK, LatencyMs: ms(time.Since(start)), Details: details}
		}
		details["class"] = "unexpected-stop"
		return failResult(name, start, VerdictWrongResult, fmt.Errorf("stopReason %q", stop), details)
	}
	// end_turn: silent-empty is bad; surfaced errors and retried success good.
	if updates == 0 || (!sawTool && !sawChunk) {
		details["class"] = "silent-empty-turn"
		return failResult(name, start, VerdictSilentEmptyTurn, fmt.Errorf("fault surfaced as silent empty end_turn"), details)
	}
	lower := strings.ToLower(text)
	if mentionsProviderError(lower) {
		details["class"] = "surfaced-error"
		return ScenarioResult{Name: name, Verdict: VerdictOK, LatencyMs: ms(time.Since(start)), Details: details}
	}
	if strings.Contains(lower, "retr") {
		details["class"] = "retried-success"
		details["retryCount"] = parseRetryCount(lower, updates)
		return ScenarioResult{Name: name, Verdict: VerdictOK, LatencyMs: ms(time.Since(start)), Details: details}
	}
	if class == "truncate" || class == "truncated" {
		details["class"] = "truncated-unflagged"
		return failResult(name, start, VerdictWrongResult, fmt.Errorf("truncated output returned as clean end_turn"), details)
	}
	if class == "empty" {
		details["class"] = "silent-empty-turn"
		return failResult(name, start, VerdictSilentEmptyTurn, fmt.Errorf("empty fault returned non-empty-looking end_turn without error"), details)
	}
	// Any other content-bearing end_turn after a fault reads as recovered.
	details["class"] = "retried-success"
	details["retryCount"] = parseRetryCount(lower, updates)
	return ScenarioResult{Name: name, Verdict: VerdictOK, LatencyMs: ms(time.Since(start)), Details: details}
}

// isErrorStop treats refused-style stops with error text as clean errors.
func isErrorStop(stop, text string) bool {
	if stop == "refused" || strings.Contains(strings.ToLower(stop), "error") {
		return true
	}
	return mentionsProviderError(strings.ToLower(text))
}

// mentionsProviderError matches agent-surfaced provider failure text.
func mentionsProviderError(lower string) bool {
	for _, m := range []string{"429", "500", "529", "rate limit", "rate-limit", "overload", "internal error", "mid-stream", "midstream", "connection reset", "provider error", "provider fault", "truncat", "stall", "upstream"} {
		if strings.Contains(lower, m) {
			return true
		}
	}
	return false
}

// parseRetryCount reads retries:N from the transcript, else falls back to
// the update count as the retry-activity proxy.
func parseRetryCount(lower string, updates int) int {
	for _, key := range []string{"retries:", "retry:", "retries ", "attempt "} {
		if i := strings.Index(lower, key); i >= 0 {
			var n int
			if _, err := fmt.Sscanf(lower[i+len(key):], "%d", &n); err == nil {
				return n
			}
		}
	}
	return updates
}

func truncText(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
