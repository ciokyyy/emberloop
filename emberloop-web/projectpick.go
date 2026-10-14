package web

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"

	"example.com/emberloop-web/el/tools/systemone"
)

const (
	projectPickNoneID    = "none"
	projectPickMinTop    = 0.5
	projectPickMinMargin = 0.2
	projectPickEpsilon   = 1e-12
)

type projectPickCandidate struct {
	Name string `json:"name"`
	Gist string `json:"gist"`
}

type projectPickState struct {
	FirstPrompt string                          `json:"first_prompt"`
	Projects    map[string]projectPickCandidate `json:"projects"`
}

type projectPickResult struct {
	SuggestedProject *Project `json:"suggestedProject"`
}

// handleProjectPick evaluates the first prompt of a projectless draft without
// starting a Session. A nil suggestion is the safe result for a general prompt,
// a low-confidence answer, a Jev error, or a missing OpenRouter key.
func (d *Daemon) handleProjectPick(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Prompt string `json:"prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, projectPickResult{SuggestedProject: d.suggestProject(r.Context(), req.Prompt)})
}

func (d *Daemon) suggestProject(ctx context.Context, prompt string) *Project {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return nil
	}

	d.projMu.Lock()
	projects := append([]Project(nil), d.projects...)
	d.projMu.Unlock()
	if len(projects) == 0 {
		return nil
	}

	keyFn := d.projectPickKeyFn
	if keyFn == nil {
		keyFn = systemone.APIKey
	}
	key := keyFn(ctx)
	if strings.TrimSpace(key) == "" {
		return nil
	}

	stateProjects := make(map[string]projectPickCandidate, len(projects))
	questionProjects := make(map[string]Project, len(projects))
	questions := make(map[string]systemone.Question, len(projects)+1)
	for i, project := range projects {
		id := fmt.Sprintf("project_%d", i)
		stateProjects[id] = projectPickCandidate{Name: project.Name, Gist: project.Gist}
		questionProjects[id] = project
		questions[id] = systemone.Question{
			Type:         systemone.TypeNoul,
			Instructions: fmt.Sprintf("Is the user's first message (see `first_prompt`) a task that belongs in the Project named %q (see its entry in `projects`)?", project.Name),
			Criteria: map[string]string{
				"true":  "The first message asks about, changes, or directly concerns work in that Project",
				"false": "The message concerns another Project, or is a general question unrelated to the listed Projects",
			},
		}
	}
	questions[projectPickNoneID] = systemone.Question{
		Type:         systemone.TypeNoul,
		Instructions: "Is the user's first message (see `first_prompt`) unrelated to every listed Project (see `projects`), so it belongs in none of them?",
		Criteria: map[string]string{
			"true":  "The message is a general question or concerns something else entirely",
			"false": "The message asks about, changes, or directly concerns work in one of the listed Projects",
		},
	}

	provider := d.projectPickProvider
	if provider == nil {
		provider = systemone.NewOpenRouter(key)
	}
	response, err := provider.Evaluate(ctx, systemone.Request{
		Model:     systemone.JevModelPinned,
		State:     projectPickState{FirstPrompt: prompt, Projects: stateProjects},
		Questions: questions,
	})
	if err != nil {
		return nil
	}

	bestID := ""
	best, second := -1.0, -1.0
	for id := range questions {
		answer, ok := response.Answers[id]
		if !ok || answer.Type != systemone.TypeNoul || answer.Noul == nil {
			return nil
		}
		probability := *answer.Noul
		if math.IsNaN(probability) || math.IsInf(probability, 0) || probability < 0 || probability > 1 {
			return nil
		}
		if probability > best {
			bestID, second, best = id, best, probability
		} else if probability > second {
			second = probability
		}
	}
	if bestID == projectPickNoneID || best < projectPickMinTop || best-second < projectPickMinMargin-projectPickEpsilon {
		return nil
	}

	project, ok := questionProjects[bestID]
	if !ok {
		return nil
	}
	// Use current metadata so a rename made while Jev was evaluating appears
	// in the banner; deletion during the call safely turns into no suggestion.
	current, ok := d.projectByID(project.ID)
	if !ok {
		return nil
	}
	return &current
}
