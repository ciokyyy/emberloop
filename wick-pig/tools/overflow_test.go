package tools

import (
	"fmt"
	"strings"
	"testing"

	"example.com/wick/statedb"
)

// Seam: overflow threshold setting — the store boundary honours
// overflow.threshold_bytes, defaulting to about 8KB.
func TestOverflowThresholdDefaultsAbout8KB(t *testing.T) {
	dir := t.TempDir()
	globalProjectEnv(t, dir, `{}`, "")
	t.Setenv("WICK_OVERFLOW_THRESHOLD_BYTES", "")
	if got := EffectiveSettings(dir).EffectiveOverflowThresholdBytes(); got != 8192 {
		t.Fatalf("default threshold = %d, want 8192", got)
	}
}

func TestOverflowThresholdFromFileEnvAndProject(t *testing.T) {
	dir := t.TempDir()
	globalProjectEnv(t, dir, `{"overflow":{"threshold_bytes":4096}}`, "")
	t.Setenv("WICK_OVERFLOW_THRESHOLD_BYTES", "")
	if got := EffectiveSettings(dir).EffectiveOverflowThresholdBytes(); got != 4096 {
		t.Fatalf("file threshold = %d, want 4096", got)
	}

	t.Setenv("WICK_OVERFLOW_THRESHOLD_BYTES", "1024")
	if got := EffectiveSettings(dir).EffectiveOverflowThresholdBytes(); got != 1024 {
		t.Fatalf("env threshold = %d, want 1024", got)
	}
	t.Setenv("WICK_OVERFLOW_THRESHOLD_BYTES", "")

	pdir := t.TempDir()
	globalProjectEnv(t, pdir, `{"overflow":{"threshold_bytes":4096}}`, `{"overflow":{"threshold_bytes":2048}}`)
	if got := EffectiveSettings(pdir).EffectiveOverflowThresholdBytes(); got != 2048 {
		t.Fatalf("project threshold = %d, want 2048", got)
	}
	if got := EffectiveSettings(pdir).SourceOf("overflow.threshold_bytes"); got != "project" {
		t.Fatalf("threshold source = %q, want project", got)
	}
}

// Seam: overflow helper — output under the threshold is byte-identical;
// over it a handle and summary come back and the bytes are stored.
func TestOverflowUnderThresholdPassesThrough(t *testing.T) {
	t.Setenv("WICK_STATE_DIR", t.TempDir())
	t.Setenv("WICK_OVERFLOW_THRESHOLD_BYTES", "")
	content := "short output\nwith two lines\n"
	out, stored, err := MaybeOverflow(t.TempDir(), "s1", "bash", "text/plain", content, "")
	if err != nil || stored || out != content {
		t.Fatalf("under threshold = %q, %v, %v; want the content unchanged", out, stored, err)
	}
}

func TestOverflowOverThresholdStoresAndSummarizes(t *testing.T) {
	t.Setenv("WICK_STATE_DIR", t.TempDir())
	t.Setenv("WICK_OVERFLOW_THRESHOLD_BYTES", "100")
	var b strings.Builder
	b.WriteString("# Caching notes\n\n")
	for i := 1; i <= 30; i++ {
		fmt.Fprintf(&b, "line %02d about the query cache and database indexes\n", i)
	}
	content := b.String()
	out, stored, err := MaybeOverflow(t.TempDir(), "s1", "bash", "text/plain", content, "")
	if err != nil || !stored {
		t.Fatalf("over threshold stored = %v, %v; want true, nil", stored, err)
	}
	id, _, ok := ParseCtxHandle(handleFromSummary(t, out))
	if !ok {
		t.Fatalf("no ctx handle in summary:\n%s", out)
	}
	for _, want := range []string{"bytes", "lines", "searchable terms", "line 01", "line 30"} {
		if !strings.Contains(out, want) {
			t.Errorf("summary misses %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "line 15 about") {
		t.Errorf("summary must not carry the full middle:\n%s", out)
	}
	db, err := statedb.Open(statedb.DBPath())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	got, found, err := statedb.GetCtxBlob(db, id)
	if err != nil || !found || got.Bytes != content {
		t.Fatalf("stored bytes differ: found=%v err=%v", found, err)
	}
}

// Seam: intent — a helper call with an intent returns matching chunks inline.
func TestOverflowIntentReturnsMatchingChunks(t *testing.T) {
	t.Setenv("WICK_STATE_DIR", t.TempDir())
	t.Setenv("WICK_OVERFLOW_THRESHOLD_BYTES", "100")
	content := "# Apples\n\napple harvest in autumn\n\n# Engines\n\nengine torque and pistons\n"
	content += strings.Repeat("filler words to push past the threshold line\n", 20)
	out, stored, err := MaybeOverflow(t.TempDir(), "s1", "bash", "text/plain", content, "engine torque")
	if err != nil || !stored {
		t.Fatalf("stored = %v, %v; want true, nil", stored, err)
	}
	if !strings.Contains(out, "torque") {
		t.Errorf("intent excerpts miss the matching chunk:\n%s", out)
	}
}

func handleFromSummary(t *testing.T, out string) string {
	t.Helper()
	for _, f := range strings.Fields(out) {
		if strings.HasPrefix(f, "ctx:") {
			return strings.Trim(f, "(),;")
		}
	}
	t.Fatalf("no handle in:\n%s", out)
	return ""
}

// Seam: ctx_search — several queries at once, scoped to a handle, plain
// when nothing matches.
func TestCtxSearchQueriesScopedAndPlain(t *testing.T) {
	t.Setenv("WICK_STATE_DIR", t.TempDir())
	t.Setenv("WICK_OVERFLOW_THRESHOLD_BYTES", "50")
	root := t.TempDir()
	outA, _, err := MaybeOverflow(root, "s1", "bash", "text/plain", "alpha uniqueword beta\n\n"+strings.Repeat("pad pad pad pad\n", 10), "")
	if err != nil {
		t.Fatal(err)
	}
	handleA := handleFromSummary(t, outA)
	if _, _, err := MaybeOverflow(root, "s1", "bash", "text/plain", "gamma otherword delta\n\n"+strings.Repeat("pad pad pad pad\n", 10), ""); err != nil {
		t.Fatal(err)
	}
	out, err := run(t, ctxSearchTool{}, root, map[string]any{"queries": []string{"uniqueword", "nosuchwordanywhere"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, handleA) || !strings.Contains(out, "uniqueword") {
		t.Errorf("search misses the stored hit:\n%s", out)
	}
	if !strings.Contains(out, "nothing matched") {
		t.Errorf("search must say plainly when nothing matches:\n%s", out)
	}
	scoped, err := run(t, ctxSearchTool{}, root, map[string]any{"queries": []string{"otherword"}, "source": handleA})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(scoped, "nothing matched") {
		t.Errorf("a search scoped to the other blob must find nothing:\n%s", scoped)
	}
}

func TestCtxSearchDefaultLimitThree(t *testing.T) {
	t.Setenv("WICK_STATE_DIR", t.TempDir())
	t.Setenv("WICK_OVERFLOW_THRESHOLD_BYTES", "50")
	root := t.TempDir()
	var b strings.Builder
	for i := 1; i <= 6; i++ {
		fmt.Fprintf(&b, "repeatword block number %d\n\n", i)
	}
	if _, _, err := MaybeOverflow(root, "s1", "bash", "text/plain", b.String(), ""); err != nil {
		t.Fatal(err)
	}
	out, err := run(t, ctxSearchTool{}, root, map[string]any{"queries": []string{"repeatword"}})
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(out, "ctx:"); n != 3 {
		t.Errorf("default limit gives %d hits, want 3:\n%s", n, out)
	}
	out5, err := run(t, ctxSearchTool{}, root, map[string]any{"queries": []string{"repeatword"}, "limit": 5})
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(out5, "ctx:"); n != 5 {
		t.Errorf("limit 5 gives %d hits, want 5:\n%s", n, out5)
	}
}

// Seam: read handle — ctx:<id>:<range> pages the stored output.
func TestReadCtxHandlePagesStoredOutput(t *testing.T) {
	t.Setenv("WICK_STATE_DIR", t.TempDir())
	t.Setenv("WICK_OVERFLOW_THRESHOLD_BYTES", "50")
	root := t.TempDir()
	var b strings.Builder
	for i := 1; i <= 30; i++ {
		fmt.Fprintf(&b, "stored-line-%02d with words\n", i)
	}
	out, _, err := MaybeOverflow(root, "s1", "bash", "text/plain", b.String(), "")
	if err != nil {
		t.Fatal(err)
	}
	id, _, _ := ParseCtxHandle(handleFromSummary(t, out))
	paged, err := run(t, readTool{}, root, map[string]any{"path": fmt.Sprintf("ctx:%d:25-27", id)})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"stored-line-25", "stored-line-26", "stored-line-27"} {
		if !strings.Contains(paged, want) {
			t.Errorf("paged read misses %q:\n%s", want, paged)
		}
	}
	if strings.Contains(paged, "stored-line-10") {
		t.Errorf("paged read leaks outside the range:\n%s", paged)
	}
	if _, err := run(t, readTool{}, root, map[string]any{"path": "ctx:999999"}); err == nil {
		t.Error("unknown handle must fail")
	}
}
