package memory

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// jevScript is one fake endpoint reply: the status and body the server
// answers every call with, recording the request it saw.
type jevScript struct {
	status int
	body   string
	hits   int
	state  string
	auth   string
	model  string
}

func jevServer(t *testing.T, s *jevScript) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.hits++
		s.auth = r.Header.Get("Authorization")
		var req struct {
			Model     string         `json:"model"`
			State     string         `json:"state"`
			Questions map[string]any `json:"questions"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		s.model, s.state = req.Model, req.State
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(s.status)
		_, _ = w.Write([]byte(s.body))
	}))
}

func TestJevDeciderParsesNoulChoiceAndScore(t *testing.T) {
	s := &jevScript{status: http.StatusOK, body: `{"model":"jev-1.13","answers":{` +
		`"durable":{"type":"noul","noul":0.8},` +
		`"kind":{"type":"choice","choice":"preference","probabilities":{"preference":0.7,"decision":0.1,"antipattern":0.1,"fact":0.05,"procedure":0.05},"confidence":0.7},` +
		`"scope_hint":{"type":"choice","choice":"user_rule","probabilities":{"user_rule":0.9,"project_rule":0.05,"situational":0.05,"none":0},"confidence":0.9}` +
		`},"usage":{"input_tokens":10,"output_tokens":2}}`}
	srv := jevServer(t, s)
	defer srv.Close()
	d := JevDecider{Config: JevConfig{BaseURL: srv.URL, APIKey: "k", Model: "jev-latest"}}
	qs := []DeciderQuestion{Noul(QuestionDurable), KindQuestion(), ScopeQuestion()}
	res, err := d.Decide(context.Background(), DeciderState{Statement: "No, use tabs"}, qs)
	if err != nil {
		t.Fatal(err)
	}
	if p, ok := res.Noul(QuestionDurable); !ok || p != 0.8 {
		t.Errorf("durable = %v, want 0.8", p)
	}
	if got, ok := res.Choice(QuestionKind); !ok || got != KindPreference {
		t.Errorf("kind = %q, want preference", got)
	}
	if got, ok := res.Choice(QuestionScopeHint); !ok || got != "user_rule" {
		t.Errorf("scope = %q, want user_rule", got)
	}
	if res.Model != "jev-latest" || res.Version == "" {
		t.Errorf("result = %q %q, want the model and a version", res.Model, res.Version)
	}
	if s.auth != "Bearer k" {
		t.Errorf("auth = %q, want the bearer key", s.auth)
	}
	if s.model != "jev-latest" {
		t.Errorf("model sent = %q, want jev-latest", s.model)
	}
	// A score answer normalises its expected position to 0..1: score 1.5 on
	// three levels reads as 0.75.
	s2 := &jevScript{status: http.StatusOK, body: `{"model":"jev-1.13","answers":{` +
		`"durable":{"type":"score","score":1.5,"legend":["low","mid","high"],"probabilities":{"0":0.1,"1":0.3,"2":0.6},"confidence":0.6}` +
		`}}`}
	srv2 := jevServer(t, s2)
	defer srv2.Close()
	d2 := JevDecider{Config: JevConfig{BaseURL: srv2.URL}}
	res2, err := d2.Decide(context.Background(), DeciderState{Statement: "No, use tabs"}, []DeciderQuestion{Noul(QuestionDurable)})
	if err != nil {
		t.Fatal(err)
	}
	if p, ok := res2.Noul(QuestionDurable); !ok || p != 0.75 {
		t.Errorf("score durable = %v, want 0.75", p)
	}
	if res2.Model != DefaultJevModel {
		t.Errorf("model = %q, want the default %q", res2.Model, DefaultJevModel)
	}
}

func TestJevDeciderMapsErrorsToUnavailable(t *testing.T) {
	qs := []DeciderQuestion{Noul(QuestionDurable)}
	for _, c := range []struct {
		name   string
		status int
		body   string
	}{
		{"unauthorized", http.StatusUnauthorized, `{"error":"bad key"}`},
		{"no credits", 402, `{"error":"no balance"}`},
		{"rejected body", http.StatusUnprocessableEntity, `{"error":"bad field"}`},
		{"rate limited", http.StatusTooManyRequests, `{"error":"slow down"}`},
		{"no answers", http.StatusBadGateway, `{"error":"upstream"}`},
		{"malformed body", http.StatusOK, `not json`},
		{"no answers map", http.StatusOK, `{"model":"jev-1.13"}`},
		{"unasked answer fails the call", http.StatusOK, `{"answers":{"relevant":{"type":"noul","noul":0.5}}}`},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := &jevScript{status: c.status, body: c.body}
			srv := jevServer(t, s)
			defer srv.Close()
			d := JevDecider{Config: JevConfig{BaseURL: srv.URL, APIKey: "k"}}
			if _, err := d.Decide(context.Background(), DeciderState{Statement: "No, use tabs"}, qs); !errors.Is(err, ErrDeciderUnavailable) {
				t.Errorf("err = %v, want unavailable", err)
			}
		})
	}
	// A timeout is unavailable, never a yes.
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
	}))
	defer slow.Close()
	d := JevDecider{Config: JevConfig{BaseURL: slow.URL, Client: &http.Client{Timeout: 50 * time.Millisecond}}}
	if _, err := d.Decide(context.Background(), DeciderState{Statement: "No, use tabs"}, qs); !errors.Is(err, ErrDeciderUnavailable) {
		t.Errorf("timeout err = %v, want unavailable", err)
	}
}

func TestJevDeciderEnforcesLimitsWithoutSending(t *testing.T) {
	s := &jevScript{status: http.StatusOK, body: `{"answers":{}}`}
	srv := jevServer(t, s)
	defer srv.Close()
	d := JevDecider{Config: JevConfig{BaseURL: srv.URL, APIKey: "k"}}
	state := DeciderState{Statement: "No, use tabs"}
	seven := []DeciderQuestion{Noul(QuestionDurable), KindQuestion(), ScopeQuestion(), Noul(QuestionContradicts), Noul(QuestionRelevant), Noul(QuestionDurable), KindQuestion()}
	if _, err := d.Decide(context.Background(), state, seven); !errors.Is(err, ErrDeciderUnavailable) {
		t.Errorf("7-question err = %v, want unavailable", err)
	}
	one := []DeciderQuestion{Choice(QuestionKind, KindPreference)}
	if _, err := d.Decide(context.Background(), state, one); !errors.Is(err, ErrDeciderUnavailable) {
		t.Errorf("1-option err = %v, want unavailable", err)
	}
	if s.hits != 0 {
		t.Errorf("endpoint hit %d times for invalid calls, want 0", s.hits)
	}
}

func TestJevDeciderNeverSendsWhenOff(t *testing.T) {
	d := JevDecider{}
	if _, err := d.Decide(context.Background(), DeciderState{Statement: "No, use tabs"}, []DeciderQuestion{Noul(QuestionDurable)}); !errors.Is(err, ErrDeciderUnavailable) {
		t.Errorf("off err = %v, want unavailable", err)
	}
	if d.Model() == "" || d.Version() == "" {
		t.Error("jev decider needs a model and version for decider_json")
	}
}

func TestJevDeciderRedactsAndCapsState(t *testing.T) {
	s := &jevScript{status: http.StatusOK, body: `{"answers":{"durable":{"type":"noul","noul":0.9}}}`}
	srv := jevServer(t, s)
	defer srv.Close()
	d := JevDecider{Config: JevConfig{BaseURL: srv.URL}}
	statement := "remember: the password is hunter2-2026, always use tabs. " + strings.Repeat("padding ", 2000)
	if _, err := d.Decide(context.Background(), DeciderState{Statement: statement}, []DeciderQuestion{Noul(QuestionDurable)}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(s.state, "hunter2") {
		t.Errorf("jev state holds the secret raw: %q", firstRunes(s.state, 200))
	}
	if !strings.Contains(s.state, "[redacted:password]") {
		t.Errorf("jev state lacks the redaction marker: %q", firstRunes(s.state, 200))
	}
	if n := len([]rune(s.state)); n > maxDeciderStatementRunes+512 {
		t.Errorf("jev state is %d runes, want it capped near %d", n, maxDeciderStatementRunes)
	}
}

func TestJevDeciderParsesLayaExtrasWithoutAnAdapter(t *testing.T) {
	// A Laya-style reply carries the contracted keys plus server extras
	// (extended usage, routing, per-answer calibration): the client reads
	// the contract and ignores the rest, so no adapter is needed.
	s := &jevScript{status: http.StatusOK, body: `{"model":"laya-rl-agent","answers":{` +
		`"durable":{"type":"noul","noul":0.85,"answer_confidence":0.9,"action":{"act_probability":1.0},"x_jev_confidence":0.88},` +
		`"kind":{"type":"choice","choice":"procedure","probabilities":{"preference":0,"decision":0,"antipattern":0,"fact":0.1,"procedure":0.9},"confidence":0.8}` +
		`},"usage":{"input_tokens":83,"output_tokens":0,"state_tokens":12},"routing":{"model":"english"}}`}
	srv := jevServer(t, s)
	defer srv.Close()
	d := JevDecider{Config: JevConfig{BaseURL: srv.URL + "/v1/systemone"}}
	res, err := d.Decide(context.Background(), DeciderState{Statement: "No, use tabs"}, []DeciderQuestion{Noul(QuestionDurable), KindQuestion()})
	if err != nil {
		t.Fatal(err)
	}
	if p, ok := res.Noul(QuestionDurable); !ok || p != 0.85 {
		t.Errorf("durable = %v, want 0.85", p)
	}
	if got, ok := res.Choice(QuestionKind); !ok || got != KindProcedure {
		t.Errorf("kind = %q, want procedure", got)
	}
}

func TestJevDeciderRejectsBadAnswersPerAnswer(t *testing.T) {
	qs := []DeciderQuestion{Noul(QuestionDurable), KindQuestion()}
	cases := []struct {
		name string
		body string
		want string // which question stays answered
	}{
		{"out-of-range noul silenced", `{"answers":{"durable":{"type":"noul","noul":7},"kind":{"type":"choice","choice":"preference","probabilities":{"preference":1},"confidence":1}}}`, QuestionKind},
		{"unknown choice silenced", `{"answers":{"durable":{"type":"noul","noul":0.9},"kind":{"type":"choice","choice":"essay","probabilities":{"essay":1},"confidence":1}}}`, QuestionDurable},
		{"choice for noul silenced", `{"answers":{"durable":{"type":"choice","choice":"x","probabilities":{"x":1},"confidence":1},"kind":{"type":"choice","choice":"preference","probabilities":{"preference":1},"confidence":1}}}`, QuestionKind},
		{"score out of range silenced", `{"answers":{"durable":{"type":"score","score":9,"legend":["a","b"],"probabilities":{"0":0.5,"1":0.5},"confidence":1},"kind":{"type":"choice","choice":"preference","probabilities":{"preference":1},"confidence":1}}}`, QuestionKind},
		{"missing answer silenced", `{"answers":{"durable":{"type":"noul","noul":0.9}}}`, QuestionDurable},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := &jevScript{status: http.StatusOK, body: c.body}
			srv := jevServer(t, s)
			defer srv.Close()
			d := JevDecider{Config: JevConfig{BaseURL: srv.URL}}
			res, err := d.Decide(context.Background(), DeciderState{Statement: "No, use tabs"}, qs)
			if err != nil {
				t.Fatal(err)
			}
			for _, q := range qs {
				_, ok := res.Answers[q.Name]
				if !ok {
					t.Errorf("question %q missing, want it present", q.Name)
					continue
				}
				answered := res.Answers[q.Name].Available
				if (q.Name == c.want) != answered {
					t.Errorf("question %q available = %v, want %v", q.Name, answered, q.Name == c.want)
				}
			}
		})
	}
}
