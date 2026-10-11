// Jev Decider: an HTTP client for the Jev decision endpoint as a Decider.
//
// The client speaks the shared {model, state, questions} body to the endpoint
// URL it is configured with, verbatim: the hosted Jev endpoint
// (POST /api/v1/decisions, e.g. https://jev-api.org/api/v1/decisions) and the
// TypeSafe POST /v1/systemone shape take the same body and return the same
// answers map, so one code path serves both. A local Laya server
// (laya[serve]) plugs in by baseURL alone, with no adapter:
//
//   - laya-serve exposes only POST /v1/systemone (plus /batch, /models and
//     /health); there is no /api/v1/decisions path on it. Point baseURL at
//     http://localhost:8000/v1/systemone.
//   - Upstream (NandhaKishorM/laya README, "Self-Hosting: HTTP Server
//     (Jev-compatible)", verified against laya 0.4.2's serve.py) states the
//     answer payload is already schema-identical to Jev's (choice / score /
//     noul answers plus an input/output usage block), so an existing Jev
//     client only needs its base URL repointed. The parser below ignores
//     unknown fields, which absorbs Laya's extras (routing, extended usage,
//     answer_confidence, action, x_jev_confidence); LAYA_JEV_STRICT=1 makes
//     the server project onto the exact Jev contract instead.
//   - Two Laya deltas callers should know, neither needing an adapter: a
//     client's model field is honoured only when it names a Laya checkpoint
//     (english, multilingual, typed-decisions); jev-* means the router
//     auto-selects. And Laya's confidence is 1 minus normalised entropy, not
//     Jev's formula, so gate on the durable probability, never on
//     confidence. Score levels need descriptions (null levels are 422).
//
// The client enforces the strict /decisions limits on every call (at most 6
// questions, 2 to 8 choice options), which also fit inside the wider
// /v1/systemone ranges, so one validation serves both shapes. The state is
// redacted with ForJev and size-capped before it is sent, and nothing is
// sent while the endpoint URL is unset (off by default). Every transport,
// status or shape failure is ErrDeciderUnavailable, never a yes.
package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Jev model names and versions, stored with every output in decider_json.
const (
	// DefaultJevModel is the rolling Jev alias sent when no model is set.
	DefaultJevModel = "jev-latest"
	// JevVersion is the decider_json version for Jev answers.
	JevVersion = "v1"
)

// jevTimeout bounds one Jev call when the config carries no HTTP client.
// Recall-time callers bound it further with their own context.
const jevTimeout = 30 * time.Second

// maxJevBodyBytes caps one Jev response body read.
const maxJevBodyBytes = 1 << 20

// JevConfig configures a JevDecider. BaseURL is the full decision-endpoint
// URL the client POSTs to verbatim: the hosted /api/v1/decisions URL, a
// TypeSafe-compatible /v1/systemone URL, or a local laya-serve
// /v1/systemone URL. Empty means off: Decide sends nothing and reports
// unavailable. APIKey is the Bearer key, sent only when non-empty, so a
// local server without one still works. Model defaults to DefaultJevModel.
// Client, when non-nil, replaces the default 30s client (tests).
type JevConfig struct {
	BaseURL string
	APIKey  string
	Model   string
	Client  *http.Client
}

// jevModel reports the model to send, defaulting to the rolling alias.
func (c JevConfig) jevModel() string {
	if m := strings.TrimSpace(c.Model); m != "" {
		return m
	}
	return DefaultJevModel
}

// jevClient reports the HTTP client to send with.
func (c JevConfig) jevClient() *http.Client {
	if c.Client != nil {
		return c.Client
	}
	return &http.Client{Timeout: jevTimeout}
}

// JevDecider asks a Jev-compatible decision endpoint. It never writes; the
// caller validates and applies.
type JevDecider struct {
	Config JevConfig
}

// Model reports the decider_json model name.
func (d JevDecider) Model() string { return d.Config.jevModel() }

// Version reports the decider_json version.
func (JevDecider) Version() string { return JevVersion }

// Decide asks the endpoint the given questions and parses the reply. The
// question list is validated before anything is built or sent, so an
// over-long list never reaches the network. An empty endpoint URL sends
// nothing. Noul, choice and score answers all parse into the result;
// a 401, 422, timeout, non-2xx status or malformed body is unavailable.
// Unknown answer ids fail the whole call; one bad answer marks only itself
// unavailable, never a yes.
func (d JevDecider) Decide(ctx context.Context, state DeciderState, questions []DeciderQuestion) (DeciderResult, error) {
	if err := ValidateQuestions(questions); err != nil {
		return DeciderResult{}, err
	}
	endpoint := strings.TrimSpace(d.Config.BaseURL)
	if endpoint == "" {
		return DeciderResult{}, fmt.Errorf("%w: jev endpoint is off", ErrDeciderUnavailable)
	}
	if u, err := url.Parse(endpoint); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return DeciderResult{}, fmt.Errorf("%w: jev endpoint %q is not an http(s) URL", ErrDeciderUnavailable, endpoint)
	}
	// Redact before anything leaves the Daemon: the only path to the Jev
	// API runs through ForJev here, and every field is size-capped first.
	body, _ := json.Marshal(jevRequest{
		Model:     d.Config.jevModel(),
		State:     ForJev(jevStateString(state)),
		Questions: jevQuestions(questions),
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return DeciderResult{}, fmt.Errorf("%w: jev request: %v", ErrDeciderUnavailable, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if key := strings.TrimSpace(d.Config.APIKey); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	res, err := d.Config.jevClient().Do(req)
	if err != nil {
		return DeciderResult{}, fmt.Errorf("%w: jev call: %v", ErrDeciderUnavailable, err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxJevBodyBytes))
	if err != nil {
		return DeciderResult{}, fmt.Errorf("%w: jev body: %v", ErrDeciderUnavailable, err)
	}
	// Any non-2xx is unavailable: 401 a missing or rejected key, 402 no
	// credits, 422 a rejected body, 429 a rate limit, 502 no answers.
	if res.StatusCode/100 != 2 {
		return DeciderResult{}, fmt.Errorf("%w: jev answered status %d", ErrDeciderUnavailable, res.StatusCode)
	}
	return parseJevReply(d.Config.jevModel(), questions, raw)
}

// jevRequest is the shared decision body: the hosted /api/v1/decisions
// endpoint and the TypeSafe /v1/systemone shape both take it.
type jevRequest struct {
	Model     string              `json:"model"`
	State     string              `json:"state"`
	Questions map[string]jevWireQ `json:"questions"`
}

// jevWireQ is one typed question on the wire.
type jevWireQ struct {
	Type         string `json:"type"`
	Instructions string `json:"instructions"`
	Criteria     any    `json:"criteria,omitempty"`
}

// jevStateString renders the size-capped state: the statement plus whatever
// else the questions read (the thing it corrected, the candidate against
// its stored rule, the task prompt). Field caps match the LLM Decider's;
// the joined state stays far under the endpoint's 60k limit. Decide redacts
// the joined string with ForJev before it is sent.
func jevStateString(state DeciderState) string {
	var b strings.Builder
	b.WriteString("Statement:\n" + firstRunes(state.Statement, maxDeciderStatementRunes) + "\n")
	if state.Context != "" {
		b.WriteString("\nWhat it corrected:\n" + firstRunes(state.Context, maxDeciderContextRunes) + "\n")
	}
	if state.Candidate != "" {
		b.WriteString("\nCandidate:\n" + firstRunes(state.Candidate, maxDeciderContextRunes) + "\n")
	}
	if state.RuleText != "" {
		b.WriteString("\nStored rule:\n" + firstRunes(state.RuleText, maxDeciderContextRunes) + "\n")
	}
	if state.Task != "" {
		b.WriteString("\nTask prompt:\n" + firstRunes(state.Task, maxDeciderTaskRunes) + "\n")
	}
	return b.String()
}

// jevQuestions renders the wire questions: noul carries instructions only,
// choice carries its options mapped to short descriptions (at most 300
// characters each for the /decisions shape).
func jevQuestions(questions []DeciderQuestion) map[string]jevWireQ {
	out := make(map[string]jevWireQ, len(questions))
	for _, q := range questions {
		w := jevWireQ{Instructions: jevInstructions(q.Name)}
		if typ, _ := QuestionType(q.Name); typ == QTypeChoice {
			w.Type = "choice"
			w.Criteria = jevCriteria(q.Name, q.Options)
		} else {
			w.Type = "noul"
		}
		out[q.Name] = w
	}
	return out
}

// jevInstructions is the question the model answers. Every text stays far
// under the endpoint's 2000-character instruction limit.
func jevInstructions(name string) string {
	switch name {
	case QuestionDurable:
		return "Does the user state a rule, preference or correction meant for future work, not only for this task? Give the probability that it does."
	case QuestionKind:
		return "What kind of durable statement is this?"
	case QuestionScopeHint:
		return "Who should this rule apply to?"
	case QuestionContradicts:
		return "Does the candidate contradict the stored rule? Give the probability that it does."
	case QuestionRelevant:
		return "Is the stored rule relevant to the task prompt? Give the probability that it is."
	}
	return name
}

// jevCriteria maps choice options to their descriptions. Unknown questions
// fall back to the option ids, so the call still carries descriptions.
func jevCriteria(name string, options []string) map[string]string {
	describe := map[string]string{}
	switch name {
	case QuestionKind:
		describe = map[string]string{
			KindPreference:  "A standing instruction about what the user likes or wants.",
			KindDecision:    "A settled choice the user made, with its reasons.",
			KindAntipattern: "Something the user says not to do.",
			KindFact:        "A note about what worked.",
			KindProcedure:   "A how-to correction about steps or workflow.",
		}
	case QuestionScopeHint:
		describe = map[string]string{
			"user_rule":    "A standing instruction for this user.",
			"project_rule": "Knowledge the whole project could use.",
			"situational":  "Only this spot, not future work.",
			"none":         "Nothing durable to keep.",
		}
	}
	out := make(map[string]string, len(options))
	for _, o := range options {
		if d, ok := describe[o]; ok {
			out[o] = d
			continue
		}
		out[o] = o
	}
	return out
}

// jevAnswer is one wire answer: noul carries a probability, choice carries
// the pick with probabilities and confidence, score carries the expected
// position on its scale with its legend, probabilities and confidence.
// Unknown fields (Laya's routing extras, extended usage) are ignored.
type jevAnswer struct {
	Type          string             `json:"type"`
	Noul          *float64           `json:"noul,omitempty"`
	Choice        string             `json:"choice,omitempty"`
	Score         *float64           `json:"score,omitempty"`
	Legend        json.RawMessage    `json:"legend,omitempty"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
	Confidence    *float64           `json:"confidence,omitempty"`
}

// jevResponse is the wire reply: answers keyed by question id.
type jevResponse struct {
	Answers map[string]jevAnswer `json:"answers"`
}

// parseJevReply parses and validates one endpoint reply. Answers for unasked
// questions fail the whole call; one out-of-range answer marks only itself
// unavailable.
func parseJevReply(model string, questions []DeciderQuestion, raw []byte) (DeciderResult, error) {
	res := DeciderResult{Answers: map[string]Answer{}, Model: model, Version: JevVersion}
	byName := map[string]DeciderQuestion{}
	for _, q := range questions {
		byName[q.Name] = q
	}
	var reply jevResponse
	if err := json.Unmarshal(bytes.TrimSpace(raw), &reply); err != nil {
		return DeciderResult{}, fmt.Errorf("%w: malformed jev reply", ErrDeciderUnavailable)
	}
	if reply.Answers == nil {
		return DeciderResult{}, fmt.Errorf("%w: jev reply holds no answers", ErrDeciderUnavailable)
	}
	for id := range reply.Answers {
		if _, asked := byName[id]; !asked {
			return DeciderResult{}, fmt.Errorf("%w: answer for unasked question %q", ErrDeciderUnavailable, id)
		}
	}
	for _, q := range questions {
		a, ok := reply.Answers[q.Name]
		if !ok {
			res.Answers[q.Name] = Answer{Question: q.Name, Available: false}
			continue
		}
		res.Answers[q.Name] = jevResultAnswer(q, a)
	}
	return res, nil
}

// jevResultAnswer maps one wire answer onto the result type. A noul answer
// carries the probability; a choice answer carries the pick, its
// probabilities and confidence; a score answer normalises its expected
// position on the scale to 0..1 (score/(levels-1)), so it reads as a
// probability alongside the others. A wire type the question cannot use, or
// any out-of-range value, is that answer unavailable, never a yes.
func jevResultAnswer(q DeciderQuestion, a jevAnswer) Answer {
	typ, _ := QuestionType(q.Name)
	conf := 0.0
	if a.Confidence != nil {
		conf = *a.Confidence
	}
	bad := Answer{Question: q.Name, Available: false}
	inRange := func(f float64) bool { return f >= 0 && f <= 1 }
	switch a.Type {
	case QTypeNoul:
		if typ != QTypeNoul || a.Noul == nil || !inRange(*a.Noul) {
			return bad
		}
		return Answer{Question: q.Name, Available: true, Prob: *a.Noul}
	case QTypeChoice:
		if typ != QTypeChoice {
			return bad
		}
		ans := Answer{Question: q.Name, Available: true, Choice: a.Choice, Probabilities: a.Probabilities, Confidence: conf}
		if err := ValidateAnswer(q, ans); err != nil {
			return bad
		}
		return ans
	case "score":
		if typ != QTypeNoul || a.Score == nil {
			return bad
		}
		levels := jevLegendLevels(a.Legend)
		if levels < 2 && len(a.Probabilities) >= 2 {
			levels = len(a.Probabilities)
		}
		if levels < 2 || *a.Score < 0 || *a.Score > float64(levels-1) {
			return bad
		}
		for _, p := range a.Probabilities {
			if !inRange(p) {
				return bad
			}
		}
		if a.Confidence != nil && !inRange(conf) {
			return bad
		}
		return Answer{Question: q.Name, Available: true, Prob: *a.Score / float64(levels-1), Probabilities: a.Probabilities, Confidence: conf}
	}
	return bad
}

// jevLegendLevels counts a score legend, which the endpoints return either
// as an ordered array of level descriptions or as an index-keyed object.
// Anything else counts zero, which rejects the answer above.
func jevLegendLevels(raw json.RawMessage) int {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return 0
	}
	var arr []any
	if err := json.Unmarshal(raw, &arr); err == nil {
		return len(arr)
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err == nil {
		return len(obj)
	}
	return 0
}
