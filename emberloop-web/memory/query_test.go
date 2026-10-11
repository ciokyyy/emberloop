package memory

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestQueryQuotesEveryTerm(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"!!! --- ::: ()", ""},
		{"TestUploadHandler", `"testuploadhandler" OR "test" OR "upload" OR "handler"`},
		{`a"b AND (c-d):e OR NOT f*`, `"a" OR "b" OR "and" OR "c" OR "d" OR "e" OR "or" OR "not" OR "f"`},
		{"internal/memory/store.go", `"internal" OR "memory" OR "store" OR "go"`},
		{"upload_handler upload_handler", `"upload" OR "handler"`},
		{"NEAR(a b, 2)", `"near" OR "a" OR "b" OR "2"`},
	}
	for _, c := range cases {
		if got := Query(c.in); got != c.want {
			t.Errorf("Query(%q)\n got %q\nwant %q", c.in, got, c.want)
		}
	}
}

func TestTokensCapLengthAndCount(t *testing.T) {
	long := strings.Repeat("x", 300)
	if got := Tokens(long); len(got) != 1 || len([]rune(got[0])) != MaxTokenRunes {
		t.Fatalf("long token: got %d terms, first %d runes; want 1 term of %d runes", len(got), runeLen(got), MaxTokenRunes)
	}
	var words []string
	for i := 0; i < 100; i++ {
		words = append(words, "word"+string(rune('a'+i%26))+strings.Repeat("z", i/26))
	}
	if got := Tokens(strings.Join(words, " ")); len(got) != MaxQueryTokens {
		t.Fatalf("many words: got %d terms, want cap %d", len(got), MaxQueryTokens)
	}
	huge := strings.Repeat("ab ", 2000)
	if got := Tokens(huge); len(got) != 1 {
		t.Fatalf("repeated term: got %d terms, want 1 after dedupe", len(got))
	}
	cut := strings.Repeat("q", MaxQueryRunes) + " tail"
	for _, term := range Tokens(cut) {
		if term == "tail" {
			t.Fatalf("input past %d runes was not cut", MaxQueryRunes)
		}
	}
}

func runeLen(terms []string) int {
	if len(terms) == 0 {
		return 0
	}
	return len([]rune(terms[0]))
}

func TestSplitTextExpandsIdentifiers(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"Use TestUploadHandler here.", "Use Test Upload Handler here"},
		{"upload_handler", "upload handler"},
		{"HTTPServer and utf8Encode", "HTTP Server and utf8 Encode"},
		{"go.errors.wrapping", "go errors wrapping"},
		{"plain words", "plain words"},
	}
	for _, c := range cases {
		if got := SplitText(c.in); got != c.want {
			t.Errorf("SplitText(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestCamelPartsBoundaries(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"TestUploadHandler", []string{"Test", "Upload", "Handler"}},
		{"HTTPServer", []string{"HTTP", "Server"}},
		{"parseV2Handler", []string{"parse", "V2", "Handler"}},
		{"lower", []string{"lower"}},
		{"ALLCAPS", []string{"ALLCAPS"}},
	}
	for _, c := range cases {
		if got := camelParts(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("camelParts(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestScrubPlainReplacesEachText(t *testing.T) {
	got, changed := scrubPlain("say x and x again", []string{"x", ""})
	if !changed || got != "say [purged] and [purged] again" {
		t.Fatalf("scrubPlain = %q, %v", got, changed)
	}
	if got, changed := scrubPlain("untouched", []string{""}); changed || got != "untouched" {
		t.Fatalf("empty text must not match: %q, %v", got, changed)
	}
}

func TestScrubJSONTextKeepsDocumentValid(t *testing.T) {
	cases := []struct {
		name, doc, text, want string
		changed               bool
	}{
		{"escaped quotes in a value", `{"q":"say \"hi\" <b>"}`, `say "hi" <b>`, `{"q":"[purged]"}`, true},
		{"object key", `{"my secret":1}`, "my secret", `{"[purged]":1}`, true},
		{"structure is not matched", `{"x":1}`, `"x":1`, `{"x":1}`, false},
		{"numbers pass through", `{"n":12345678901234567890,"s":"a"}`, "a", `{"n":12345678901234567890,"s":"[purged]"}`, true},
		{"not json falls back to plain", `plain x`, "x", `plain [purged]`, true},
		{"no match keeps the document", `{"a":"b"}`, "zzz", `{"a":"b"}`, false},
	}
	for _, c := range cases {
		got, changed := scrubJSONText(c.doc, []string{c.text})
		if got != c.want || changed != c.changed {
			t.Errorf("%s: scrubJSONText = %q, %v; want %q, %v", c.name, got, changed, c.want, c.changed)
		}
	}
}

func TestTokensKeepCombiningMarks(t *testing.T) {
	got := Tokens("नमस्ते world")
	if len(got) != 2 || got[0] != "नमस्ते" {
		t.Fatalf("Tokens = %q; want the Devanagari word whole", got)
	}
}

func TestSnapshotDueRule(t *testing.T) {
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	if !snapshotDue(time.Time{}, now) {
		t.Fatal("no snapshot yet: want due")
	}
	if snapshotDue(now.Add(-5*time.Hour-59*time.Minute), now) {
		t.Fatal("newest 5h59m old: want not due")
	}
	if !snapshotDue(now.Add(-SnapshotInterval), now) {
		t.Fatal("newest exactly 6h old: want due")
	}
}

func TestValidUserID(t *testing.T) {
	ok := []string{"0123456789abcdef0123456789abcdef", "alice", "a-b_c", "A9"}
	bad := []string{"", "-lead", "../x", "a/b", "a b", "a.b", strings.Repeat("a", 129)}
	for _, id := range ok {
		if !validUserID(id) {
			t.Errorf("validUserID(%q) = false, want true", id)
		}
	}
	for _, id := range bad {
		if validUserID(id) {
			t.Errorf("validUserID(%q) = true, want false", id)
		}
	}
}
