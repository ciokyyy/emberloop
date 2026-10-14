package web

import (
	"context"
	"fmt"
	"os"
	"sort"
	"testing"
	"time"

	"example.com/emberloop-web/el/tools/systemone"
)

// TestLiveJevProjectPick measures which project-description wording best picks
// a Project for a user's FIRST message through OpenRouter's systemone endpoint
// (model typesafe/jev-1.13) via the existing tools/systemone client.
//
// Design: ONE call per prompt. Each call carries one noul question per project
// (id = project name) asking the probability the prompt belongs in that
// project, plus a "none" noul question for "none of these". The predicted
// project is the argmax over all answers. Three variants change only what each
// project entry in State carries: names only, name+path, name+gist. The
// Instructions/Criteria shape is identical across variants and follows the
// find cascade's conventions (cascade.go stage2): the question names the
// candidate in backticks, points at its State entry, and carries true/false
// criteria.
//
// Budget: HARD cap of 100 Evaluate calls total (25 prompts x 3 variants = 75
// planned, plus one retry per call at most). The counter aborts past 100.
// Each call bills like any other Jev call. Runs only with EL_LIVE_JEV_PICK=1;
// without it (or without a key from systemone.APIKey: OPENROUTER_API_KEY else
// pig auth) it skips, so the repo check never touches the network. The key
// travels only in the Authorization header and is never logged.
func TestLiveJevProjectPick(t *testing.T) {
	if os.Getenv("EL_LIVE_JEV_PICK") == "" {
		t.Skip("set EL_LIVE_JEV_PICK=1 to run the project-pick wording experiment")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	key := systemone.APIKey(ctx)
	if key == "" {
		t.Skip("no OpenRouter key: set OPENROUTER_API_KEY or pig auth for openrouter")
	}
	provider := systemone.NewOpenRouter(key)

	const pickBudget = 100
	var calls int
	evaluate := func(req systemone.Request) (systemone.Response, time.Duration) {
		for attempt := 0; ; attempt++ {
			calls++
			if calls > pickBudget {
				t.Fatalf("pick budget exceeded: %d calls past the %d-call cap", calls, pickBudget)
			}
			start := time.Now()
			resp, err := provider.Evaluate(ctx, req)
			latency := time.Since(start)
			if err == nil {
				return resp, latency
			}
			if attempt == 0 && ctx.Err() == nil {
				t.Logf("pick call %d failed (%v), retrying once", calls, err)
				continue
			}
			t.Fatalf("pick call %d: %v", calls, err)
			panic("unreachable")
		}
	}

	type scored struct {
		prompt  string
		want    string
		got     string
		top     float64
		second  float64
		ms      int64
		cost    float64
		hasCost bool
	}
	for _, v := range pickVariants {
		var (
			results           []scored
			correct           int
			noneTotal, noneFP int
			sumMs             int64
			sumCost           float64
			costCalls         int
		)
		for _, p := range pickPrompts {
			projects := make(map[string]any, len(pickProjects))
			questions := make(map[string]systemone.Question, len(pickProjects)+1)
			for _, proj := range pickProjects {
				projects[proj.name] = v.entry(proj)
				questions[proj.name] = systemone.Question{
					Type:         systemone.TypeNoul,
					Instructions: fmt.Sprintf("Is the user's first message (see `first_prompt`) a task that belongs in the `%s` project (see its entry in `projects`)?", proj.name),
					Criteria: map[string]string{
						"true":  "The first message asks about, changes, or directly concerns work in that project",
						"false": "The message concerns a different project, or is a general question unrelated to any listed project",
					},
				}
			}
			questions[pickNoneID] = systemone.Question{
				Type:         systemone.TypeNoul,
				Instructions: "Is the user's first message (see `first_prompt`) unrelated to every listed project (see `projects`), so it belongs in none of them?",
				Criteria: map[string]string{
					"true":  "The message is a general question or concerns something else entirely",
					"false": "The message asks about, changes, or directly concerns work in one of the listed projects",
				},
			}
			req := systemone.Request{
				Model:     systemone.JevModelPinned,
				State:     map[string]any{"first_prompt": p.text, "projects": projects},
				Questions: questions,
			}
			resp, latency := evaluate(req)
			probs := make(map[string]float64, len(questions))
			for id := range questions {
				probs[id] = 0
				if ans, ok := resp.Answers[id]; ok && ans.Noul != nil {
					probs[id] = *ans.Noul
				}
			}
			ids := make([]string, 0, len(probs))
			for id := range probs {
				ids = append(ids, id)
			}
			sort.Slice(ids, func(i, j int) bool {
				if probs[ids[i]] == probs[ids[j]] {
					return ids[i] < ids[j]
				}
				return probs[ids[i]] > probs[ids[j]]
			})
			got, top := ids[0], probs[ids[0]]
			second := probs[ids[1]]
			s := scored{prompt: p.text, want: p.want, got: got, top: top, second: second, ms: latency.Milliseconds()}
			if resp.Usage != nil {
				s.cost, s.hasCost = resp.Usage.Cost, true
				sumCost += resp.Usage.Cost
				costCalls++
			}
			results = append(results, s)
			sumMs += s.ms
			if got == p.want {
				correct++
			}
			if p.want == pickNoneID {
				noneTotal++
				if got != pickNoneID {
					noneFP++
				}
			}
		}
		n := float64(len(results))
		meanCost := 0.0
		if costCalls > 0 {
			meanCost = sumCost / float64(costCalls)
		}
		t.Logf("variant %s: top-1 accuracy %.3f (%d/%d), none-FP %.3f (%d/%d), mean latency %dms, mean cost $%.6f/call over %d/%d calls with usage",
			v.name, float64(correct)/n, correct, len(results),
			float64(noneFP)/float64(pickMax(noneTotal, 1)), noneFP, noneTotal,
			sumMs/int64(len(results)), meanCost, costCalls, len(results))
		// Offline threshold sweep over (top-probability, margin-over-second):
		// no extra calls. Best = highest accepted accuracy, ties broken by
		// higher coverage.
		bestAcc, bestCover := -1.0, -1.0
		var bestTop, bestMargin float64
		for _, topThr := range []float64{0, 0.5, 0.6, 0.7, 0.8, 0.9} {
			for _, marginThr := range []float64{0, 0.1, 0.2, 0.3} {
				var acc, abst int
				for _, r := range results {
					if r.top < topThr || r.top-r.second < marginThr {
						abst++
						continue
					}
					if r.got == r.want {
						acc++
					}
				}
				denom := len(results) - abst
				rate := 0.0
				if denom > 0 {
					rate = float64(acc) / float64(denom)
				}
				cover := float64(denom) / n
				t.Logf("variant %s accept(top>=%.1f, margin>=%.1f): accepted-accuracy %.3f (%d/%d), abstention %.3f (%d/%d)",
					v.name, topThr, marginThr, rate, acc, denom, float64(abst)/n, abst, len(results))
				if rate > bestAcc || (rate == bestAcc && cover > bestCover) {
					bestAcc, bestCover, bestTop, bestMargin = rate, cover, topThr, marginThr
				}
			}
		}
		t.Logf("variant %s best accept: top>=%.1f margin>=%.1f accepted-accuracy %.3f coverage %.3f",
			v.name, bestTop, bestMargin, bestAcc, bestCover)
		for _, r := range results {
			if r.got != r.want {
				t.Logf("variant %s MISS want=%s got=%s top=%.2f second=%.2f %q",
					v.name, r.want, r.got, r.top, r.second, r.prompt)
			}
		}
	}
	t.Logf("project-pick experiment used %d Jev calls (budget %d), model %s", calls, pickBudget, systemone.JevModelPinned)
}

func pickMax(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// pickNoneID is the question/answer id for "none of these projects".
const pickNoneID = "none"

// pickProject is one real project from ~/.local/share/pig/enderbloom/projects.json.
type pickProject struct {
	name string
	path string
	gist string
}

// pickProjects mirrors the four real projects. Gists come from each project's
// own docs: emberloop's README first lines, scintilla-chat-v2's CONTEXT.md
// plus its apps/ layout (SvelteKit apps, Turbo, oxlint gate), tarek's
// data/result_00.md (Italian Wengo.it fortune-teller lead research), and
// cioky's absence (empty directory, verified with ls: no files at all).
var pickProjects = []pickProject{
	{name: "cioky", path: "/home/cioky/Projects/cioky", gist: "empty directory with no files yet; a blank parked project slot"},
	{name: "tarek", path: "/home/cioky/Projects/tarek", gist: "Italian lead-research notes on Wengo.it fortune-teller (cartomanti) profiles in data/result*.md"},
	{name: "emberloop", path: "/home/cioky/Projects/emberloop", gist: "Go harness for parallel agent implement/review loops over pig and claude sessions"},
	{name: "scintilla-chat-v2", path: "/home/cioky/Projects/scintilla-chat-v2", gist: "Scintilla tarot-reader marketplace monorepo: SvelteKit apps under apps/, Turbo plus oxlint gate"},
}

// pickVariant is one project-description wording under test: entry builds the
// `projects` State entry (and hence what the question can point at) for a
// project. The "none" question wording is identical across variants.
type pickVariant struct {
	name  string
	entry func(pickProject) string
}

// pickVariants holds the three wordings under comparison.
var pickVariants = []pickVariant{
	{name: "v1-names-only", entry: func(p pickProject) string { return p.name }},
	{name: "v2-name-path", entry: func(p pickProject) string { return p.name + " (" + p.path + ")" }},
	{name: "v3-name-gist", entry: func(p pickProject) string { return p.name + " (" + p.path + "): " + p.gist }},
}

// pickPrompt is one labelled first message. want is the expected answer id
// (a project name, or "none"); ambiguous marks dual-plausible prompts, kept
// in the totals and called out in the analysis.
type pickPrompt struct {
	text      string
	want      string
	ambiguous bool
}

// pickPrompts is the labelled set: 4 explicit-name, 5 path-mention, 8
// implicit-topic, 3 ambiguous, 5 none/general. emberloop-web/ and installer/
// prompts want emberloop: both live inside the emberloop repo checkout.
var pickPrompts = []pickPrompt{
	// Explicit project name.
	{text: "In emberloop, how does the finish gate decide whether a ticket merges?", want: "emberloop"},
	{text: "scintilla-chat-v2: the Svelte doctor score dropped after my last change, what did I break?", want: "scintilla-chat-v2"},
	{text: "In tarek, add the slice_01 URLs to the Wengo lead research the same way as slice_00.", want: "tarek"},
	{text: "Write a README in cioky describing what that project directory is for.", want: "cioky"},
	// File-path mention.
	{text: "Fix the failing test in emberloop-web/memory/jev.go around score legends.", want: "emberloop"},
	{text: "The Turbo pipeline in scintilla-chat-v2 is slow; profile the build across packages/apps.", want: "scintilla-chat-v2"},
	{text: "Dedupe the overlapping result_0*.md files under tarek/data.", want: "tarek"},
	{text: "There is nothing in /home/cioky/Projects/cioky yet; start a notes file there.", want: "cioky"},
	{text: "installer/install.sh fails on a fresh machine with no Go toolchain.", want: "emberloop"},
	// Implicit by topic.
	{text: "My parallel agent loop keeps merging without reviewer approval; how do I gate it?", want: "emberloop"},
	{text: "How do pig and claude sessions share one worktree in my harness?", want: "emberloop"},
	{text: "How do I add a new pig extension beside emberloop and pig-antigravity-auth?", want: "emberloop"},
	{text: "The SvelteKit build warns about unused CSS in my chat app; where does that come from?", want: "scintilla-chat-v2"},
	{text: "The oxlint gate flags my new file under the anti-slop rules; how do I satisfy it?", want: "scintilla-chat-v2"},
	{text: "Cartomante Cleo shows 7228 consulti on her bio; cross-check her profile against external sources.", want: "tarek"},
	{text: "Translate the Wengo lead-research method section into English.", want: "tarek"},
	{text: "I want a blank directory to start a personal wiki; use the empty one.", want: "cioky"},
	// Ambiguous (dual-plausible).
	{text: "Add a README documenting the project setup.", want: "cioky", ambiguous: true},
	{text: "Speed up the test suite, it takes far too long.", want: "scintilla-chat-v2", ambiguous: true},
	{text: "Explain how SQLite FTS5 ranking works in general.", want: "none", ambiguous: true},
	// None of these / general questions.
	{text: "What is the capital of France?", want: "none"},
	{text: "Write a haiku about rain.", want: "none"},
	{text: "How do I bake sourdough bread at home?", want: "none"},
	{text: "What is the difference between TCP and UDP?", want: "none"},
	{text: "Remember: always use tabs in this repo.", want: "none"},
}
