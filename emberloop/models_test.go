package emberloop

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

// pigListModelsOutput is a captured `pig --list-models` table: a header, then
// one row per model, with the thinking column deciding whether pig's levels
// apply.
const pigListModelsOutput = `provider     model                                               context  max-out  thinking  images
deepseek     deepseek-flash                                      1M       384K     yes       yes   
openrouter   amazon/nova-lite-v1                                 300K     5.1K     no        yes   
opencode-go  kimi-k3                                             1.0M     131.1K   yes       yes   
`

// claudeInitializeResponse is a captured answer to the initialize control
// request, trimmed to the models it carries: value, displayName and, for a
// model that takes effort, supportedEffortLevels.
const claudeInitializeResponse = `{"type":"control_response","response":{"subtype":"success","request_id":"emberloop-models","response":{"models":[{"value":"default","displayName":"Default (recommended)","supportsEffort":true,"supportedEffortLevels":["low","medium","high","xhigh","max"]},{"value":"haiku","displayName":"Haiku 4.5"},{"value":"opus","displayName":"Opus 5.5","supportsEffort":true,"supportedEffortLevels":["low","medium","high","xhigh","max"]}]}}}`

func TestPigModelsParseTheListModelsTable(t *testing.T) {
	run := func(cmd string, args []string, _ ExecOpts) (Result, error) {
		if cmd != pigBin() || !reflect.DeepEqual(args, []string{"--list-models"}) {
			t.Fatalf("pig listing ran %s %v", cmd, args)
		}
		return Result{Stdout: pigListModelsOutput}, nil
	}
	got, err := pigHarness{}.Models(run)
	if err != nil {
		t.Fatal(err)
	}
	// A model's id keeps its provider: pig routes a bare id to openai.
	want := []Model{
		{ID: "deepseek/deepseek-flash", Efforts: pigEffortLevels},
		{ID: "openrouter/amazon/nova-lite-v1"},
		{ID: "opencode-go/kimi-k3", Efforts: pigEffortLevels},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("pig models = %+v, want %+v", got, want)
	}
}

// The header fixes the columns, so a blank cell does not shift the thinking
// column into the images one.
func TestPigModelsReadTheThinkingColumnAcrossABlankCell(t *testing.T) {
	out := "\nprovider     model    context  max-out  thinking  images\n" +
		"deepseek     flash    1M                no        yes   \n"
	got := parsePigModels(out)
	want := []Model{{ID: "deepseek/flash"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("pig models = %+v, want %+v", got, want)
	}
}

func TestClaudeModelsParseTheInitializeResponse(t *testing.T) {
	var stdin string
	run := func(cmd string, args []string, o ExecOpts) (Result, error) {
		if cmd != "claude" {
			t.Fatalf("claude listing ran %s", cmd)
		}
		for _, want := range []string{"-p", "--input-format", "stream-json", "--output-format", "stream-json", "--no-session-persistence", "--verbose"} {
			if !containsArg(args, want) {
				t.Errorf("claude listing is missing %q: %v", want, args)
			}
		}
		stdin = o.Stdin
		return Result{Stdout: claudeInitializeResponse}, nil
	}
	got, err := claudeHarness{}.Models(run)
	if err != nil {
		t.Fatal(err)
	}
	want := []Model{
		{ID: "default", Label: "Default (recommended)", Efforts: []string{"low", "medium", "high", "xhigh", "max"}},
		{ID: "haiku", Label: "Haiku 4.5"},
		{ID: "opus", Label: "Opus 5.5", Efforts: []string{"low", "medium", "high", "xhigh", "max"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("claude models = %+v, want %+v", got, want)
	}
	// No user message is sent, so the listing spends no tokens; the only thing
	// on stdin is the initialize control request.
	if !strings.Contains(stdin, `"subtype":"initialize"`) {
		t.Errorf("stdin %q is not the initialize request", stdin)
	}
	if strings.Contains(stdin, `"type":"user"`) {
		t.Errorf("stdin %q carries a user message", stdin)
	}
}

func containsArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

// A harness's list is cached under the state dir; the second read is the cache,
// not a second CLI run.
func TestCachedModelsBuildsOnceThenReadsTheCache(t *testing.T) {
	isolateState(t)
	calls := 0
	run := func(string, []string, ExecOpts) (Result, error) {
		calls++
		return Result{Stdout: pigListModelsOutput}, nil
	}
	first, err := CachedModels("pig", run, false)
	if err != nil {
		t.Fatal(err)
	}
	second, err := CachedModels("pig", run, false)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Errorf("the CLI ran %d times, want the cache to answer the second read", calls)
	}
	if !reflect.DeepEqual(first, second) {
		t.Errorf("cache = %+v, want %+v", second, first)
	}
	var cached struct {
		At time.Time `json:"at"`
	}
	raw, err := os.ReadFile(modelCachePath("pig"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &cached); err != nil {
		t.Fatalf("cache %s is not JSON: %v", raw, err)
	}
	if cached.At.IsZero() || time.Since(cached.At) > time.Minute {
		t.Errorf("cache timestamp is %v, want the time it was built", cached.At)
	}
}

func TestCachedModelsRefreshRebuilds(t *testing.T) {
	isolateState(t)
	calls := 0
	run := func(string, []string, ExecOpts) (Result, error) {
		calls++
		return Result{Stdout: pigListModelsOutput}, nil
	}
	if _, err := CachedModels("pig", run, false); err != nil {
		t.Fatal(err)
	}
	if _, err := CachedModels("pig", run, true); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Errorf("--refresh ran the CLI %d times, want 2", calls)
	}
}

// A refresh that fails still answers from the cache it has, so a broken CLI
// does not empty a picker.
func TestCachedModelsFallsBackToTheCacheWhenRefreshFails(t *testing.T) {
	isolateState(t)
	ok := func(string, []string, ExecOpts) (Result, error) { return Result{Stdout: pigListModelsOutput}, nil }
	want, err := CachedModels("pig", ok, false)
	if err != nil {
		t.Fatal(err)
	}
	fail := func(string, []string, ExecOpts) (Result, error) { return Result{Code: 1, Stderr: "boom"}, nil }
	got, err := CachedModels("pig", fail, true)
	if err != nil {
		t.Fatalf("a failed refresh with a cache present should fall back, got %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("fallback = %+v, want %+v", got, want)
	}
}

func TestCachedModelsNamesAnUnknownHarness(t *testing.T) {
	isolateState(t)
	if _, err := CachedModels("gemini", func(string, []string, ExecOpts) (Result, error) { return Result{}, nil }, false); err == nil {
		t.Fatal("an unknown harness should be an error")
	}
}

// The Exec seam writes stdin and closes it. That is what lets claude's
// stream-json listing answer and exit without a turn.
func TestOSExecWritesStdin(t *testing.T) {
	res, err := osExec("cat", nil, ExecOpts{Stdin: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Stdout != "hello" {
		t.Fatalf("stdout = %q, want the stdin it was handed", res.Stdout)
	}
}
