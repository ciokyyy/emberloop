package web

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"example.com/emberloop-web/memory"
)

// Memory Deciders: the miner's heuristic pass always proposes candidates,
// and a confirming Decider judges them. Modes mirror the spec: heuristic
// (default, no network), llm (a one-off pig call with no tools), ensemble
// (the heuristic proposes, the llm confirms against the thresholds). The
// jev Decider lands in ticket 21.

// Decider modes.
const (
	deciderHeuristic = "heuristic"
	deciderLLM       = "llm"
	deciderEnsemble  = "ensemble"
)

// deciderMaxTokens caps one Decider reply; deciderTimeout bounds one call.
const (
	deciderMaxTokens = 1024
	deciderTimeout   = 2 * time.Minute
)

// deciderSettings are the knobs stored in the settings file. They are
// host-global: only the admin writes them, like signup_enabled.
type deciderSettings struct {
	Mode  string  `json:"decider_mode"`
	Model string  `json:"memory_model"`
	Low   float64 `json:"decider_confirm_low"`
	High  float64 `json:"decider_confirm_high"`
}

// readDeciderSettings reads the settings file on every call, so a change
// applies to the next mine without a restart.
func readDeciderSettings() deciderSettings {
	s := deciderSettings{Mode: deciderHeuristic, Low: memory.DefaultConfirmLow, High: memory.DefaultConfirmHigh}
	all, err := readSettingsMap()
	if err != nil {
		return s
	}
	if raw, ok := all["decider_mode"]; ok {
		var str string
		if json.Unmarshal(raw, &str) == nil {
			if str == deciderHeuristic || str == deciderLLM || str == deciderEnsemble {
				s.Mode = str
			}
		}
	}
	if raw, ok := all["memory_model"]; ok {
		var str string
		if json.Unmarshal(raw, &str) == nil {
			s.Model = strings.TrimSpace(str)
		}
	}
	if raw, ok := all["decider_confirm_low"]; ok {
		var f float64
		if json.Unmarshal(raw, &f) == nil && f >= 0 && f <= 1 {
			s.Low = f
		}
	}
	if raw, ok := all["decider_confirm_high"]; ok {
		var f float64
		if json.Unmarshal(raw, &f) == nil && f >= 0 && f <= 1 {
			s.High = f
		}
	}
	if s.Low > s.High {
		s.Low, s.High = memory.DefaultConfirmLow, memory.DefaultConfirmHigh
	}
	return s
}

// deciderNote names why the confirming Decider cannot run as configured, or
// empty when it can: llm and ensemble need an authed memory model, else they
// fall back to the heuristic, and crossed thresholds fall back to the
// defaults.
func deciderNote(set deciderSettings) string {
	var notes []string
	if lo, hi, ok := rawDeciderThresholds(); ok && lo > hi {
		notes = append(notes, fmt.Sprintf("decider_confirm_low %v exceeds decider_confirm_high %v: using the defaults", lo, hi))
	}
	if set.Mode == deciderLLM || set.Mode == deciderEnsemble {
		switch {
		case set.Model == "":
			notes = append(notes, "no memory model: confirmation falls back to the heuristic")
		case !pigModelAuthed(set.Model):
			notes = append(notes, "memory model "+set.Model+" is not an authed pig model: confirmation falls back to the heuristic")
		}
	}
	return strings.Join(notes, "; ")
}

// rawDeciderThresholds reports the file's confirm thresholds as written,
// before readDeciderSettings normalizes them. It reports false unless both
// keys parse, so the note only names a crossed pair the reset hides.
func rawDeciderThresholds() (float64, float64, bool) {
	all, err := readSettingsMap()
	if err != nil {
		return 0, 0, false
	}
	rawLo, hasLo := all["decider_confirm_low"]
	rawHi, hasHi := all["decider_confirm_high"]
	if !hasLo || !hasHi {
		return 0, 0, false
	}
	var lo, hi float64
	if json.Unmarshal(rawLo, &lo) != nil || json.Unmarshal(rawHi, &hi) != nil {
		return 0, 0, false
	}
	return lo, hi, true
}

// mineQuestions are the questions the miner asks: durable to keep or drop,
// kind and scope_hint to store with the observation. Contradiction pairing
// (ticket 22) and recall-time relevance (ticket 24) ask their questions
// where they run.
var mineQuestions = []memory.DeciderQuestion{
	memory.Noul(memory.QuestionDurable),
	memory.KindQuestion(),
	memory.ScopeQuestion(),
}

// mineDecision is one miner verdict: whether the candidate is kept, the kind
// and scope to store, and the decider_json provenance.
type mineDecision struct {
	keep      bool
	kind      string
	scope     string
	durable   float64
	decider   string // mode that judged: heuristic, llm, ensemble, test
	model     string
	version   string
	eligible  bool // at or above confirmHigh: queue-eligible (ticket 22)
	confirmed bool // a confirming Decider answered, not the fallback
}

// llmMineDecider is the confirming LLM Decider for set, or nil without a
// memory model. The caller falls back to the heuristic when it is nil.
func (d *Daemon) llmMineDecider(model string) memory.Decider {
	if strings.TrimSpace(model) == "" {
		return nil
	}
	return memory.LLMDecider{ModelName: model, Call: d.deciderLLMCall}
}

// deciderLLMCall runs one Decider completion: the same bare pig one-shot the
// compress summarizer uses, with no tools and no Session behind it. The user
// text is redacted for the Decider before it is sent.
func (d *Daemon) deciderLLMCall(_ context.Context, model, system, user string) (string, error) {
	redacted := memory.ForDecider(user)
	if d.deciderOneShot != nil {
		return d.deciderOneShot(model, system, redacted)
	}
	return pigOneShot("", model, system, redacted, deciderMaxTokens, deciderTimeout)
}

// decideMine judges one heuristic hit. The heuristic pass always proposes
// and always offers kind and scope; the confirming Decider (llm, ensemble's
// second, or the test seam) judges durable. A confirm keeps at confirmLow and
// is queue-eligible at confirmHigh, in every mode. No confirmer answering
// means no eligibility: an error or a missing answer leaves the heuristic
// keep standing, but the candidate is never queue-eligible on it. Only the
// heuristic mode offers eligibility from the heuristic score. It never
// errors: callers store or drop on the verdict.
func (d *Daemon) decideMine(text, contextText string, hit memory.HeuristicResult) mineDecision {
	set := readDeciderSettings()
	ctx := context.Background()
	state := memory.DeciderState{Statement: text, Context: contextText}
	out := mineDecision{
		keep: true, kind: memory.KindPreference, scope: "user_rule",
		durable: hit.Confidence, decider: set.Mode,
		model: memory.HeuristicModel, version: memory.HeuristicVersion,
	}
	if hres, err := (memory.HeuristicDecider{}).Decide(ctx, state, mineQuestions); err == nil {
		if k, ok := hres.Choice(memory.QuestionKind); ok && validMineKind(k) {
			out.kind = k
		}
		if s, ok := hres.Choice(memory.QuestionScopeHint); ok && validMineScope(s) {
			out.scope = s
		}
	}
	var second memory.Decider
	ensemble := false
	switch {
	case d.decider != nil:
		second = d.decider
		out.decider = "test"
	case set.Mode == deciderLLM:
		second = d.llmMineDecider(set.Model)
	case set.Mode == deciderEnsemble:
		second = d.llmMineDecider(set.Model)
		ensemble = true
	}
	if second == nil && !ensemble {
		out.eligible = out.durable >= set.High
		return out
	}
	if ensemble {
		return out.judgeEnsemble(ctx, state, second, set)
	}
	if res, err := second.Decide(ctx, state, mineQuestions); err == nil {
		if out.applyConfirm(res, set) {
			return out
		}
	}
	out.eligible = false
	return out
}

// applyConfirm folds a confirming result into the verdict: durable, kind and
// scope when answered and valid, kept at confirmLow and eligible at
// confirmHigh. It reports whether durable was answered; without it the
// heuristic verdict stands untouched.
func (out *mineDecision) applyConfirm(res memory.DeciderResult, set deciderSettings) bool {
	durable, ok := res.Noul(memory.QuestionDurable)
	if !ok {
		return false
	}
	out.durable, out.confirmed = durable, true
	if res.Model != "" && res.Version != "" {
		out.model, out.version = res.Model, res.Version
	}
	if k, ok := res.Choice(memory.QuestionKind); ok && validMineKind(k) {
		out.kind = k
	}
	if s, ok := res.Choice(memory.QuestionScopeHint); ok && validMineScope(s) {
		out.scope = s
	}
	out.keep = durable >= set.Low
	out.eligible = durable >= set.High
	return true
}

// judgeEnsemble confirms with the second Decider against the thresholds. A
// nil second (no memory model) observes without confirming: stored, never
// queue-eligible.
func (out *mineDecision) judgeEnsemble(ctx context.Context, state memory.DeciderState, second memory.Decider, set deciderSettings) mineDecision {
	e := memory.EnsembleDecider{First: memory.HeuristicDecider{}, Low: set.Low, High: set.High}
	if second != nil {
		e.Second = second
	} else {
		e.Second = &memory.ScriptedDecider{Unavailable: true}
	}
	v, err := e.Judge(ctx, state)
	if err != nil {
		// An unconfirmed ensemble candidate is never queue-eligible: the
		// second Decider must confirm each one.
		out.eligible = false
		return *out
	}
	out.durable, out.eligible = v.Durable, v.QueueEligible
	if v.Confirmed {
		out.model, out.version, out.confirmed = set.Model, memory.LLMVersion, true
	}
	if v.Verdict == memory.VerdictDrop {
		out.keep = false
	}
	return *out
}

// validMineKind reports whether k is a storable Rule kind.
func validMineKind(k string) bool {
	switch k {
	case memory.KindPreference, memory.KindDecision, memory.KindAntipattern, memory.KindFact, memory.KindProcedure:
		return true
	}
	return false
}

// validMineScope reports whether s is a storable scope hint.
func validMineScope(s string) bool {
	switch s {
	case "user_rule", "project_rule", "situational", "none":
		return true
	}
	return false
}

// deciderPatch is the PUT shape: every field optional.
type deciderPatch struct {
	Mode  *string  `json:"decider_mode"`
	Model *string  `json:"memory_model"`
	Low   *float64 `json:"decider_confirm_low"`
	High  *float64 `json:"decider_confirm_high"`
}

func (p *deciderPatch) validate() string {
	if p == nil {
		return ""
	}
	if p.Mode != nil {
		switch *p.Mode {
		case deciderHeuristic, deciderLLM, deciderEnsemble:
		default:
			return "decider_mode must be heuristic, llm or ensemble"
		}
	}
	if p.Model != nil {
		m := strings.TrimSpace(*p.Model)
		if m != "" && !pigModelAuthed(m) {
			return "memory_model must be an authed pig model"
		}
	}
	cur := readDeciderSettings()
	lo, hi := cur.Low, cur.High
	if p.Low != nil {
		lo = *p.Low
	}
	if p.High != nil {
		hi = *p.High
	}
	if p.Low != nil && (*p.Low < 0 || *p.Low > 1) {
		return "decider_confirm_low must be between 0 and 1"
	}
	if p.High != nil && (*p.High < 0 || *p.High > 1) {
		return "decider_confirm_high must be between 0 and 1"
	}
	if p.Low != nil || p.High != nil {
		if lo > hi {
			return "decider_confirm_low must not exceed decider_confirm_high"
		}
	}
	return ""
}

// apply writes the set fields into the settings map.
func (p *deciderPatch) apply(all map[string]json.RawMessage) {
	if p == nil {
		return
	}
	set := func(k string, v any) {
		raw, _ := json.Marshal(v)
		all[k] = raw
	}
	if p.Mode != nil {
		set("decider_mode", *p.Mode)
	}
	if p.Model != nil {
		set("memory_model", strings.TrimSpace(*p.Model))
	}
	if p.Low != nil {
		set("decider_confirm_low", *p.Low)
	}
	if p.High != nil {
		set("decider_confirm_high", *p.High)
	}
}
