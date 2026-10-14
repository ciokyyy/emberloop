package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"example.com/emberloop-web/el/tools/systemone"
)

func TestProjectPickHTTPGateAndWording(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "test-key")
	projects := []Project{
		{ID: "alpha", Name: "Alpha", Path: "/private/projects/alpha", Gist: "Builds small solar-powered widgets."},
		{ID: "beta", Name: "Beta", Path: "/private/projects/beta", Gist: "Research notes about migration patterns."},
	}
	scores := map[string]float64{}
	var calls int
	var lastRequest systemone.Request
	var lastBody string
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodPost {
			t.Errorf("systemone method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("authorization = %q, want test key", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read systemone body: %v", err)
			return
		}
		lastBody = string(body)
		if err := json.Unmarshal(body, &lastRequest); err != nil {
			t.Errorf("decode systemone request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		answers := make(map[string]systemone.Answer, len(lastRequest.Questions))
		for id := range lastRequest.Questions {
			probability := scores[id]
			answers[id] = systemone.Answer{Type: systemone.TypeNoul, Noul: &probability}
		}
		_ = json.NewEncoder(w).Encode(systemone.Response{Answers: answers})
	}))
	defer endpoint.Close()

	d := NewDaemon(tempDir(t), NewFakeBackend())
	d.projects = projects
	// Use the production key lookup while redirecting only the HTTP endpoint.
	d.projectPickProvider = systemone.NewOpenRouterWithEndpoint("test-key", endpoint.URL)
	api := httptest.NewServer(authedHandler(t, d))
	defer api.Close()

	cases := []struct {
		name   string
		scores map[string]float64
		want   string
	}{
		{name: "accept at top and margin boundaries", scores: map[string]float64{"project_0": 0.5, "project_1": 0.3, projectPickNoneID: 0.1}, want: "alpha"},
		{name: "below top threshold", scores: map[string]float64{"project_0": 0.49, "project_1": 0.3, projectPickNoneID: 0.21}},
		{name: "below margin", scores: map[string]float64{"project_0": 0.7, "project_1": 0.51, projectPickNoneID: 0.1}},
		{name: "none wins", scores: map[string]float64{"project_0": 0.1, "project_1": 0.05, projectPickNoneID: 0.85}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			scores = tc.scores
			before := calls
			code, body := apiJSON(t, http.MethodPost, api.URL+"/api/projects/suggest", map[string]string{"prompt": "Fix the widget startup sequence."})
			if code != http.StatusOK {
				t.Fatalf("suggest = %d %s", code, body)
			}
			var result projectPickResult
			if err := json.Unmarshal(body, &result); err != nil {
				t.Fatal(err)
			}
			if calls != before+1 {
				t.Fatalf("systemone calls = %d, want exactly one call", calls-before)
			}
			if tc.want == "" {
				if result.SuggestedProject != nil {
					t.Fatalf("suggestion = %+v, want none", result.SuggestedProject)
				}
				return
			}
			if result.SuggestedProject == nil || result.SuggestedProject.ID != tc.want {
				t.Fatalf("suggestion = %+v, want Project %q", result.SuggestedProject, tc.want)
			}
			if lastRequest.Model != systemone.JevModelPinned {
				t.Errorf("model = %q, want %q", lastRequest.Model, systemone.JevModelPinned)
			}
			if len(lastRequest.Questions) != len(projects)+1 {
				t.Errorf("questions = %d, want one per Project plus none", len(lastRequest.Questions))
			}
			if q, ok := lastRequest.Questions[projectPickNoneID]; !ok || q.Type != systemone.TypeNoul || !strings.Contains(q.Instructions, "none") {
				t.Errorf("none question = %+v, want noul question for none", q)
			}
			for i, project := range projects {
				id := fmt.Sprintf("project_%d", i)
				q, ok := lastRequest.Questions[id]
				if !ok || q.Type != systemone.TypeNoul || !strings.Contains(q.Instructions, project.Name) {
					t.Errorf("question %q = %+v, want noul wording for Project %q", id, q, project.Name)
				}
			}
			state, ok := lastRequest.State.(map[string]any)
			if !ok {
				t.Fatalf("state = %T, want JSON object", lastRequest.State)
			}
			if state["first_prompt"] != "Fix the widget startup sequence." {
				t.Errorf("first_prompt = %v", state["first_prompt"])
			}
			candidates, ok := state["projects"].(map[string]any)
			if !ok || len(candidates) != 2 {
				t.Fatalf("projects state = %#v", state["projects"])
			}
			for i, project := range projects {
				entry, ok := candidates[fmt.Sprintf("project_%d", i)].(map[string]any)
				if !ok || entry["name"] != project.Name || entry["gist"] != project.Gist {
					t.Errorf("candidate %d = %#v, want name %q and gist %q", i, candidates[fmt.Sprintf("project_%d", i)], project.Name, project.Gist)
				}
				if strings.Contains(lastBody, project.Path) {
					t.Errorf("Project path %q was sent to Jev", project.Path)
				}
			}
		})
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.sessions) != 0 {
		t.Fatalf("suggestion started %d Sessions; pick must finish before Session creation", len(d.sessions))
	}
}

func TestProjectPickWithoutOpenRouterKeyReturnsNone(t *testing.T) {
	var calls int
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, _ = io.Copy(io.Discard, r.Body)
		_ = json.NewEncoder(w).Encode(systemone.Response{Answers: map[string]systemone.Answer{}})
	}))
	defer endpoint.Close()

	d := NewDaemon(tempDir(t), NewFakeBackend())
	d.projects = []Project{{ID: "alpha", Name: "Alpha", Path: "/private/projects/alpha", Gist: "Builds solar-powered widgets."}}
	d.projectPickKeyFn = func(context.Context) string { return "" }
	d.projectPickProvider = systemone.NewOpenRouterWithEndpoint("unused", endpoint.URL)
	api := httptest.NewServer(authedHandler(t, d))
	defer api.Close()

	code, body := apiJSON(t, http.MethodPost, api.URL+"/api/projects/suggest", map[string]string{"prompt": "Fix the widget startup sequence."})
	if code != http.StatusOK {
		t.Fatalf("suggest = %d %s", code, body)
	}
	var result projectPickResult
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatal(err)
	}
	code, body = apiJSON(t, http.MethodPost, api.URL+"/api/sessions", map[string]string{"harness": "fake", "model": "fake-model"})
	if code != http.StatusCreated {
		t.Fatalf("draft session without a project = %d %s", code, body)
	}
	var session SessionMeta
	if err := json.Unmarshal(body, &session); err != nil {
		t.Fatal(err)
	}
	if session.ProjectID != "" || session.WorkDir != d.users.homeDir("admin") {
		t.Fatalf("draft session = %+v, want today's unprojected home-directory behavior", session)
	}
	if result.SuggestedProject != nil || calls != 0 {
		t.Fatalf("result = %+v, calls = %d; no key must mean no Jev call and no suggestion", result, calls)
	}
}
