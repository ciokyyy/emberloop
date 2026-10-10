package web

import (
	"strings"
	"testing"
)

// Table tests for the reviewer reply format: one own-line VERDICT plus
// `file:line severity: problem` findings.
func TestParseReviewTable(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		wantOK   bool
		verdict  string
		findings []ReviewFinding
	}{
		{
			name:    "good pass without findings",
			text:    "Looks solid.\nVERDICT: pass\n",
			wantOK:  true,
			verdict: "pass",
		},
		{
			name:    "good changes with two findings",
			text:    "VERDICT: changes\nwork.go:10 blocker: handle nil\napi.go:3 minor: rename var\n",
			wantOK:  true,
			verdict: "changes",
			findings: []ReviewFinding{
				{File: "work.go", Line: 10, Severity: "blocker", Problem: "handle nil"},
				{File: "api.go", Line: 3, Severity: "minor", Problem: "rename var"},
			},
		},
		{
			name:   "missing verdict",
			text:   "work.go:10 blocker: handle nil\n",
			wantOK: false,
		},
		{
			name:    "extra prose ignored",
			text:    "I reviewed the branch carefully.\nVERDICT: pass\nThanks, great work!\n",
			wantOK:  true,
			verdict: "pass",
		},
		{
			name:    "CRLF line endings",
			text:    "VERDICT: changes\r\nwork.go:10 blocker: handle nil\r\n",
			wantOK:  true,
			verdict: "changes",
			findings: []ReviewFinding{
				{File: "work.go", Line: 10, Severity: "blocker", Problem: "handle nil"},
			},
		},
		{
			name:   "bad severity",
			text:   "VERDICT: changes\nwork.go:10 critical: handle nil\n",
			wantOK: false,
		},
		{
			name:    "multi-line problem",
			text:    "VERDICT: changes\nwork.go:10 blocker: handle nil\n  it panics on empty input\n",
			wantOK:  true,
			verdict: "changes",
			findings: []ReviewFinding{
				{File: "work.go", Line: 10, Severity: "blocker", Problem: "handle nil\nit panics on empty input"},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseReview(tc.text)
			if got.OK != tc.wantOK {
				t.Fatalf("OK = %v, want %v (verdict %q findings %+v)", got.OK, tc.wantOK, got.Verdict, got.Findings)
			}
			if !tc.wantOK {
				return
			}
			if got.Verdict != tc.verdict {
				t.Fatalf("verdict = %q, want %q", got.Verdict, tc.verdict)
			}
			if len(got.Findings) != len(tc.findings) {
				t.Fatalf("findings = %+v, want %+v", got.Findings, tc.findings)
			}
			for i, want := range tc.findings {
				if got.Findings[i] != want {
					t.Fatalf("finding %d = %+v, want %+v", i, got.Findings[i], want)
				}
			}
		})
	}
}

func TestReviewerPromptCarriesItsAxisBrief(t *testing.T) {
	spec := reviewerPrompt("01-a", "spec.md", reviewerSpecType, "b", "abc")
	if !strings.Contains(spec, "Spec axis.") || strings.Contains(spec, "Smell baseline") {
		t.Errorf("spec reviewer prompt has the wrong brief: %q", spec)
	}
	std := reviewerPrompt("01-a", "spec.md", reviewerStandardsType, "b", "abc")
	if !strings.Contains(std, "Standards axis.") || !strings.Contains(std, "Smell baseline") || !strings.Contains(std, "DOX check") {
		t.Errorf("standards reviewer prompt has the wrong brief: %q", std)
	}
	if !strings.Contains(spec, "Review ticket 01-a") || !strings.Contains(std, "VERDICT: pass") {
		t.Error("the verdict format prefix must stay")
	}
}

func TestWebWorkerIsNeverToldToSpawnReviewers(t *testing.T) {
	if strings.Contains(webStallNudge, "Spawn your reviewers") {
		t.Errorf("stall nudge tells a Run worker to spawn reviewers: %q", webStallNudge)
	}
	if !strings.Contains(workerReviewNote, "Do not spawn reviewers") {
		t.Errorf("worker prompt note lost: %q", workerReviewNote)
	}
}
