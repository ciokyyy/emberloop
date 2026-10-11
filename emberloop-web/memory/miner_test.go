package memory

import (
	"testing"
)

// heuristicGoldens are the ticket-19 acceptance cases for the ordered pass:
// guardrails accepted, denylisted phrases rejected, forward pivots rejected,
// bare praise rejected, CJK corrections accepted.
var heuristicGoldens = []struct {
	name string
	text string
	want bool
	typ  string
}{
	{"guardrail dont-unless", "don't add tests unless asked", true, HitGuardrail},
	{"guardrail only-what-asked", "only change what I asked, nothing else", true, HitGuardrail},
	{"guardrail minimal", "please keep minimal changes here", true, HitGuardrail},
	{"explicit remember", "remember: always wrap errors in this repo", true, HitExplicit},
	{"deny no-problem", "no problem, thanks for the fix", false, ""},
	{"deny never-mind", "never mind, I found it", false, ""},
	{"deny no-worries", "no worries at all", false, ""},
	{"deny dont-worry", "don't worry about it", false, ""},
	{"forward pivot", "Perfect! Now let's add pagination to the list", false, ""},
	{"forward pivot go-ahead", "Great approach! Now let's go ahead and build the export", false, ""},
	{"bare praise short", "Perfect!", false, ""},
	{"bare praise nailed", "nailed it", false, ""},
	{"bare praise love", "i love it", false, ""},
	{"long praise kept", "Perfect! That's exactly what I wanted for the upload handler flow", true, HitPositive},
	{"strong no", "No, use tabs here", true, HitAuto},
	{"strong dont", "don't use mocks for this package", true, HitAuto},
	{"strong thats-wrong", "that's wrong, the retry belongs in the caller", true, HitAuto},
	{"strong i-meant", "I meant the other branch, not main", true, HitAuto},
	{"strong use-not", "use errors.As not string matching for this check", true, HitAuto},
	{"strong stop", "stop pushing to main", true, HitAuto},
	{"weak actually", "actually, use tabs here", true, HitAuto},
	{"question rejected", "can you fix this for me?", false, ""},
	{"question trailing", "What does this function do?", false, ""},
	{"request rejected", "please add a retry helper", false, ""},
	{"task request", "I need a new export command", false, ""},
	{"slash guard", "/reflect --scan-history for this repo", false, ""},
	{"slash remember guard", "/remember use tabs", false, ""},
	{"too short", "ok", false, ""},
	{"cjk japanese", "いや、そうじゃなくてタブを使って", true, HitAuto},
	{"cjk chigau", "違う、こっちを使ってください", true, HitAuto},
	{"cjk chinese", "不是，用这个方法", true, HitAuto},
	{"cjk korean", "아니, 이렇게 해", true, HitAuto},
}

func TestClassifyHeuristicGoldens(t *testing.T) {
	for _, c := range heuristicGoldens {
		got, ok := ClassifyHeuristic(c.text)
		if ok != c.want {
			t.Errorf("%s: ClassifyHeuristic(%q) = %v, want %v", c.name, c.text, ok, c.want)
			continue
		}
		if c.want && got.Type != c.typ {
			t.Errorf("%s: type = %q, want %q (patterns %q)", c.name, got.Type, c.typ, got.Patterns)
		}
	}
}

func TestClassifyHeuristicOrderSlashFirst(t *testing.T) {
	// A slash command holding an explicit marker is still a command, never
	// feedback: the guard runs before the explicit check.
	if _, ok := ClassifyHeuristic("/review remember: always wrap errors"); ok {
		t.Error("slash command with remember: classified, want rejected")
	}
}

func TestClassifyHeuristicWeakGatedByLength(t *testing.T) {
	short := "actually, use tabs"
	if _, ok := ClassifyHeuristic(short); !ok {
		t.Errorf("short weak %q rejected, want kept", short)
	}
	long := "actually, " + longFiller()
	if _, ok := ClassifyHeuristic(long); ok {
		t.Errorf("long weak message classified, want skipped past %d chars", maxWeakPatternLength)
	}
}

func longFiller() string {
	s := "this is a long task description with plenty of context about the thing we are building together "
	for len(s) < maxWeakPatternLength+20 {
		s += s
	}
	return s
}
