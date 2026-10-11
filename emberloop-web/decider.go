package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"example.com/emberloop-web/memory"
)

// Memory Deciders: the miner's heuristic pass always proposes candidates,
// and a confirming Decider judges them. Modes mirror the spec: heuristic
// (default, no network), llm (a one-off pig call with no tools), jev (the
// Jev decision endpoint over HTTP, ticket 21), ensemble (the heuristic
// proposes, the llm or jev Decider confirms against the thresholds).

// Decider modes.
const (
	deciderHeuristic = "heuristic"
	deciderLLM       = "llm"
	deciderJev       = "jev"
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
			if str == deciderHeuristic || str == deciderLLM || str == deciderJev || str == deciderEnsemble {
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
// empty when it can: llm needs an authed memory model and jev needs its
// endpoint URL, else they fall back to the heuristic; crossed thresholds
// fall back to the defaults. When jev confirms, the note names that redacted
// user turns leave the host for the endpoint.
func deciderNote(set deciderSettings) string {
	var notes []string
	if lo, hi, ok := rawDeciderThresholds(); ok && lo > hi {
		notes = append(notes, fmt.Sprintf("decider_confirm_low %v exceeds decider_confirm_high %v: using the defaults", lo, hi))
	}
	jev := readJevSettings()
	switch set.Mode {
	case deciderLLM:
		notes = append(notes, llmModelNote(set.Model)...)
	case deciderJev:
		if jev.BaseURL == "" {
			notes = append(notes, "jev is off: set the endpoint URL to confirm with Jev, candidates stay unconfirmed")
		} else {
			notes = append(notes, "jev sends redacted user turns to "+jev.BaseURL)
		}
	case deciderEnsemble:
		if jev.BaseURL != "" {
			notes = append(notes, "ensemble confirms with jev: redacted user turns go to "+jev.BaseURL)
		} else {
			notes = append(notes, llmModelNote(set.Model)...)
		}
	}
	return strings.Join(notes, "; ")
}

// llmModelNote names why the memory model cannot confirm, or nothing when it
// can: llm and (without jev) ensemble need an authed pig model.
func llmModelNote(model string) []string {
	switch {
	case model == "":
		return []string{"no memory model: confirmation falls back to the heuristic"}
	case !pigModelAuthed(model):
		return []string{"memory model " + model + " is not an authed pig model: confirmation falls back to the heuristic"}
	}
	return nil
}

// jevSettings are the Jev endpoint knobs stored in the settings file. They
// are host-global: only the admin writes them, like signup_enabled. BaseURL
// is the full decision-endpoint URL the client POSTs to verbatim: the hosted
// /api/v1/decisions URL, a TypeSafe-compatible /v1/systemone URL, or a local
// laya-serve /v1/systemone URL. Empty means off: the client never sends.
// APIKey is the Bearer key, empty for a local server without one; it never
// leaves the settings file for the settings view. Model defaults to
// memory.DefaultJevModel.
type jevSettings struct {
	BaseURL   string `json:"base_url"`
	APIKey    string `json:"-"`
	APIKeySet bool   `json:"api_key_set"`
	Model     string `json:"model"`
}

// readJevSettings reads the settings file on every call, so a change applies
// to the next mine without a restart.
func readJevSettings() jevSettings {
	s := jevSettings{Model: memory.DefaultJevModel}
	all, err := readSettingsMap()
	if err != nil {
		return s
	}
	if raw, ok := all["jev_base_url"]; ok {
		var str string
		if json.Unmarshal(raw, &str) == nil {
			s.BaseURL = strings.TrimSpace(str)
		}
	}
	if raw, ok := all["jev_api_key"]; ok {
		var str string
		if json.Unmarshal(raw, &str) == nil {
			s.APIKey = strings.TrimSpace(str)
			s.APIKeySet = s.APIKey != ""
		}
	}
	if raw, ok := all["jev_model"]; ok {
		var str string
		if json.Unmarshal(raw, &str) == nil && strings.TrimSpace(str) != "" {
			s.Model = strings.TrimSpace(str)
		}
	}
	return s
}

// jevMineDecider is the confirming Jev Decider, or nil while the endpoint is
// off (no base URL). The caller falls back to the heuristic when it is nil.
// The key stays optional so a local Laya server without one still works.
func (d *Daemon) jevMineDecider() memory.Decider {
	set := readJevSettings()
	if set.BaseURL == "" {
		return nil
	}
	return memory.JevDecider{Config: memory.JevConfig{BaseURL: set.BaseURL, APIKey: set.APIKey, Model: set.Model}}
}

// jevPatch is the PUT shape: every field optional.
type jevPatch struct {
	BaseURL *string `json:"base_url"`
	APIKey  *string `json:"api_key"`
	Model   *string `json:"model"`
}

// validate refuses a patch with a non-URL endpoint, an over-long key or an
// over-long model. An empty base URL turns the endpoint off; an empty model
// resets it to the default.
func (p *jevPatch) validate() string {
	if p == nil {
		return ""
	}
	if p.BaseURL != nil {
		u := strings.TrimSpace(*p.BaseURL)
		if u != "" {
			parsed, err := url.Parse(u)
			if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
				return "jev base_url must be an http(s) URL for the decision endpoint"
			}
		}
	}
	if p.APIKey != nil && len(*p.APIKey) > 4096 {
		return "jev api_key is limited to 4096 characters"
	}
	if p.Model != nil {
		m := strings.TrimSpace(*p.Model)
		if len(m) > 128 {
			return "jev model is limited to 128 characters"
		}
		if strings.Contains(m, " ") || strings.Contains(m, "\t") || strings.Contains(m, "\n") {
			return "jev model must be one id"
		}
	}
	return ""
}

// apply writes the set fields into the settings map.
func (p *jevPatch) apply(all map[string]json.RawMessage) {
	if p == nil {
		return
	}
	set := func(k string, v any) {
		raw, _ := json.Marshal(v)
		all[k] = raw
	}
	if p.BaseURL != nil {
		set("jev_base_url", strings.TrimSpace(*p.BaseURL))
	}
	if p.APIKey != nil {
		set("jev_api_key", strings.TrimSpace(*p.APIKey))
	}
	if p.Model != nil {
		set("jev_model", strings.TrimSpace(*p.Model))
	}
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
	decider   string // mode that judged: heuristic, llm, jev, ensemble, test
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

// minerSubjectKey proposes the subject key for a mined statement. The
// confirming Decider names it when it implements SubjectKeyProposer (the test
// seam, or the memory model in llm and ensemble modes); otherwise the
// heuristic fallback proposes it. The result is normalized; empty means
// keyless and never promotes. It also reports who proposed the key (the
// Decider model, or the heuristic name) for the decider_json provenance.
func (d *Daemon) minerSubjectKey(statement, contextText string) (string, string) {
	state := memory.DeciderState{Statement: statement, Context: contextText}
	ctx := context.Background()
	var candidates []memory.Decider
	if d.decider != nil {
		candidates = []memory.Decider{d.decider}
	} else {
		set := readDeciderSettings()
		if (set.Mode == deciderLLM || set.Mode == deciderEnsemble) && strings.TrimSpace(set.Model) != "" {
			if llm := d.llmMineDecider(set.Model); llm != nil {
				candidates = append(candidates, llm)
			}
		}
		candidates = append(candidates, memory.HeuristicDecider{})
	}
	for _, c := range candidates {
		p, ok := c.(memory.SubjectKeyProposer)
		if !ok {
			continue
		}
		if key, err := p.ProposeSubjectKey(ctx, state); err == nil {
			if norm := memory.NormalizeSubjectKey(key); norm != "" {
				return norm, c.Model()
			}
		}
	}
	return memory.NormalizeSubjectKey(memory.ProposeSubjectKey(statement)), memory.HeuristicModel
}

// decideMine judges one heuristic hit. The heuristic pass always proposes
// and always offers kind and scope; the confirming Decider (llm, jev,
// ensemble's second, or the test seam) judges durable. A confirm keeps at
// confirmLow and is queue-eligible at confirmHigh, in every mode. No
// confirmer answering means no eligibility: an error or a missing answer
// leaves the heuristic keep standing, but the candidate is never
// queue-eligible on it. Only the heuristic mode offers eligibility from the
// heuristic score. It never errors: callers store or drop on the verdict.
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
	case set.Mode == deciderJev:
		second = d.jevMineDecider()
	case set.Mode == deciderEnsemble:
		ensemble = true
		// The ensemble confirms with jev when its endpoint is on, else
		// with the memory model: the configured second Decider.
		if j := d.jevMineDecider(); j != nil {
			second = j
		} else {
			second = d.llmMineDecider(set.Model)
		}
	}
	if second == nil && !ensemble {
		// Only the heuristic mode offers eligibility from the heuristic
		// score: without a confirmer (llm without a model, jev off) the
		// hit is stored unconfirmed, never eligible.
		out.eligible = set.Mode == deciderHeuristic && d.decider == nil && out.durable >= set.High
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
// nil second (no memory model and jev off) observes without confirming:
// stored, never queue-eligible.
func (out *mineDecision) judgeEnsemble(ctx context.Context, state memory.DeciderState, second memory.Decider, set deciderSettings) mineDecision {
	e := memory.EnsembleDecider{First: memory.HeuristicDecider{}, Low: set.Low, High: set.High}
	if second != nil {
		e.Second = second
	} else {
		e.Second = memory.UnavailableDecider{}
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
		out.model, out.version, out.confirmed = second.Model(), second.Version(), true
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
		case deciderHeuristic, deciderLLM, deciderJev, deciderEnsemble:
		default:
			return "decider_mode must be heuristic, llm, jev or ensemble"
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
