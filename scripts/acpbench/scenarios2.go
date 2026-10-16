package main

import (
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
	"time"
)

// slowPrompt asks for the stub slow tool, holding a turn open when the agent
// cooperates. The fake answers it only with --slow-ms; a fast answer means
// the turn raced shut before we could probe it.
const slowPrompt = "Call the slow tool with ms 15000, then answer done."

// promptRes is one finished prompt turn.
type promptRes struct {
	stop string
	err  error
}

// turnRace reports whether the turn finished before d elapsed, returning the
// turn result with it: the receive is the observation, so callers must not
// read done again on the raced path.
func turnRace(done <-chan promptRes, d time.Duration) (promptRes, bool) {
	select {
	case r := <-done:
		return r, true
	case <-time.After(d):
		return promptRes{}, false
	}
}

// --- scenario 4: cancel ---

func scCancel(b *benchCfg) ScenarioResult {
	name := "cancel"
	start := time.Now()
	col := &collector{}
	c, sid, err := b.startSession(col, PermAutoApprove)
	if err != nil {
		return failResult(name, start, VerdictProtocolError, err, nil)
	}
	defer c.Close()
	done := make(chan promptRes, 1)
	p0 := time.Now()
	slow := b.echoPromptFor(true)
	go func() {
		stop, err := c.Prompt(sid, slow)
		done <- promptRes{stop, err}
	}()
	if r, raced := turnRace(done, raceWait); raced {
		_ = r
		return failResult(name, start, VerdictSkipped, fmt.Errorf("turn finished before cancel; need a slow turn (fake-agent --slow-ms)"), map[string]any{"raced": true})
	}
	updatesBefore, sawToolBefore, _, _, _, _ := col.snapshot()
	phase := "streaming"
	if sawToolBefore {
		phase = "slow-tool"
	}
	_ = c.Cancel(sid)
	b.markPhase("prompt-turn", p0, time.Now())
	select {
	case r := <-done:
		details := map[string]any{
			"stopReason": r.stop, "cancelPhase": phase, "updatesBeforeCancel": updatesBefore,
		}
		if r.err != nil {
			return failResult(name, start, VerdictProtocolError, r.err, details)
		}
		if r.stop != "cancelled" {
			return failResult(name, start, VerdictWrongResult, fmt.Errorf("stopReason %q, want cancelled", r.stop), details)
		}
		// The session must stay reusable after a cancel.
		col2 := &collector{}
		c.onUpdate = col2.onUpdate
		stop2, err2 := c.Prompt(sid, "Say hi.")
		details["reuseStop"] = stop2
		if err2 != nil || stop2 != "end_turn" {
			details["reuseErr"] = errString(err2)
			return failResult(name, start, VerdictWrongResult, fmt.Errorf("session not reusable after cancel"), details)
		}
		return ScenarioResult{Name: name, Verdict: VerdictOK, LatencyMs: ms(time.Since(start)), Details: details}
	case <-time.After(cancelWait):
		details := map[string]any{"cancelPhase": phase, "updatesBeforeCancel": updatesBefore}
		return failResult(name, start, VerdictTimeoutHang, fmt.Errorf("prompt did not return cancelled within %s", cancelWait), details)
	}
}

// --- scenario 5: steer_followup ---

func scSteer(b *benchCfg) ScenarioResult {
	name := "steer_followup"
	start := time.Now()
	col := &collector{}
	c, sid, err := b.startSession(col, PermAutoApprove)
	if err != nil {
		return failResult(name, start, VerdictProtocolError, err, nil)
	}
	defer c.Close()
	slow := b.echoPromptFor(true)
	done := make(chan promptRes, 1)
	go func() {
		stop, err := c.Prompt(sid, slow)
		done <- promptRes{stop, err}
	}()
	if _, raced := turnRace(done, raceWait); raced {
		return failResult(name, start, VerdictSkipped, fmt.Errorf("turn finished before steer; need a slow turn (fake-agent --slow-ms)"), map[string]any{"raced": true})
	}
	if c.steeringSupported() {
		outcome, serr := c.Steer(sid, "follow-up: hurry up")
		details := map[string]any{"steeringAdvertised": true, "outcome": outcome}
		_ = c.Cancel(sid)
		select {
		case <-done:
		case <-time.After(cancelWait):
		}
		if serr != nil {
			if isTimeout(serr) {
				return failResult(name, start, VerdictTimeoutHang, serr, details)
			}
			return failResult(name, start, VerdictProtocolError, serr, details)
		}
		if outcome != "injected" {
			details["note"] = "steer answered but did not inject"
		}
		return ScenarioResult{Name: name, Verdict: VerdictOK, LatencyMs: ms(time.Since(start)), Details: details}
	}
	// No steering: probe a second concurrent prompt.
	type sres struct {
		stop string
		err  error
		at   time.Time
	}
	second := make(chan sres, 1)
	go func() {
		stop, err := c.PromptBlocks(sid, []any{map[string]any{"type": "text", "text": "second prompt"}}, steerWait)
		second <- sres{stop, err, time.Now()}
	}()
	_ = c.Cancel(sid)
	firstDone := time.Time{}
	select {
	case <-done:
		firstDone = time.Now()
	case <-time.After(cancelWait + 5*time.Second):
	}
	select {
	case r := <-second:
		details := map[string]any{"steeringAdvertised": false, "secondStop": r.stop}
		if r.err != nil {
			if strings.Contains(r.err.Error(), "already running") || strings.Contains(r.err.Error(), "busy") {
				details["outcome"] = "rejected"
				return ScenarioResult{Name: name, Verdict: VerdictOK, LatencyMs: ms(time.Since(start)), Details: details}
			}
			if isTimeout(r.err) {
				details["outcome"] = "hang"
				return failResult(name, start, VerdictTimeoutHang, r.err, details)
			}
			details["outcome"] = "rejected"
			details["rejectErr"] = r.err.Error()
			return ScenarioResult{Name: name, Verdict: VerdictOK, LatencyMs: ms(time.Since(start)), Details: details}
		}
		if !firstDone.IsZero() && r.at.After(firstDone) {
			details["outcome"] = "queued"
		} else {
			details["outcome"] = "concurrent"
		}
		return ScenarioResult{Name: name, Verdict: VerdictOK, LatencyMs: ms(time.Since(start)), Details: details}
	case <-time.After(steerWait + 5*time.Second):
		return failResult(name, start, VerdictTimeoutHang, fmt.Errorf("second prompt hung"), map[string]any{"steeringAdvertised": false, "outcome": "hang"})
	}
}

// --- scenario 6: load_resume (three legs) ---

func scLoadResume(b *benchCfg) ScenarioResult {
	name := "load_resume"
	start := time.Now()
	details := map[string]any{}
	// Leg 1: session/resume when the agent answers it (same process).
	// Leg 2: session/load in the SAME process. Leg 3: session/load in a fresh
	// process after kill+respawn (cross-process). Each leg reports separately.
	col := &collector{}
	c, sid, err := b.startSession(col, PermAutoApprove)
	if err != nil {
		return failResult(name, start, VerdictProtocolError, err, nil)
	}
	details["sessionId"] = sid
	if _, err := c.Prompt(sid, "Remember turn one."); err != nil {
		c.Close()
		return failResult(name, start, VerdictProtocolError, err, map[string]any{"phase": "first-prompt"})
	}
	legOK := map[string]bool{}
	// Leg 1: session/resume in the live session.
	t1 := time.Now()
	if _, rerr := c.callTimeout("session/resume", sessionParams(b.cwd, b.mcpServers(), b.sessionMeta(), sid), ctlTimeout); rerr == nil {
		details["resume"] = map[string]any{"verdict": "used", "latencyMs": ms(time.Since(t1))}
		legOK["resume"] = true
	} else if isMethodNotFound(rerr) {
		details["resume"] = map[string]any{"verdict": "unsupported", "latencyMs": ms(time.Since(t1))}
		legOK["resume"] = true // absent method is not a failure
	} else {
		details["resume"] = map[string]any{"verdict": "failed", "error": rerr.Error(), "latencyMs": ms(time.Since(t1))}
	}
	// Leg 2: session/load same-process.
	t2 := time.Now()
	if raw, lerr := c.LoadSessionRaw(sid, b.cwd, b.mcpServers(), b.sessionMeta()); lerr != nil {
		details["loadSameProcess"] = map[string]any{"verdict": "failed", "error": lerr.Error(), "latencyMs": ms(time.Since(t2))}
	} else {
		d := discoverModels(raw)
		details["loadSameProcess"] = map[string]any{"verdict": "ok", "latencyMs": ms(time.Since(t2)), "configKeys": d.RawKeys}
		legOK["same"] = true
	}
	colA := &collector{}
	c.onUpdate = colA.onUpdate
	stopA, errA := c.Prompt(sid, "history-check please")
	_, _, _, textA, _, _ := colA.snapshot()
	details["sameProcessCheck"] = map[string]any{"stopReason": stopA, "error": errString(errA), "history": textA}
	if errA != nil || stopA != "end_turn" {
		details["loadSameProcess"] = map[string]any{"verdict": "failed", "error": fmt.Sprintf("verify turn: stop=%q err=%v", stopA, errA)}
		delete(legOK, "same")
	}
	c.Kill()
	c.Close()
	// Leg 3: kill + respawn + session/load cross-process.
	c2, err := b.spawnConn()
	if err != nil {
		return failResult(name, start, VerdictCrash, err, details)
	}
	defer c2.Close()
	if err := c2.Initialize(); err != nil {
		return failResult(name, start, VerdictProtocolError, err, details)
	}
	c2.permMode = PermAutoApprove
	t3 := time.Now()
	if _, lerr := c2.LoadSessionRaw(sid, b.cwd, b.mcpServers(), b.sessionMeta()); lerr != nil {
		details["loadCrossProcess"] = map[string]any{"verdict": "failed", "error": lerr.Error(), "latencyMs": ms(time.Since(t3))}
		return failResult(name, start, VerdictProtocolError, lerr, details)
	}
	details["loadCrossProcess"] = map[string]any{"verdict": "loaded", "latencyMs": ms(time.Since(t3))}
	col2 := &collector{}
	c2.onUpdate = col2.onUpdate
	stop, err := c2.Prompt(sid, "history-check please")
	_, _, _, text, _, _ := col2.snapshot()
	details["stopReason"] = stop
	if err != nil {
		return failResult(name, start, VerdictWrongResult, err, details)
	}
	if strings.Contains(text, "history:") {
		details["history"] = text
	}
	if stop != "end_turn" {
		return failResult(name, start, VerdictWrongResult, fmt.Errorf("stopReason %q after load", stop), details)
	}
	legOK["cross"] = true
	details["legs"] = map[string]any{"resume": legStatus(details, "resume"), "loadSameProcess": legStatus(details, "loadSameProcess"), "loadCrossProcess": "ok"}
	if !legOK["cross"] || !legOK["same"] {
		return failResult(name, start, VerdictWrongResult, fmt.Errorf("load legs failed"), details)
	}
	return ScenarioResult{Name: name, Verdict: VerdictOK, LatencyMs: ms(time.Since(start)), Details: details}
}

func legStatus(details map[string]any, key string) string {
	if m, ok := details[key].(map[string]any); ok {
		if v, _ := m["verdict"].(string); v != "" {
			return v
		}
	}
	return "unknown"
}

// --- scenario 7: set_model / set_config_option (real ids only) ---

func scSetModel(b *benchCfg) ScenarioResult {
	name := "set_model_config"
	start := time.Now()
	col := &collector{}
	c, err := b.spawnConn()
	if err != nil {
		return failResult(name, start, VerdictCrash, err, nil)
	}
	defer c.Close()
	c.permMode = PermAutoApprove
	c.onUpdate = col.onUpdate
	if err := c.Initialize(); err != nil {
		return failResult(name, start, VerdictProtocolError, err, nil)
	}
	sid, newRaw, err := c.NewSessionRaw(b.cwd, b.mcpServers(), b.sessionMeta())
	if err != nil {
		return failResult(name, start, VerdictProtocolError, err, nil)
	}
	disc := discoverModels(newRaw)
	details := map[string]any{
		"discoveredModels": disc.Models, "currentModel": disc.Current,
		"newResultKeys": disc.RawKeys,
	}
	optIDs := []string{}
	for _, o := range disc.Options {
		optIDs = append(optIDs, o.ID)
	}
	details["configOptions"] = optIDs
	configID, value, modelID := modelConfigTarget(disc)
	details["target"] = map[string]any{"configId": configID, "value": value, "modelId": modelID}
	if value == "" && modelID == "" {
		details["note"] = "agent advertised no usable model id; not probing with a bogus id"
		return failResult(name, start, VerdictSkipped, fmt.Errorf("no discoverable model id"), details)
	}
	// Prefer the real config-option path; fall back to set_model(modelId).
	setVia := ""
	setErr := error(nil)
	if configID != "" {
		setErr = c.SetConfigOption(sid, configID, value)
		if setErr == nil {
			setVia = "set_config_option"
			details["set_config_option"] = "applied:" + configID + "=" + value
		} else if isMethodNotFound(setErr) {
			details["set_config_option"] = "method-absent: " + setErr.Error()
		} else {
			details["set_config_option"] = "id-rejected: " + setErr.Error()
		}
	} else {
		details["set_config_option"] = "no-model-option"
	}
	if setVia == "" && modelID != "" {
		if merr := c.SetModel(sid, modelID); merr != nil {
			if isMethodNotFound(merr) {
				details["set_model"] = "method-absent: " + merr.Error()
			} else {
				details["set_model"] = "id-rejected: " + merr.Error()
			}
			if setErr == nil && configID != "" {
				setErr = merr
			} else if configID == "" {
				setErr = merr
			}
		} else {
			setVia = "set_model"
			setErr = nil
			details["set_model"] = "applied:modelId=" + modelID
		}
	}
	if setVia == "" {
		details["verified"] = "neither method applied a real id"
		if setErr != nil {
			return failResult(name, start, VerdictWrongResult, setErr, details)
		}
		return failResult(name, start, VerdictWrongResult, fmt.Errorf("no method applied"), details)
	}
	want := value
	if want == "" {
		want = modelID
	}
	stop, perr := c.Prompt(sid, "Answer with your current model name.")
	_, _, _, text, _, _ := col.snapshot()
	details["stopReason"] = stop
	details["via"] = setVia
	if perr != nil {
		return failResult(name, start, VerdictProtocolError, perr, details)
	}
	if want != "" && strings.Contains(text, want) {
		details["verified"] = "applied-and-verified: next turn names " + want
	} else {
		details["verified"] = "applied-unverified: next turn does not name " + want
	}
	return ScenarioResult{Name: name, Verdict: VerdictOK, LatencyMs: ms(time.Since(start)), Details: details}
}

// --- scenario 8: images ---

// onePixelPNG is a 1x1 transparent PNG.
const onePixelPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="

func scImages(b *benchCfg) ScenarioResult {
	name := "images"
	start := time.Now()
	col := &collector{}
	c, sid, err := b.startSession(col, PermAutoApprove)
	if err != nil {
		return failResult(name, start, VerdictProtocolError, err, nil)
	}
	defer c.Close()
	if _, derr := base64.StdEncoding.DecodeString(onePixelPNG); derr != nil {
		return failResult(name, start, VerdictProtocolError, derr, nil)
	}
	stop, perr := c.PromptImages(sid, "Describe this image.", []testImage{{Mime: "image/png", Data: onePixelPNG}})
	updates, sawTool, sawChunk, text, _, _ := col.snapshot()
	details := map[string]any{
		"stopReason": stop, "updates": updates, "textBytes": len(text),
	}
	if perr != nil {
		details["mode"] = "error"
		return failResult(name, start, VerdictProtocolError, perr, details)
	}
	_ = sawTool
	if strings.Contains(text, "dropped:image") {
		details["mode"] = "dropped"
		return failResult(name, start, VerdictWrongResult, fmt.Errorf("agent dropped the image block"), details)
	}
	if !sawChunk || updates == 0 {
		details["mode"] = "dropped"
		return failResult(name, start, VerdictWrongResult, fmt.Errorf("image answered with no content"), details)
	}
	details["mode"] = "accepted"
	return ScenarioResult{Name: name, Verdict: VerdictOK, LatencyMs: ms(time.Since(start)), Details: details}
}

// --- scenario 9: permission ---

func scPermission(b *benchCfg) ScenarioResult {
	name := "permission"
	start := time.Now()
	details := map[string]any{}
	prompt := b.echoPromptFor(false)
	// Approve leg.
	col := &collector{}
	c, sid, err := b.startSession(col, PermAutoApprove)
	if err != nil {
		return failResult(name, start, VerdictProtocolError, err, nil)
	}
	_, echoBefore, _ := b.stubEchoCalls()
	stop, perr := c.Prompt(sid, prompt)
	_, sawTool, _, _, _, _ := col.snapshot()
	col.mu.Lock()
	asked := len(col.perms) > 0
	perms := append([]string(nil), col.perms...)
	col.mu.Unlock()
	c.Close()
	details["approveAsked"] = asked
	details["approvePerms"] = perms
	details["approveStop"] = stop
	if !asked {
		details["note"] = "agent asked no permission; rerun against fake-agent --ask-permission"
		return failResult(name, start, VerdictSkipped, fmt.Errorf("no permission request observed"), details)
	}
	if perr != nil || !sawTool {
		return failResult(name, start, VerdictWrongResult, fmt.Errorf("approved turn failed: stop=%q err=%v", stop, perr), details)
	}
	// Deny leg: stub-side execution count must not grow. Optimistic
	// tool_call updates are not proof; the stub log is.
	_, echoAfterApprove, _ := b.stubEchoCalls()
	details["approveStubEchoCalls"] = echoAfterApprove - echoBefore
	col2 := &collector{}
	c2, sid2, err := b.startSession(col2, PermDeny)
	if err != nil {
		return failResult(name, start, VerdictProtocolError, err, details)
	}
	stop2, perr2 := c2.Prompt(sid2, prompt)
	_, sawTool2, _, text2, _, _ := col2.snapshot()
	c2.Close()
	_, echoAfterDeny, _ := b.stubEchoCalls()
	details["denyStop"] = stop2
	details["denyErr"] = errString(perr2)
	details["denySawTool"] = sawTool2
	details["denyStubEchoCalls"] = echoAfterDeny - echoAfterApprove
	details["denyStubVerified"] = echoAfterDeny == echoAfterApprove
	if b.mcpStub {
		if echoAfterDeny != echoAfterApprove {
			return failResult(name, start, VerdictWrongResult, fmt.Errorf("denied tool still ran stub-side"), details)
		}
	} else if sawTool2 {
		return failResult(name, start, VerdictWrongResult, fmt.Errorf("denied tool still ran"), details)
	}
	if !strings.Contains(text2, "denied") {
		details["note"] = "denial not acknowledged in transcript"
	}
	return ScenarioResult{Name: name, Verdict: VerdictOK, LatencyMs: ms(time.Since(start)), Details: details}
}

// --- scenario 10: crash_recovery ---

func scCrash(b *benchCfg) ScenarioResult {
	name := "crash_recovery"
	start := time.Now()
	col := &collector{}
	c, sid, err := b.startSession(col, PermAutoApprove)
	if err != nil {
		return failResult(name, start, VerdictProtocolError, err, nil)
	}
	defer c.Close()
	slow := b.echoPromptFor(true)
	done := make(chan promptRes, 1)
	p0 := time.Now()
	go func() {
		stop, err := c.Prompt(sid, slow)
		done <- promptRes{stop, err}
	}()
	if _, raced := turnRace(done, raceWait); raced {
		return failResult(name, start, VerdictSkipped, fmt.Errorf("turn finished before kill; need a slow turn (fake-agent --slow-ms)"), map[string]any{"raced": true})
	}
	c.Kill() // SIGKILL mid-turn.
	b.markPhase("prompt-turn", p0, time.Now())
	select {
	case r := <-done:
		details := map[string]any{
			"stopReason":  r.stop,
			"detectionMs": ms(time.Since(p0)) - ms(raceWait),
		}
		if r.err == nil {
			return failResult(name, start, VerdictWrongResult, fmt.Errorf("prompt returned ok after SIGKILL"), details)
		}
		return ScenarioResult{Name: name, Verdict: VerdictOK, LatencyMs: ms(time.Since(start)), Details: details}
	case <-time.After(crashDetectWait):
		return failResult(name, start, VerdictTimeoutHang, fmt.Errorf("client hung after SIGKILL for %s", crashDetectWait), nil)
	}
}

// statPhase merges every recorded window of a phase into one summary.
func (b *benchCfg) statPhase(name string) phaseStat {
	b.mu.Lock()
	windows := append([]memWindow(nil), b.phases[name]...)
	b.mu.Unlock()
	var st phaseStat
	for _, w := range windows {
		s := b.mem.statWindow(w.start, w.end)
		st.Samples += s.Samples
		st.LastRSS = s.LastRSS
		if s.MaxRSS > st.MaxRSS {
			st.MaxRSS = s.MaxRSS
		}
		if s.MaxProcs > st.MaxProcs {
			st.MaxProcs = s.MaxProcs
		}
	}
	return st
}

// --- scenario 11: concurrency ---

func scConcurrency(b *benchCfg) ScenarioResult {
	name := "concurrency"
	start := time.Now()
	n := b.sessions
	if n < 1 {
		n = 1
	}
	details := map[string]any{"sessions": n}
	// Part A: N sessions in N processes, prompted in parallel.
	type ares struct {
		stop string
		err  error
	}
	a0 := time.Now()
	results := make([]ares, n)
	var wg sync.WaitGroup
	conns := make([]*acpConn, n)
	sids := make([]string, n)
	okA := true
	for i := 0; i < n; i++ {
		col := &collector{}
		c, sid, err := b.startSession(col, PermAutoApprove)
		if err != nil {
			okA = false
			details[fmt.Sprintf("proc%d", i)] = "start: " + err.Error()
			continue
		}
		conns[i] = c
		sids[i] = sid
	}
	for i := 0; i < n; i++ {
		if conns[i] == nil {
			continue
		}
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			col := &collector{}
			conns[i].onUpdate = col.onUpdate
			stop, err := conns[i].Prompt(sids[i], b.echoPromptFor(false))
			results[i] = ares{stop, err}
		}(i)
	}
	wg.Wait()
	b.markPhase("concurrency-multi", a0, time.Now())
	for _, c := range conns {
		if c != nil {
			c.Close()
		}
	}
	okCount := 0
	for i, r := range results {
		if conns[i] == nil {
			continue
		}
		if r.err == nil && r.stop == "end_turn" {
			okCount++
		} else {
			okA = false
			details[fmt.Sprintf("proc%d", i)] = fmt.Sprintf("stop=%q err=%v", r.stop, r.err)
		}
	}
	details["multiProcessOk"] = okCount
	// Part B: N sessions inside one process, prompted sequentially.
	c, err := b.spawnConn()
	if err != nil {
		return failResult(name, start, VerdictCrash, err, details)
	}
	defer c.Close()
	if err := c.Initialize(); err != nil {
		return failResult(name, start, VerdictProtocolError, err, details)
	}
	singleOK := 0
	for i := 0; i < n; i++ {
		col := &collector{}
		c.onUpdate = col.onUpdate
		sid, serr := c.NewSession(b.cwd, b.mcpServers(), b.sessionMeta())
		if serr != nil {
			details[fmt.Sprintf("sess%d", i)] = "new: " + serr.Error()
			continue
		}
		stop, perr := c.Prompt(sid, b.echoPromptFor(false))
		if perr == nil && stop == "end_turn" {
			singleOK++
		} else {
			details[fmt.Sprintf("sess%d", i)] = fmt.Sprintf("stop=%q err=%v", stop, perr)
		}
	}
	details["singleProcessOk"] = singleOK
	if !okA || singleOK != n {
		return failResult(name, start, VerdictProtocolError, fmt.Errorf("multi=%d/%d single=%d/%d", okCount, n, singleOK, n), details)
	}
	// Per-session marginal RAM over the multi-process peak.
	idle := b.statPhase("idle-after-new-session")
	peak := b.statPhase("concurrency-multi")
	if idle.LastRSS > 0 && peak.MaxRSS > 0 && n > 0 {
		m := (peak.MaxRSS - idle.LastRSS) / int64(n)
		if m < 0 {
			m = 0
		}
		b.mu.Lock()
		b.marginal = &m
		b.mu.Unlock()
		details["perSessionMarginalBytes"] = m
	}
	return ScenarioResult{Name: name, Verdict: VerdictOK, LatencyMs: ms(time.Since(start)), Details: details}
}

// --- scenario 12: soak ---

func scSoak(b *benchCfg) ScenarioResult {
	name := "soak"
	start := time.Now()
	k := b.iterations
	if k < 1 {
		k = 1
	}
	col := &collector{}
	c, sid, err := b.startSession(col, PermAutoApprove)
	if err != nil {
		return failResult(name, start, VerdictProtocolError, err, nil)
	}
	defer c.Close()
	// RAM phases even on soak-only runs: an idle baseline plus per-turn
	// windows, so the memory section is never empty for soak.
	s0 := time.Now()
	time.Sleep(settleWait)
	b.markPhase("idle-after-new-session", s0, time.Now())
	var lats []float64
	okCount, hangCount, wrongCount, cancelCount := 0, 0, 0, 0
	iters := []map[string]any{}
	for i := 0; i < k; i++ {
		// --cancel-mix N: every Nth iteration runs the SLOW directive and
		// cancels mid-stream instead of a plain echo turn.
		if b.cancelMix > 0 && (i+1)%b.cancelMix == 0 {
			rec := b.soakCancelIter(c, sid, i)
			iters = append(iters, rec)
			lats = append(lats, toFloat(rec["latencyMs"]))
			switch rec["class"] {
			case "cancelled":
				cancelCount++
			case "hang":
				hangCount++
			default:
				wrongCount++
			}
			if c.exited() {
				break
			}
			continue
		}
		ti := &collector{}
		c.onUpdate = ti.onUpdate
		t0 := time.Now()
		t00 := t0
		stop, perr := c.Prompt(sid, b.echoPromptFor(false))
		lat := float64(ms(time.Since(t00)))
		b.markPhase("prompt-turn", t0, time.Now())
		updates, sawTool, sawChunk, _, _, tools := ti.snapshot()
		sawEcho := colSawBaseTool(ti, "echo")
		o := turnOutcome{err: perr, stopReason: stop, updates: updates,
			sawToolCall: sawEcho || sawTool, sawChunk: sawChunk, expectTool: true}
		if perr != nil {
			o.timedOut = isTimeout(perr)
			o.crashed = c.exited()
		}
		v := classifyTurn(o)
		if perr == nil && stop != "end_turn" && v == VerdictOK {
			// A non-end_turn stop is a wrong result even when updates look fine.
			v = VerdictWrongResult
		}
		class := v
		if perr == nil && stop == "end_turn" {
			class = "end_turn"
		}
		lats = append(lats, lat)
		iters = append(iters, map[string]any{
			"i": i, "latencyMs": lat, "stopReason": stop,
			"class": class, "updates": updates, "tools": tools, "error": errString(perr),
		})
		// successRate counts end_turn as success: an agent that answers
		// every turn cleanly must score 1.0 even when update shapes vary.
		if perr == nil && stop == "end_turn" {
			okCount++
		} else if v == VerdictTimeoutHang || v == VerdictCrash {
			hangCount++
		} else {
			wrongCount++
		}
		if c.exited() {
			break
		}
	}
	post0 := time.Now()
	time.Sleep(300 * time.Millisecond)
	b.markPhase("post-turn", post0, time.Now())
	details := map[string]any{
		"iterations": k, "ok": okCount, "hangs": hangCount, "wrong": wrongCount,
		"cancelled": cancelCount, "cancelMix": b.cancelMix, "faultRate": b.faultRate,
		"successRate": float64(okCount) / float64(k),
		"p50Ms":       percentile(lats, 50), "p95Ms": percentile(lats, 95),
		"iters": iters,
	}
	verdict := VerdictOK
	var verr error
	if hangCount > 0 {
		verdict = VerdictTimeoutHang
		verr = fmt.Errorf("%d/%d turns hung", hangCount, k)
	} else if wrongCount > 0 {
		verdict = VerdictWrongResult
		verr = fmt.Errorf("%d/%d turns wrong", wrongCount, k)
	}
	r := ScenarioResult{Name: name, Verdict: verdict, LatencyMs: ms(time.Since(start)), Details: details}
	if verr != nil {
		r.Error = verr.Error()
	}
	return r
}

// soakCancelIter runs one cancel-mix iteration: SLOW directive mid-stream
// cancel, class cancelled/hang/wrong.
func (b *benchCfg) soakCancelIter(c *acpConn, sid string, i int) map[string]any {
	t0 := time.Now()
	done := make(chan promptRes, 1)
	slow := b.echoPromptFor(true)
	go func() {
		stop, err := c.Prompt(sid, slow)
		done <- promptRes{stop, err}
	}()
	if r, raced := turnRace(done, raceWait); raced {
		return map[string]any{"i": i, "latencyMs": float64(ms(time.Since(t0))),
			"stopReason": r.stop, "class": "no-slow-turn", "error": errString(r.err)}
	}
	_ = c.Cancel(sid)
	select {
	case r := <-done:
		class := "wrong"
		if r.err == nil && r.stop == "cancelled" {
			class = "cancelled"
		} else if r.err != nil && isTimeout(r.err) {
			class = "hang"
		}
		return map[string]any{"i": i, "latencyMs": float64(ms(time.Since(t0))),
			"stopReason": r.stop, "class": class, "error": errString(r.err)}
	case <-time.After(cancelWait):
		return map[string]any{"i": i, "latencyMs": float64(ms(time.Since(t0))),
			"stopReason": "", "class": "hang", "error": "cancel-mix turn hung"}
	}
}

func toFloat(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case int64:
		return float64(t)
	case int:
		return float64(t)
	}
	return 0
}
func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
