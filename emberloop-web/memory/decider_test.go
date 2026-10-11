package memory

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestValidateQuestionsRejectsBadLists(t *testing.T) {
	good := []DeciderQuestion{Noul(QuestionDurable), KindQuestion(), ScopeQuestion()}
	if err := ValidateQuestions(good); err != nil {
		t.Fatalf("good questions rejected: %v", err)
	}
	bad := [][]DeciderQuestion{
		nil,
		{},
		{Noul(QuestionDurable), Noul(QuestionDurable)},
		{Noul("tomorrow")},
		{Choice(QuestionKind, KindPreference)},
		{Choice(QuestionKind, KindPreference, KindDecision, KindAntipattern, KindFact, KindProcedure, "essay", "note", "memo")},
		{Choice(QuestionKind, KindOptions[:4]...)},
		{Choice(QuestionScopeHint, ScopeOptions[:3]...)},
		{Noul(QuestionDurable), Noul(QuestionContradicts), Noul(QuestionRelevant), KindQuestion(), ScopeQuestion(), Noul(QuestionDurable), KindQuestion()},
	}
	for i, qs := range bad {
		if err := ValidateQuestions(qs); !errors.Is(err, ErrDeciderUnavailable) {
			t.Errorf("list %d err = %v, want ErrDeciderUnavailable", i, err)
		}
	}
}

func TestValidateAnswerRejectsOutOfRange(t *testing.T) {
	durable := Noul(QuestionDurable)
	if err := ValidateAnswer(durable, Answer{Question: durable.Name, Prob: 0.8}); err != nil {
		t.Fatalf("good noul rejected: %v", err)
	}
	if err := ValidateAnswer(durable, Answer{Question: durable.Name, Prob: 1.5}); !errors.Is(err, ErrDeciderUnavailable) {
		t.Errorf("noul 1.5 err = %v, want unavailable", err)
	}
	if err := ValidateAnswer(durable, Answer{Question: durable.Name, Prob: -0.1}); !errors.Is(err, ErrDeciderUnavailable) {
		t.Errorf("noul -0.1 err = %v, want unavailable", err)
	}
	kind := KindQuestion()
	goodChoice := Answer{Question: kind.Name, Choice: KindPreference, Probabilities: map[string]float64{KindPreference: 1}, Confidence: 0.7}
	if err := ValidateAnswer(kind, goodChoice); err != nil {
		t.Fatalf("good choice rejected: %v", err)
	}
	unknown := goodChoice
	unknown.Choice = "essay"
	if err := ValidateAnswer(kind, unknown); !errors.Is(err, ErrDeciderUnavailable) {
		t.Errorf("unknown choice err = %v, want unavailable", err)
	}
	badProb := goodChoice
	badProb.Probabilities = map[string]float64{KindPreference: 2}
	if err := ValidateAnswer(kind, badProb); !errors.Is(err, ErrDeciderUnavailable) {
		t.Errorf("bad probability err = %v, want unavailable", err)
	}
	badConf := goodChoice
	badConf.Confidence = -1
	if err := ValidateAnswer(kind, badConf); !errors.Is(err, ErrDeciderUnavailable) {
		t.Errorf("bad confidence err = %v, want unavailable", err)
	}
}

func TestHeuristicDeciderDurableFollowsThePass(t *testing.T) {
	d := HeuristicDecider{}
	if d.Model() == "" || d.Version() == "" {
		t.Fatal("heuristic decider needs a model and version for decider_json")
	}
	for _, c := range heuristicGoldens {
		res, err := d.Decide(context.Background(), DeciderState{Statement: c.text}, []DeciderQuestion{Noul(QuestionDurable)})
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		prob, ok := res.Noul(QuestionDurable)
		if !ok {
			t.Fatalf("%s: durable unanswered", c.name)
		}
		if durable := prob > 0; durable != c.want {
			t.Errorf("%s: durable %v (prob %v), want %v", c.name, durable, prob, c.want)
		}
	}
}

func TestHeuristicDeciderKindMapping(t *testing.T) {
	d := HeuristicDecider{}
	cases := []struct{ text, want string }{
		{"No, never return wrapped errors in this repo", KindAntipattern},
		{"don't add tests unless asked", KindAntipattern},
		{"we decided to use tabs for this repo", KindDecision},
		{"No, use tabs here", KindPreference},
		{"remember: always wrap errors in this repo", KindPreference},
		{"only change what I asked, nothing else", KindPreference},
		{"that's wrong, the retry belongs in the caller", KindProcedure},
		{"Perfect! That's exactly what I wanted for the upload handler flow", KindFact},
	}
	for _, c := range cases {
		res, err := d.Decide(context.Background(), DeciderState{Statement: c.text}, []DeciderQuestion{KindQuestion()})
		if err != nil {
			t.Fatal(err)
		}
		got, ok := res.Choice(QuestionKind)
		if !ok || got != c.want {
			t.Errorf("kind(%q) = %q, want %q", c.text, got, c.want)
		}
	}
}

func TestHeuristicDeciderScopeMapping(t *testing.T) {
	d := HeuristicDecider{}
	cases := []struct{ text, want string }{
		{"No, always use tabs in this repo", "project_rule"},
		{"No, use tabs from now on", "user_rule"},
		{"No, use tabs here for now", "situational"},
		{"No, use tabs", "user_rule"},
	}
	for _, c := range cases {
		res, err := d.Decide(context.Background(), DeciderState{Statement: c.text}, []DeciderQuestion{ScopeQuestion()})
		if err != nil {
			t.Fatal(err)
		}
		got, ok := res.Choice(QuestionScopeHint)
		if !ok || got != c.want {
			t.Errorf("scope(%q) = %q, want %q", c.text, got, c.want)
		}
	}
	// Nothing durable is scope none.
	res, err := d.Decide(context.Background(), DeciderState{Statement: "ok"}, []DeciderQuestion{Noul(QuestionDurable), ScopeQuestion()})
	if err != nil {
		t.Fatal(err)
	}
	if p, _ := res.Noul(QuestionDurable); p != 0 {
		t.Errorf("durable(ok) = %v, want 0", p)
	}
	if got, _ := res.Choice(QuestionScopeHint); got != "none" {
		t.Errorf("scope(ok) = %q, want none", got)
	}
}

func TestHeuristicDeciderContradictsAndRelevant(t *testing.T) {
	d := HeuristicDecider{}
	ctx := context.Background()
	contra, err := d.Decide(ctx, DeciderState{
		Candidate: "always return wrapped errors",
		RuleText:  "never return wrapped errors",
	}, []DeciderQuestion{Noul(QuestionContradicts)})
	if err != nil {
		t.Fatal(err)
	}
	if p, _ := contra.Noul(QuestionContradicts); p != 1 {
		t.Errorf("contradicts = %v, want 1", p)
	}
	same, err := d.Decide(ctx, DeciderState{
		Candidate: "use tabs for indentation",
		RuleText:  "close the response body",
	}, []DeciderQuestion{Noul(QuestionContradicts)})
	if err != nil {
		t.Fatal(err)
	}
	if p, _ := same.Noul(QuestionContradicts); p != 0 {
		t.Errorf("contradicts of unrelated = %v, want 0", p)
	}
	rel, err := d.Decide(ctx, DeciderState{
		Task:     "fix the upload handler retry",
		RuleText: "the upload handler owns retries",
	}, []DeciderQuestion{Noul(QuestionRelevant)})
	if err != nil {
		t.Fatal(err)
	}
	if p, _ := rel.Noul(QuestionRelevant); p != 1 {
		t.Errorf("relevant = %v, want 1", p)
	}
	irrel, err := d.Decide(ctx, DeciderState{
		Task:     "fix the upload handler retry",
		RuleText: "use tabs for indentation",
	}, []DeciderQuestion{Noul(QuestionRelevant)})
	if err != nil {
		t.Fatal(err)
	}
	if p, _ := irrel.Noul(QuestionRelevant); p != 0 {
		t.Errorf("relevant of unrelated = %v, want 0", p)
	}
}

func llmReply(answers string) string {
	return `{"answers":[` + answers + `]}`
}

func TestLLMDeciderParsesGoodReplies(t *testing.T) {
	ctx := context.Background()
	d := LLMDecider{ModelName: "cheap", Call: func(_ context.Context, model, system, user string) (string, error) {
		return llmReply(`{"question":"durable","noul":0.8},{"question":"kind","choice":"preference","probabilities":{"preference":0.7,"decision":0.1,"antipattern":0.1,"fact":0.05,"procedure":0.05},"confidence":0.7}`), nil
	}}
	res, err := d.Decide(ctx, DeciderState{Statement: "No, use tabs"}, []DeciderQuestion{Noul(QuestionDurable), KindQuestion()})
	if err != nil {
		t.Fatal(err)
	}
	if p, ok := res.Noul(QuestionDurable); !ok || p != 0.8 {
		t.Errorf("durable = %v, want 0.8", p)
	}
	if got, ok := res.Choice(QuestionKind); !ok || got != KindPreference {
		t.Errorf("kind = %q, want preference", got)
	}
	if res.Model != "cheap" || res.Version == "" {
		t.Errorf("result = %q %q, want the model and a version", res.Model, res.Version)
	}
}

func TestLLMDeciderRejectsBadOutputAsUnavailable(t *testing.T) {
	ctx := context.Background()
	qs := []DeciderQuestion{Noul(QuestionDurable), KindQuestion()}
	cases := []struct {
		name  string
		call  func(context.Context, string, string, string) (string, error)
		check func(t *testing.T, res DeciderResult, err error)
	}{
		{"malformed fails the call", func(_ context.Context, _, _, _ string) (string, error) {
			return "not json", nil
		}, func(t *testing.T, _ DeciderResult, err error) {
			t.Helper()
			if !errors.Is(err, ErrDeciderUnavailable) {
				t.Errorf("err = %v, want unavailable", err)
			}
		}},
		{"unasked answer fails the call", func(_ context.Context, _, _, _ string) (string, error) {
			return llmReply(`{"question":"durable","noul":0.5},{"question":"relevant","noul":0.5}`), nil
		}, func(t *testing.T, _ DeciderResult, err error) {
			t.Helper()
			if !errors.Is(err, ErrDeciderUnavailable) {
				t.Errorf("err = %v, want unavailable", err)
			}
		}},
		{"out-of-range noul is that answer unavailable, never a yes", func(_ context.Context, _, _, _ string) (string, error) {
			return llmReply(`{"question":"durable","noul":7},{"question":"kind","choice":"preference","probabilities":{"preference":0.7,"decision":0.1,"antipattern":0.1,"fact":0.05,"procedure":0.05},"confidence":0.7}`), nil
		}, func(t *testing.T, res DeciderResult, err error) {
			t.Helper()
			if err != nil {
				t.Fatalf("err = %v, want per-answer unavailable", err)
			}
			if _, ok := res.Noul(QuestionDurable); ok {
				t.Error("out-of-range durable answered, want unavailable")
			}
			if got, ok := res.Choice(QuestionKind); !ok || got != KindPreference {
				t.Errorf("kind = %q, want the good answer kept", got)
			}
		}},
		{"unknown choice is unavailable", func(_ context.Context, _, _, _ string) (string, error) {
			return llmReply(`{"question":"durable","noul":0.9},{"question":"kind","choice":"essay","probabilities":{"essay":1},"confidence":0.9}`), nil
		}, func(t *testing.T, res DeciderResult, err error) {
			t.Helper()
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if _, ok := res.Choice(QuestionKind); ok {
				t.Error("unknown choice answered, want unavailable")
			}
			if p, ok := res.Noul(QuestionDurable); !ok || p != 0.9 {
				t.Errorf("durable = %v, want the good answer kept", p)
			}
		}},
		{"missing answer is unavailable", func(_ context.Context, _, _, _ string) (string, error) {
			return llmReply(`{"question":"durable","noul":0.9}`), nil
		}, func(t *testing.T, res DeciderResult, err error) {
			t.Helper()
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if _, ok := res.Choice(QuestionKind); ok {
				t.Error("missing kind answered, want unavailable")
			}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := LLMDecider{ModelName: "cheap", Call: c.call}
			res, err := d.Decide(ctx, DeciderState{Statement: "No, use tabs"}, qs)
			c.check(t, res, err)
		})
	}
}

func TestLLMDeciderRedactsBeforeTheCall(t *testing.T) {
	ctx := context.Background()
	var gotUser string
	d := LLMDecider{ModelName: "cheap", Call: func(_ context.Context, _, _ string, user string) (string, error) {
		gotUser = user
		return llmReply(`{"question":"durable","noul":0.9}`), nil
	}}
	statement := "remember: the password is hunter2-2026, always use tabs"
	if _, err := d.Decide(ctx, DeciderState{Statement: statement}, []DeciderQuestion{Noul(QuestionDurable)}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(gotUser, "hunter2") {
		t.Errorf("llm input holds the secret raw: %q", gotUser)
	}
	if !strings.Contains(gotUser, "[redacted:password]") {
		t.Errorf("llm input lacks the redaction marker: %q", gotUser)
	}
}

func TestLLMDeciderNeedsAModel(t *testing.T) {
	d := LLMDecider{Call: func(context.Context, string, string, string) (string, error) {
		return llmReply(`{"question":"durable","noul":1}`), nil
	}}
	if _, err := d.Decide(context.Background(), DeciderState{Statement: "No, use tabs"}, []DeciderQuestion{Noul(QuestionDurable)}); !errors.Is(err, ErrDeciderUnavailable) {
		t.Errorf("model-less llm err = %v, want unavailable", err)
	}
}

func TestEnsembleThresholds(t *testing.T) {
	ctx := context.Background()
	state := DeciderState{Statement: "No, use tabs here"}
	cases := []struct {
		name      string
		second    float64
		unavail   bool
		verdict   string
		eligible  bool
		confirmed bool
	}{
		{"below low drops", 0.1, false, VerdictDrop, false, true},
		{"between stores an observation", 0.5, false, VerdictObserve, false, true},
		{"at high is queue-eligible", 0.7, false, VerdictEligible, true, true},
		{"above high is queue-eligible", 0.95, false, VerdictEligible, true, true},
		{"unavailable second stores without eligibility, never a yes", 0.95, true, VerdictObserve, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := EnsembleDecider{First: HeuristicDecider{}, Second: &ScriptedDecider{Durable: c.second, Unavailable: c.unavail}}
			got, err := e.Judge(ctx, state)
			if err != nil {
				t.Fatal(err)
			}
			if !got.Proposed || got.Verdict != c.verdict || got.QueueEligible != c.eligible || got.Confirmed != c.confirmed {
				t.Errorf("verdict = %+v, want %s eligible %v confirmed %v", got, c.verdict, c.eligible, c.confirmed)
			}
		})
	}
}

func TestEnsembleDropsUnproposed(t *testing.T) {
	e := EnsembleDecider{First: HeuristicDecider{}, Second: &ScriptedDecider{Durable: 0.99}}
	got, err := e.Judge(context.Background(), DeciderState{Statement: "ok"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Proposed || got.Verdict != VerdictDrop || got.QueueEligible {
		t.Errorf("unproposed verdict = %+v, want a drop", got)
	}
}

func TestEnsembleRejectsBadThresholds(t *testing.T) {
	e := EnsembleDecider{First: HeuristicDecider{}, Second: &ScriptedDecider{Durable: 1}, Low: 0.9, High: 0.1}
	if _, err := e.Judge(context.Background(), DeciderState{Statement: "No, use tabs"}); !errors.Is(err, ErrDeciderUnavailable) {
		t.Errorf("err = %v, want unavailable", err)
	}
}

func TestScriptedDeciderDrivesTests(t *testing.T) {
	s := &ScriptedDecider{Durable: 0.6, Kind: KindFact, Scope: "situational"}
	res, err := s.Decide(context.Background(), DeciderState{}, []DeciderQuestion{Noul(QuestionDurable), KindQuestion(), ScopeQuestion(), Noul(QuestionContradicts), Noul(QuestionRelevant)})
	if err != nil {
		t.Fatal(err)
	}
	if p, _ := res.Noul(QuestionDurable); p != 0.6 {
		t.Errorf("durable = %v", p)
	}
	if got, _ := res.Choice(QuestionKind); got != KindFact {
		t.Errorf("kind = %q", got)
	}
	if got, _ := res.Choice(QuestionScopeHint); got != "situational" {
		t.Errorf("scope = %q", got)
	}
	if s.Calls != 1 || s.Model() == "" || s.Version() == "" {
		t.Errorf("scripted = calls %d model %q version %q, want counted calls and names", s.Calls, s.Model(), s.Version())
	}
	s.Unavailable = true
	res, err = s.Decide(context.Background(), DeciderState{}, []DeciderQuestion{Noul(QuestionDurable)})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := res.Noul(QuestionDurable); ok {
		t.Error("unavailable script answered, want silence")
	}
	var doc map[string]any
	res2, _ := (&ScriptedDecider{Durable: 1}).Decide(context.Background(), DeciderState{}, []DeciderQuestion{Noul(QuestionDurable)})
	raw, _ := json.Marshal(res2)
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if doc["Model"] == "" || doc["Version"] == "" {
		t.Errorf("result serializes without model/version: %s", raw)
	}
}
