package web

import (
	"strings"
	"testing"
)

// Table tests for the reviewer reply format: one own-line VERDICT plus
// `file:line severity: problem` findings.
func TestParseReviewTable(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	spec := reviewerPrompt("01-a", "spec.md", reviewerSpecType, "b", "abc")
	if !strings.Contains(spec, "Spec axis.") || strings.Contains(spec, "Smell baseline") {
		t.Errorf("spec reviewer prompt has the wrong brief: %q", spec)
	}
	std := reviewerPrompt("01-a", "spec.md", reviewerStandardsType, "b", "abc")
	if !strings.Contains(std, "Standards axis.") || !strings.Contains(std, "Smell baseline") || !strings.Contains(std, "DOX check") {
		t.Errorf("standards reviewer prompt has the wrong brief: %q", std)
	}
	if !strings.Contains(spec, "Review ticket 01-a") {
		t.Error("the review ticket prefix must stay")
	}
	for _, p := range []string{spec, std} {
		if !strings.Contains(p, "submit tool") || !strings.Contains(p, "verdict") || !strings.Contains(p, "tip") || !strings.Contains(p, "ran") {
			t.Errorf("reviewer prompt must describe the submit tool with verdict, tip, ran and findings: %q", p)
		}
		if strings.Contains(p, "VERDICT: pass") || strings.Contains(p, "RESPOND IN FORMAT") {
			t.Errorf("reviewer prompt must drop the free-text verdict format: %q", p)
		}
	}
}

func TestWebWorkerIsNeverToldToSpawnReviewers(t *testing.T) {
	t.Parallel()
	if strings.Contains(webStallNudge, "Spawn your reviewers") {
		t.Errorf("stall nudge tells a Run worker to spawn reviewers: %q", webStallNudge)
	}
	if !strings.Contains(workerReviewNote, "Do not spawn reviewers") {
		t.Errorf("worker prompt note lost: %q", workerReviewNote)
	}
}

// Table tests for the reviewer submit tool validation: a `changes` needs a
// blocker or major, a `pass` needs `ran`, and severities stay
// blocker|major|minor. A stale tip is not a validation error: it records and
// does not count.
func TestValidateReviewerSubmitTable(t *testing.T) {
	t.Parallel()
	ran := []ReviewRan{{Command: "go vet ./...", ExitCode: 0}}
	tests := []struct {
		name    string
		sub     ReviewerSubmit
		wantErr string
	}{
		{
			name: "good pass with ran",
			sub:  ReviewerSubmit{Verdict: "pass", Tip: "abc", Ran: ran},
		},
		{
			name:    "pass without ran",
			sub:     ReviewerSubmit{Verdict: "pass", Tip: "abc"},
			wantErr: "ran",
		},
		{
			name:    "changes with blocker",
			sub:     ReviewerSubmit{Verdict: "changes", Tip: "abc", Ran: ran, Findings: []ReviewFinding{{File: "w.go", Line: 1, Severity: "blocker", Problem: "nil"}}},
			wantErr: "",
		},
		{
			name:    "changes with only minors",
			sub:     ReviewerSubmit{Verdict: "changes", Tip: "abc", Ran: ran, Findings: []ReviewFinding{{File: "w.go", Line: 1, Severity: "minor", Problem: "nit"}}},
			wantErr: "blocker",
		},
		{
			name:    "pass with minors",
			sub:     ReviewerSubmit{Verdict: "pass", Tip: "abc", Ran: ran, Findings: []ReviewFinding{{File: "w.go", Line: 1, Severity: "minor", Problem: "nit"}}},
			wantErr: "",
		},
		{
			name:    "bad verdict",
			sub:     ReviewerSubmit{Verdict: "maybe", Tip: "abc", Ran: ran},
			wantErr: "verdict",
		},
		{
			name:    "missing tip",
			sub:     ReviewerSubmit{Verdict: "pass", Ran: ran},
			wantErr: "tip",
		},
		{
			name:    "bad severity",
			sub:     ReviewerSubmit{Verdict: "changes", Tip: "abc", Ran: ran, Findings: []ReviewFinding{{File: "w.go", Line: 1, Severity: "critical", Problem: "x"}}},
			wantErr: "severity",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateReviewerSubmit(tc.sub)
			if tc.wantErr == "" && err != nil {
				t.Fatalf("Validate = %v, want nil", err)
			}
			if tc.wantErr != "" && (err == nil || !strings.Contains(strings.ToLower(err.Error()), tc.wantErr)) {
				t.Fatalf("Validate = %v, want error containing %q", err, tc.wantErr)
			}
		})
	}
}
