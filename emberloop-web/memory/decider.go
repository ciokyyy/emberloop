// Decider interface and its heuristic, LLM and ensemble implementations.
//
// A Decider answers typed questions about a statement and returns
// probabilities. Questions (at most 6 per call):
//
//   - durable (noul): does the user state a rule, preference or correction
//     meant for future work, not only for this task?
//   - kind (choice): preference, decision, antipattern, fact, procedure.
//   - scope_hint (choice): user_rule, project_rule, situational, none. A
//     project_rule hint is only a hint; Rules stay private until a human
//     promotes them.
//   - contradicts (noul): the state holds the candidate and one stored Rule;
//     does the candidate contradict it?
//   - relevant (noul, one per candidate, recall time): is this Rule relevant
//     to the task prompt?
//
// Implementations: HeuristicDecider (the regex pass from miner.go; default,
// no network, pass/fail scores), LLMDecider (a one-off pig call with no
// tools and JSON-schema output; the caller supplies the completion seam, so
// this package never touches the network) and EnsembleDecider (the heuristic
// proposes, a second Decider confirms against confirmLow/confirmHigh).
// ScriptedDecider is the fake for tests.
//
// Validation, thresholds and all state changes are ordinary Go code. A
// Decider never writes: callers validate the answers and apply them.
// Invalid or out-of-range output is rejected as unavailable, never as a yes.
package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Typed questions a Decider answers.
const (
	QuestionDurable     = "durable"
	QuestionKind        = "kind"
	QuestionScopeHint   = "scope_hint"
	QuestionContradicts = "contradicts"
	QuestionRelevant    = "relevant"
)

// Question types: noul carries a probability, choice picks one option.
const (
	QTypeNoul   = "noul"
	QTypeChoice = "choice"
)

// Choice options for the kind and scope_hint questions.
var (
	KindOptions  = []string{KindPreference, KindDecision, KindAntipattern, KindFact, KindProcedure}
	ScopeOptions = []string{"user_rule", "project_rule", "situational", "none"}
)

// Ensemble thresholds and verdicts. Below ConfirmLow a candidate is dropped;
// between the thresholds it is stored as an observation; at or above
// ConfirmHigh it is eligible for the queue.
const (
	DefaultConfirmLow  = 0.35
	DefaultConfirmHigh = 0.7

	VerdictDrop     = "drop"
	VerdictObserve  = "observe"
	VerdictEligible = "eligible"
)

// Decider model names and versions, stored with every output in decider_json.
const (
	HeuristicModel   = "heuristic"
	HeuristicVersion = "v1"
	LLMVersion       = "v1"
)

// MaxDeciderQuestions caps one Decider call, per the spec.
const MaxDeciderQuestions = 6

// ErrDeciderUnavailable is the failure of a Decider call that produced no
// usable answer: a model error, a timeout, malformed output or an
// out-of-range value. Callers treat it as unavailable, never as a yes.
var ErrDeciderUnavailable = fmt.Errorf("memory: decider unavailable")

// DeciderState is what a Decider judges. Fields travel only where the
// question needs them: durable/kind/scope_hint read Statement (plus Context),
// contradicts reads Candidate and RuleText, relevant reads Task and RuleText.
type DeciderState struct {
	Statement string // the user statement under judgment
	Context   string // the thing it corrected (tool call or diff hunk)
	Candidate string // a mined candidate, for contradicts
	RuleText  string // one stored Rule's text, for contradicts and relevant
	Task      string // the task prompt, for relevant
}

// DeciderQuestion is one typed question. Options holds the choice options;
// it is empty for noul questions.
type DeciderQuestion struct {
	Name    string
	Options []string
}

// Noul asks a yes/no probability question; Choice asks a kind/scope question
// with the spec's options.
func Noul(name string) DeciderQuestion { return DeciderQuestion{Name: name} }

// Choice asks a choice question with options (2 to 8, enforced).
func Choice(name string, options ...string) DeciderQuestion {
	return DeciderQuestion{Name: name, Options: options}
}

// KindQuestion is the kind question with the spec's five options.
func KindQuestion() DeciderQuestion { return Choice(QuestionKind, KindOptions...) }

// ScopeQuestion is the scope_hint question with the spec's four options.
func ScopeQuestion() DeciderQuestion { return Choice(QuestionScopeHint, ScopeOptions...) }

// QuestionType reports a question's type, false for an unknown question.
func QuestionType(name string) (string, bool) {
	switch name {
	case QuestionDurable, QuestionContradicts, QuestionRelevant:
		return QTypeNoul, true
	case QuestionKind, QuestionScopeHint:
		return QTypeChoice, true
	}
	return "", false
}

// Answer is one judged question. Available is false when the Decider could
// not answer it; callers treat that as unavailable, never as a yes.
type Answer struct {
	Question      string
	Available     bool
	Prob          float64 // noul probability, 0 to 1
	Choice        string
	Probabilities map[string]float64
	Confidence    float64
}

// DeciderResult is one Decider call: the answers by question plus the model
// and version, which the caller stores in decider_json.
type DeciderResult struct {
	Answers map[string]Answer
	Model   string
	Version string
}

// Noul returns the probability of a noul question, false when unanswered.
func (r DeciderResult) Noul(name string) (float64, bool) {
	a, ok := r.Answers[name]
	if !ok || !a.Available {
		return 0, false
	}
	return a.Prob, true
}

// Choice returns the pick of a choice question, false when unanswered.
func (r DeciderResult) Choice(name string) (string, bool) {
	a, ok := r.Answers[name]
	if !ok || !a.Available {
		return "", false
	}
	return a.Choice, true
}

// Decider answers typed questions about a statement and returns
// probabilities. It never writes; the caller validates and applies.
type Decider interface {
	Decide(ctx context.Context, state DeciderState, questions []DeciderQuestion) (DeciderResult, error)
	Model() string
	Version() string
}

// ValidateQuestions rejects a question list the Decider must not send: empty,
// over the 6-question cap, unknown names, or choice options outside 2 to 8
// (and outside the spec's sets for kind and scope_hint).
func ValidateQuestions(qs []DeciderQuestion) error {
	if len(qs) == 0 || len(qs) > MaxDeciderQuestions {
		return fmt.Errorf("%w: %d questions, want 1 to %d", ErrDeciderUnavailable, len(qs), MaxDeciderQuestions)
	}
	seen := map[string]bool{}
	for _, q := range qs {
		typ, ok := QuestionType(q.Name)
		if !ok {
			return fmt.Errorf("%w: unknown question %q", ErrDeciderUnavailable, q.Name)
		}
		if seen[q.Name] {
			return fmt.Errorf("%w: duplicate question %q", ErrDeciderUnavailable, q.Name)
		}
		seen[q.Name] = true
		if typ == QTypeChoice {
			if len(q.Options) < 2 || len(q.Options) > 8 {
				return fmt.Errorf("%w: question %q has %d options, want 2 to 8", ErrDeciderUnavailable, q.Name, len(q.Options))
			}
			var want []string
			switch q.Name {
			case QuestionKind:
				want = KindOptions
			case QuestionScopeHint:
				want = ScopeOptions
			}
			if want != nil && !sameOptions(q.Options, want) {
				return fmt.Errorf("%w: question %q options %q, want %q", ErrDeciderUnavailable, q.Name, q.Options, want)
			}
		}
	}
	return nil
}

func sameOptions(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ValidateAnswer rejects one answer: a noul probability outside 0 to 1, a
// choice outside its options, or a probability or confidence outside 0 to 1.
func ValidateAnswer(q DeciderQuestion, a Answer) error {
	bad := func(format string, args ...any) error {
		return fmt.Errorf("%w: question %q: %s", ErrDeciderUnavailable, q.Name, fmt.Sprintf(format, args...))
	}
	typ, _ := QuestionType(q.Name)
	switch typ {
	case QTypeNoul:
		if a.Prob < 0 || a.Prob > 1 {
			return bad("probability %v out of range", a.Prob)
		}
	case QTypeChoice:
		found := false
		for _, o := range q.Options {
			if a.Choice == o {
				found = true
				break
			}
		}
		if !found {
			return bad("choice %q not in %q", a.Choice, q.Options)
		}
		for opt, p := range a.Probabilities {
			if p < 0 || p > 1 {
				return bad("probability %v for %q out of range", p, opt)
			}
		}
		if a.Confidence < 0 || a.Confidence > 1 {
			return bad("confidence %v out of range", a.Confidence)
		}
	}
	return nil
}

// HeuristicDecider is the regex pass from miner.go as a Decider. Default, no
// network. Scores are pass/fail (1 or 0), except durable, which carries the
// pass's confidence.
type HeuristicDecider struct{}

// Model reports the decider_json model name.
func (HeuristicDecider) Model() string { return HeuristicModel }

// Version reports the decider_json version.
func (HeuristicDecider) Version() string { return HeuristicVersion }

// Decide answers the asked questions about state.Statement. It answers every
// known question; a statement with nothing durable still gets durable 0
// (plus placeholder kind and scope none), so the caller can drop it without
// a special case.
func (HeuristicDecider) Decide(_ context.Context, state DeciderState, questions []DeciderQuestion) (DeciderResult, error) {
	if err := ValidateQuestions(questions); err != nil {
		return DeciderResult{}, err
	}
	hit, ok := ClassifyHeuristic(state.Statement)
	res := DeciderResult{Answers: map[string]Answer{}, Model: HeuristicModel, Version: HeuristicVersion}
	for _, q := range questions {
		switch q.Name {
		case QuestionDurable:
			prob := 0.0
			if ok {
				prob = hit.Confidence
			}
			res.Answers[q.Name] = Answer{Question: q.Name, Available: true, Prob: prob}
		case QuestionKind:
			res.Answers[q.Name] = heuristicKind(state.Statement, hit, ok)
		case QuestionScopeHint:
			res.Answers[q.Name] = heuristicScope(state.Statement, ok)
		case QuestionContradicts:
			prob := 0.0
			if state.Candidate != "" && state.RuleText != "" && Relation(state.Candidate, state.RuleText) == RelContradicts {
				prob = 1.0
			}
			res.Answers[q.Name] = Answer{Question: q.Name, Available: true, Prob: prob}
		case QuestionRelevant:
			rule := state.RuleText
			if rule == "" {
				rule = state.Candidate
			}
			prob := 0.0
			if sharesToken(state.Task, rule) {
				prob = 1.0
			}
			res.Answers[q.Name] = Answer{Question: q.Name, Available: true, Prob: prob}
		}
	}
	return res, nil
}

// heuristicKind maps a statement to a kind, best effort. Negations of
// behavior ("don't", "never") are antipatterns; settled picks ("we decided")
// are decisions; standing instructions are preferences; how-to corrections
// are procedures; bare praise that survived the forward-pivot guard is a fact
// about what worked. Anything else keeps the pass's hint: guardrails and
// explicit notes read as preferences, corrections as procedures.
func heuristicKind(text string, hit HeuristicResult, ok bool) Answer {
	lower := strings.ToLower(text)
	has := func(words ...string) bool {
		for _, w := range words {
			if strings.Contains(lower, w) {
				return true
			}
		}
		return false
	}
	kind := KindPreference
	switch {
	case has("don't", "do not", "never", "stop ", "avoid", "unless"):
		kind = KindAntipattern
	case has("we decided", "decided to", "we chose", "we picked", "going with", "agreed"):
		kind = KindDecision
	case has("always", "prefer", "usually", "remember:", "only change", "minimal", "tabs", "style", "format") || (ok && (hit.Type == HitGuardrail || hit.Type == HitExplicit)):
		kind = KindPreference
	case has("how to", "steps", "procedure", "workflow", "run ", "command"):
		kind = KindProcedure
	case ok && hit.Type == HitPositive:
		kind = KindFact
	case ok && hit.Type == HitAuto:
		kind = KindProcedure
	}
	probs := map[string]float64{}
	for _, o := range KindOptions {
		probs[o] = 0.0
	}
	probs[kind] = 1.0
	return Answer{Question: QuestionKind, Available: true, Choice: kind, Probabilities: probs, Confidence: 1.0}
}

// Generalizing and situational markers for the scope_hint heuristic. A
// project_rule hint is only a hint; Rules stay private until a human
// promotes them.
var (
	scopeProjectMarkers     = []string{"in this repo", "in this project", "project-wide", "for the team", "everyone should", "team "}
	scopeGeneralMarkers     = []string{"always", "from now on", "in general", "as a rule"}
	scopeSituationalMarkers = []string{"here", "this function", "for now", "this file", "just this", "this time", "right now", "in this case"}
)

// heuristicScope maps a statement to a scope hint. Nothing durable is scope
// none; project markers hint project_rule; generalizing language reads as a
// user rule; situational language reads as situational; a durable statement
// with no markers defaults to a user rule.
func heuristicScope(text string, durable bool) Answer {
	lower := strings.ToLower(text)
	has := func(words ...string) bool {
		for _, w := range words {
			if strings.Contains(lower, w) {
				return true
			}
		}
		return false
	}
	scope := "none"
	switch {
	case !durable:
		scope = "none"
	case has(scopeProjectMarkers...):
		scope = "project_rule"
	case has(scopeSituationalMarkers...):
		scope = "situational"
	case has(scopeGeneralMarkers...):
		scope = "user_rule"
	default:
		scope = "user_rule"
	}
	probs := map[string]float64{}
	for _, o := range ScopeOptions {
		probs[o] = 0.0
	}
	probs[scope] = 1.0
	return Answer{Question: QuestionScopeHint, Available: true, Choice: scope, Probabilities: probs, Confidence: 1.0}
}

// sharesToken reports whether task and rule share a substance token of more
// than two letters, after the dedup normalization (filler dropped, negation
// kept). One shared token is enough for the pass/fail heuristic.
func sharesToken(task, rule string) bool {
	if task == "" || rule == "" {
		return false
	}
	have := tokenSet(NormalizeQuote(task))
	for w := range tokenSet(NormalizeQuote(rule)) {
		if len([]rune(w)) > 2 && have[w] {
			return true
		}
	}
	return false
}

// Decider input caps, in runes. The state is redacted and size-capped before
// any model sees it.
const (
	maxDeciderStatementRunes = 4000
	maxDeciderContextRunes   = 2000
	maxDeciderTaskRunes      = 2000
)

// LLMDecider asks a pig model with no tools and JSON-schema output. Call is
// the one-off completion seam (the Daemon wires it to the same pig one-shot
// the compress summarizer uses); this package never touches the network.
// The state is redacted with ForDecider in Decide, before Call runs.
type LLMDecider struct {
	ModelName string
	Call      func(ctx context.Context, model, system, user string) (string, error)
}

// Model reports the model the Decider asks.
func (d LLMDecider) Model() string { return d.ModelName }

// Version reports the decider_json version.
func (LLMDecider) Version() string { return LLMVersion }

// Decide asks the model the given questions and parses the reply. Malformed
// JSON fails the whole call as unavailable; one bad answer marks only that
// answer unavailable, never a yes.
func (d LLMDecider) Decide(ctx context.Context, state DeciderState, questions []DeciderQuestion) (DeciderResult, error) {
	if err := ValidateQuestions(questions); err != nil {
		return DeciderResult{}, err
	}
	if strings.TrimSpace(d.ModelName) == "" || d.Call == nil {
		return DeciderResult{}, fmt.Errorf("%w: llm decider has no model", ErrDeciderUnavailable)
	}
	system := deciderSystemPrompt(questions)
	// Redact before the model sees anything: the only path to the Decider
	// LLM runs through ForDecider here.
	user := deciderUserMessage(DeciderState{
		Statement: ForDecider(firstRunes(state.Statement, maxDeciderStatementRunes)),
		Context:   ForDecider(firstRunes(state.Context, maxDeciderContextRunes)),
		Candidate: ForDecider(firstRunes(state.Candidate, maxDeciderContextRunes)),
		RuleText:  ForDecider(firstRunes(state.RuleText, maxDeciderContextRunes)),
		Task:      ForDecider(firstRunes(state.Task, maxDeciderTaskRunes)),
	}, questions)
	raw, err := d.Call(ctx, d.ModelName, system, user)
	if err != nil {
		return DeciderResult{}, fmt.Errorf("%w: %v", ErrDeciderUnavailable, err)
	}
	return parseDeciderReply(d.ModelName, questions, raw)
}

// deciderSystemPrompt tells the model the questions and the exact reply
// shape: one JSON object with an answers array, nothing else.
func deciderSystemPrompt(questions []DeciderQuestion) string {
	var b strings.Builder
	b.WriteString("You judge whether a user's message states something worth remembering for future work.\n")
	b.WriteString("Answer each question below. Reply with one JSON object and nothing else, in exactly this shape:\n")
	b.WriteString(`{"answers":[{"question":"durable","noul":0.8},{"question":"kind","choice":"preference","probabilities":{"preference":0.7,"decision":0.1,"antipattern":0.1,"fact":0.05,"procedure":0.05},"confidence":0.7}]}` + "\n")
	b.WriteString("Rules: noul is a probability from 0 to 1. choice must be one of the listed options, with probabilities for every option from 0 to 1 and a confidence from 0 to 1. durable asks whether the user states a rule, preference or correction meant for future work, not only for this task. kind picks the note kind. scope_hint picks user_rule (this user's standing instruction), project_rule (knowledge the whole project could use), situational (only this spot) or none. contradicts asks whether the candidate contradicts the stored rule. relevant asks whether the stored rule matters for the task prompt.\n")
	b.WriteString("Questions:\n")
	for _, q := range questions {
		if len(q.Options) == 0 {
			b.WriteString("- " + q.Name + " (noul)\n")
		} else {
			b.WriteString("- " + q.Name + " (choice: " + strings.Join(q.Options, ", ") + ")\n")
		}
	}
	return b.String()
}

// deciderUserMessage renders the redacted state and the questions for the model.
func deciderUserMessage(state DeciderState, questions []DeciderQuestion) string {
	var b strings.Builder
	b.WriteString("Statement:\n" + state.Statement + "\n")
	if state.Context != "" {
		b.WriteString("\nWhat it corrected:\n" + state.Context + "\n")
	}
	if state.Candidate != "" {
		b.WriteString("\nCandidate:\n" + state.Candidate + "\n")
	}
	if state.RuleText != "" {
		b.WriteString("\nStored rule:\n" + state.RuleText + "\n")
	}
	if state.Task != "" {
		b.WriteString("\nTask prompt:\n" + state.Task + "\n")
	}
	names := make([]string, len(questions))
	for i, q := range questions {
		names[i] = q.Name
	}
	b.WriteString("\nAnswer: " + strings.Join(names, ", ") + "\n")
	return b.String()
}

// deciderReply is the parsed shape of a model reply.
type deciderReply struct {
	Answers []deciderReplyAnswer `json:"answers"`
}

type deciderReplyAnswer struct {
	Question      string             `json:"question"`
	Noul          *float64           `json:"noul"`
	Choice        string             `json:"choice"`
	Probabilities map[string]float64 `json:"probabilities"`
	Confidence    *float64           `json:"confidence"`
}

// parseDeciderReply parses and validates one model reply. Unknown shapes and
// answers for unasked questions fail the whole call; one out-of-range answer
// marks only itself unavailable.
func parseDeciderReply(model string, questions []DeciderQuestion, raw string) (DeciderResult, error) {
	res := DeciderResult{Answers: map[string]Answer{}, Model: model, Version: LLMVersion}
	byName := map[string]DeciderQuestion{}
	for _, q := range questions {
		byName[q.Name] = q
	}
	var reply deciderReply
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &reply); err != nil {
		return DeciderResult{}, fmt.Errorf("%w: malformed decider reply", ErrDeciderUnavailable)
	}
	seen := map[string]bool{}
	for _, a := range reply.Answers {
		q, asked := byName[a.Question]
		if !asked {
			return DeciderResult{}, fmt.Errorf("%w: answer for unasked question %q", ErrDeciderUnavailable, a.Question)
		}
		if seen[a.Question] {
			return DeciderResult{}, fmt.Errorf("%w: duplicate answer for %q", ErrDeciderUnavailable, a.Question)
		}
		seen[a.Question] = true
		typ, _ := QuestionType(q.Name)
		ans := Answer{Question: q.Name, Available: true}
		valid := true
		switch typ {
		case QTypeNoul:
			if a.Noul == nil {
				valid = false
				break
			}
			ans.Prob = *a.Noul
		case QTypeChoice:
			conf := 0.0
			if a.Confidence != nil {
				conf = *a.Confidence
			}
			ans.Choice, ans.Probabilities, ans.Confidence = a.Choice, a.Probabilities, conf
		}
		if valid {
			if err := ValidateAnswer(q, ans); err != nil {
				valid = false
			}
		}
		ans.Available = valid
		if !valid {
			ans = Answer{Question: q.Name, Available: false}
		}
		res.Answers[q.Name] = ans
	}
	for _, q := range questions {
		if _, ok := res.Answers[q.Name]; !ok {
			res.Answers[q.Name] = Answer{Question: q.Name, Available: false}
		}
	}
	return res, nil
}

// EnsembleDecider proposes with First (the heuristic) and confirms with
// Second (the llm or jev Decider). A candidate the heuristic never proposed
// is dropped; a candidate the second Decider cannot judge is stored as an
// observation but never queue-eligible.
type EnsembleDecider struct {
	First  Decider
	Second Decider
	Low    float64 // confirmLow; both zero means the defaults
	High   float64 // confirmHigh; both zero means the defaults
}

// EnsembleVerdict is one ensemble judgment.
type EnsembleVerdict struct {
	Proposed      bool    // the heuristic proposed the candidate
	Durable       float64 // the confirming durable probability
	Verdict       string  // drop, observe or eligible
	QueueEligible bool    // durable at or above confirmHigh
	Confirmed     bool    // the second Decider answered
}

// thresholds resolves the ensemble thresholds, defaulting when unset.
func (e EnsembleDecider) thresholds() (float64, float64, error) {
	lo, hi := e.Low, e.High
	if lo == 0 && hi == 0 {
		lo, hi = DefaultConfirmLow, DefaultConfirmHigh
	}
	if lo < 0 || hi > 1 || lo > hi {
		return 0, 0, fmt.Errorf("%w: thresholds %v %v", ErrDeciderUnavailable, lo, hi)
	}
	return lo, hi, nil
}

// Judge proposes state with First and confirms durable with Second. Only the
// durable question travels: kind and scope come from the proposing pass.
func (e EnsembleDecider) Judge(ctx context.Context, state DeciderState) (EnsembleVerdict, error) {
	lo, hi, err := e.thresholds()
	if err != nil {
		return EnsembleVerdict{}, err
	}
	if e.First == nil || e.Second == nil {
		return EnsembleVerdict{}, fmt.Errorf("%w: ensemble needs two deciders", ErrDeciderUnavailable)
	}
	first, err := e.First.Decide(ctx, state, []DeciderQuestion{Noul(QuestionDurable)})
	if err != nil {
		return EnsembleVerdict{}, err
	}
	proposed, ok := first.Noul(QuestionDurable)
	if !ok || proposed <= 0 {
		return EnsembleVerdict{Verdict: VerdictDrop}, nil
	}
	second, err := e.Second.Decide(ctx, state, []DeciderQuestion{Noul(QuestionDurable)})
	if err != nil {
		return EnsembleVerdict{Proposed: true, Durable: proposed, Verdict: VerdictObserve}, nil
	}
	durable, ok := second.Noul(QuestionDurable)
	if !ok {
		return EnsembleVerdict{Proposed: true, Durable: proposed, Verdict: VerdictObserve}, nil
	}
	switch {
	case durable < lo:
		return EnsembleVerdict{Proposed: true, Durable: durable, Verdict: VerdictDrop, Confirmed: true}, nil
	case durable >= hi:
		return EnsembleVerdict{Proposed: true, Durable: durable, Verdict: VerdictEligible, QueueEligible: true, Confirmed: true}, nil
	default:
		return EnsembleVerdict{Proposed: true, Durable: durable, Verdict: VerdictObserve, Confirmed: true}, nil
	}
}

// UnavailableDecider is a Decider that never answers: every asked question
// reads as unavailable. The ensemble uses it when no second Decider is
// configured, so the candidate observes unconfirmed instead of failing the
// mine.
type UnavailableDecider struct{}

// Model reports the decider_json model name.
func (UnavailableDecider) Model() string { return "unavailable" }

// Version reports the decider_json version.
func (UnavailableDecider) Version() string { return "v1" }

// Decide answers nothing: every question is unavailable, never a yes.
func (UnavailableDecider) Decide(_ context.Context, _ DeciderState, questions []DeciderQuestion) (DeciderResult, error) {
	if err := ValidateQuestions(questions); err != nil {
		return DeciderResult{}, err
	}
	res := DeciderResult{Answers: map[string]Answer{}, Model: "unavailable", Version: "v1"}
	for _, q := range questions {
		res.Answers[q.Name] = Answer{Question: q.Name, Available: false}
	}
	return res, nil
}

// ScriptedDecider is the fake Decider: scripted probabilities and choices
// drive Daemon tests. Unavailable (or Err) answers read as unavailable.
type ScriptedDecider struct {
	ModelName   string
	VersionName string
	Durable     float64
	Kind        string
	Scope       string
	Contradicts float64
	Relevant    float64
	Unavailable bool
	Err         error
	Calls       int
}

// scriptedModelName reports the decider_json model name, defaulting for tests.
func (s *ScriptedDecider) scriptedModelName() string {
	if s.ModelName == "" {
		return "scripted"
	}
	return s.ModelName
}

// scriptedVersionName reports the decider_json version, defaulting for tests.
func (s *ScriptedDecider) scriptedVersionName() string {
	if s.VersionName == "" {
		return "test"
	}
	return s.VersionName
}

// Model reports the decider_json model name.
func (s *ScriptedDecider) Model() string { return s.scriptedModelName() }

// Version reports the decider_json version.
func (s *ScriptedDecider) Version() string { return s.scriptedVersionName() }

// Decide answers every asked question from the script.
func (s *ScriptedDecider) Decide(_ context.Context, _ DeciderState, questions []DeciderQuestion) (DeciderResult, error) {
	if err := ValidateQuestions(questions); err != nil {
		return DeciderResult{}, err
	}
	s.Calls++
	if s.Err != nil {
		return DeciderResult{}, fmt.Errorf("%w: %v", ErrDeciderUnavailable, s.Err)
	}
	res := DeciderResult{Answers: map[string]Answer{}, Model: s.scriptedModelName(), Version: s.scriptedVersionName()}
	probs := func(winner string, options []string) map[string]float64 {
		m := map[string]float64{}
		for _, o := range options {
			m[o] = 0.0
		}
		m[winner] = 1.0
		return m
	}
	for _, q := range questions {
		if s.Unavailable {
			res.Answers[q.Name] = Answer{Question: q.Name, Available: false}
			continue
		}
		switch q.Name {
		case QuestionDurable:
			res.Answers[q.Name] = Answer{Question: q.Name, Available: true, Prob: s.Durable}
		case QuestionKind:
			kind := s.Kind
			if kind == "" {
				kind = KindPreference
			}
			res.Answers[q.Name] = Answer{Question: q.Name, Available: true, Choice: kind, Probabilities: probs(kind, KindOptions), Confidence: 1.0}
		case QuestionScopeHint:
			scope := s.Scope
			if scope == "" {
				scope = "user_rule"
			}
			res.Answers[q.Name] = Answer{Question: q.Name, Available: true, Choice: scope, Probabilities: probs(scope, ScopeOptions), Confidence: 1.0}
		case QuestionContradicts:
			res.Answers[q.Name] = Answer{Question: q.Name, Available: true, Prob: s.Contradicts}
		case QuestionRelevant:
			res.Answers[q.Name] = Answer{Question: q.Name, Available: true, Prob: s.Relevant}
		}
	}
	return res, nil
}
