package memory

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestExtractAnchorsFindsPathsPackagesAndModules(t *testing.T) {
	quote := "fix the upload handler in emberloop-web/memory/bank.go, see TestUploadHandler"
	tools := []string{`{"name":"edit","input":"file=emberloop-web/memory/bank.go"}`}
	goMod := []byte("module example.com/emberloop-web\n\ngo 1.26.0\n\nrequire modernc.org/sqlite v1.59.0\n")
	// Mention a module path so it resolves to module@version.
	quote += " using modernc.org/sqlite for storage"
	got := ExtractAnchors(quote, tools, goMod)
	if len(got) == 0 {
		t.Fatal("ExtractAnchors returned nothing")
	}
	if !slices.Contains(got, "emberloop-web/memory/bank.go") {
		t.Errorf("missing path anchor: %q", got)
	}
	if !slices.Contains(got, "modernc.org/sqlite@v1.59.0") {
		t.Errorf("missing module@version anchor: %q", got)
	}
	if !slices.IsSorted(got) {
		t.Errorf("anchors not sorted: %q", got)
	}
}

func TestExtractAnchorsDeterministic(t *testing.T) {
	quote := "use emberloop-web/memory/bank.go and modernc.org/sqlite@v1.59.0 twice bank.go"
	tools := []string{"edit emberloop-web/memory/bank.go", "read go.mod"}
	goMod := []byte("module example.com/x\nrequire modernc.org/sqlite v1.59.0\n")
	first := ExtractAnchors(quote, tools, goMod)
	second := ExtractAnchors(quote, tools, goMod)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("not deterministic:\nfirst  %q\nsecond %q", first, second)
	}
	seen := make(map[string]int)
	for _, a := range first {
		seen[a]++
		if seen[a] > 1 {
			t.Fatalf("duplicate anchor %q in %q", a, first)
		}
	}
	// Order must not depend on input order: shuffle the tool texts.
	shuffled := ExtractAnchors(quote, []string{"read go.mod", "edit emberloop-web/memory/bank.go"}, goMod)
	if !reflect.DeepEqual(first, shuffled) {
		t.Fatalf("order depends on tool order:\n%q\n%q", first, shuffled)
	}
}

func TestExtractAnchorsParsesGoModBlocks(t *testing.T) {
	goMod := []byte("module example.com/x\n\nrequire (\n\tgithub.com/MichaelKinsy/PiG v0.4.0\n\tmodernc.org/sqlite v1.59.0 // indirect\n)\n")
	quote := "touch github.com/MichaelKinsy/PiG for spawn and modernc.org/sqlite for banks"
	got := ExtractAnchors(quote, nil, goMod)
	for _, want := range []string{"github.com/MichaelKinsy/PiG@v0.4.0", "modernc.org/sqlite@v1.59.0"} {
		if !slices.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
}

func TestExtractAnchorsCapsAndSkipsNoise(t *testing.T) {
	var parts []string
	for i := 0; i < MaxAnchors+10; i++ {
		parts = append(parts, "pkg/dir"+strings.Repeat("x", i)+"file.go")
	}
	got := ExtractAnchors(strings.Join(parts, " "), nil, nil)
	if len(got) != MaxAnchors {
		t.Fatalf("got %d anchors, want cap %d: %q", len(got), MaxAnchors, got)
	}
	if got == nil {
		t.Fatal("capped result must not be nil")
	}
	// Noise without anchors gives nil, not an empty slice.
	if out := ExtractAnchors("hello world", nil, nil); out != nil {
		t.Fatalf("noise anchors = %q, want nil", out)
	}
}

func TestNormalizeAnchorsDropsPreferences(t *testing.T) {
	if out := NormalizeAnchors(KindPreference, []string{"a/b.go"}); out != nil {
		t.Fatalf("preference anchors = %q, want nil", out)
	}
	got := NormalizeAnchors(KindFact, []string{"b.go", "a.go", "a.go"})
	if !reflect.DeepEqual(got, []string{"a.go", "b.go"}) {
		t.Fatalf("NormalizeAnchors = %q, want sorted deduped", got)
	}
}

func TestRuleValidationAnchors(t *testing.T) {
	if err := validRule(Rule{Text: "x", Kind: KindPreference, Status: StatusActive, ApprovedBy: "u1", Anchors: []string{"a/b.go"}}); err == nil {
		t.Fatal("preference with anchors accepted")
	}
	if err := validRule(Rule{Text: "x", Kind: KindFact, Status: StatusActive, ApprovedBy: "u1", Anchors: []string{""}}); err == nil {
		t.Fatal("empty anchor accepted")
	}
	// cleanAnchor and validRule agree: interior whitespace never stores.
	if err := validRule(Rule{Text: "x", Kind: KindFact, Status: StatusActive, ApprovedBy: "u1", Anchors: []string{"foo bar"}}); err == nil {
		t.Fatal("interior-space anchor accepted")
	}
	// The spec caps only Rule text; anchor count and length are extraction
	// caps, not write limits.
	var many []string
	for i := 0; i < MaxAnchors+1; i++ {
		many = append(many, "a/"+strings.Repeat("x", i)+"/b.go")
	}
	if err := validRule(Rule{Text: "x", Kind: KindFact, Status: StatusActive, ApprovedBy: "u1", Anchors: many}); err != nil {
		t.Fatalf("anchor overflow refused: %v", err)
	}
	long := strings.Repeat("y", MaxAnchorRunes+1)
	if err := validRule(Rule{Text: "x", Kind: KindFact, Status: StatusActive, ApprovedBy: "u1", Anchors: []string{long}}); err != nil {
		t.Fatalf("long anchor refused: %v", err)
	}
}
