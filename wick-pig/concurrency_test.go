package wick

// Reusable multi-process concurrency helper.
//
// Several wick sessions are separate wick-mcp processes, so an in-process
// mutex cannot serialize what they do to shared state. A goroutine test
// cannot reproduce that: every goroutine shares the mutex. This helper
// starts N real helper processes (the test binary re-executed with
// -test.run=TestConcurrencyHelperProcess) against a file-backed fake herdr
// that records splits, closes and pane lists. The grid, reviewer and spacing
// tickets reuse the runner and the fake; this ticket uses the worker-spawn
// and worker-finish modes.

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// ---------------------------------------------------------------------------
// file-backed fake herdr
// ---------------------------------------------------------------------------

// concurrencyHerdrState is the fake's shared state, kept in one JSON file so
// every helper process sees every other process's tabs and panes.
type concurrencyHerdrState struct {
	Tabs   map[string]string   `json:"tabs"`   // label -> tab id
	Labels map[string]string   `json:"labels"` // tab id -> label
	Panes  map[string][]string `json:"panes"`  // tab id -> pane ids in creation order
	Next   int                 `json:"next"`
}

func concurrencyHerdrDir() string { return os.Getenv("WICK_CONCURRENCY_HERDR") }

func concurrencyHerdrFile() string { return filepath.Join(concurrencyHerdrDir(), "herdr.json") }

func concurrencyHerdrCalls() string { return filepath.Join(concurrencyHerdrDir(), "calls.log") }

func loadConcurrencyHerdr() concurrencyHerdrState {
	st := concurrencyHerdrState{Tabs: map[string]string{}, Labels: map[string]string{}, Panes: map[string][]string{}}
	raw, err := os.ReadFile(concurrencyHerdrFile())
	if err != nil {
		return st
	}
	_ = json.Unmarshal(raw, &st)
	if st.Tabs == nil {
		st.Tabs = map[string]string{}
	}
	if st.Labels == nil {
		st.Labels = map[string]string{}
	}
	if st.Panes == nil {
		st.Panes = map[string][]string{}
	}
	return st
}

func saveConcurrencyHerdr(st concurrencyHerdrState) {
	_ = os.MkdirAll(concurrencyHerdrDir(), 0o755)
	raw, _ := json.Marshal(st)
	_ = os.WriteFile(concurrencyHerdrFile()+".tmp", append(raw, '\n'), 0o644)
	_ = os.Rename(concurrencyHerdrFile()+".tmp", concurrencyHerdrFile())
}

// logConcurrencyCall appends one herdr call to the shared log, so the parent
// can assert which splits, closes and pane lists happened.
func logConcurrencyCall(args []string) {
	_ = withFileLock("concurrency-herdr-log", func() error {
		_ = os.MkdirAll(concurrencyHerdrDir(), 0o755)
		f, err := os.OpenFile(concurrencyHerdrCalls(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		defer f.Close()
		_, _ = fmt.Fprintln(f, strings.Join(args, " "))
		return nil
	})
}

// concurrencyHerdrExec is an Exec that answers herdr calls from the shared
// file state and delegates everything else (git) to the real osExec. The
// state update for each call happens under a cross-process lock, so the
// fake itself never corrupts; the caller's own check-then-act race (two
// spawns both seeing no tab and both creating one) still shows, which is
// what the concurrency tests assert on.
func concurrencyHerdrExec() Exec {
	return func(cmd string, args []string, opts ExecOpts) (Result, error) {
		if cmd != herdrBin() {
			return osExec(cmd, args, opts)
		}
		logConcurrencyCall(args)
		if len(args) < 2 {
			return Result{Stdout: "{}"}, nil
		}
		reply := func(result map[string]any) Result {
			b, _ := json.Marshal(map[string]any{"id": "cli", "result": result})
			return Result{Stdout: string(b)}
		}
		switch args[0] + " " + args[1] {
		case "tab list":
			var out concurrencyHerdrState
			_ = withFileLock("concurrency-herdr", func() error {
				out = loadConcurrencyHerdr()
				return nil
			})
			tabs := []any{}
			for label, id := range out.Tabs {
				tabs = append(tabs, map[string]any{"tab_id": id, "label": label, "workspace_id": "w1"})
			}
			return reply(map[string]any{"tabs": tabs}), nil
		case "tab create":
			label, _ := flag(args, "--label")
			var id, pane string
			_ = withFileLock("concurrency-herdr", func() error {
				st := loadConcurrencyHerdr()
				st.Next++
				id = fmt.Sprintf("w1:t%d", st.Next)
				pane = fmt.Sprintf("w1:p%d", st.Next)
				// A second create for the same label still creates: the
				// caller's check-then-create is what must prevent it.
				st.Tabs[label] = id
				st.Labels[id] = label
				st.Panes[id] = []string{pane}
				saveConcurrencyHerdr(st)
				return nil
			})
			return reply(map[string]any{
				"tab":       map[string]any{"tab_id": id, "label": label, "workspace_id": "w1"},
				"root_pane": map[string]any{"pane_id": pane, "tab_id": id},
			}), nil
		case "tab close":
			_ = withFileLock("concurrency-herdr", func() error {
				st := loadConcurrencyHerdr()
				id := args[2]
				for label, tid := range st.Tabs {
					if tid == id {
						delete(st.Tabs, label)
					}
				}
				delete(st.Labels, id)
				delete(st.Panes, id)
				saveConcurrencyHerdr(st)
				return nil
			})
			return Result{Stdout: `{"result":{}}`}, nil
		case "pane list":
			var out concurrencyHerdrState
			_ = withFileLock("concurrency-herdr", func() error {
				out = loadConcurrencyHerdr()
				return nil
			})
			panes := []any{}
			for tab, ps := range out.Panes {
				for _, p := range ps {
					panes = append(panes, map[string]any{"pane_id": p, "tab_id": tab, "workspace_id": "w1"})
				}
			}
			return reply(map[string]any{"panes": panes}), nil
		case "pane split":
			pane, _ := flag(args, "--pane")
			var id, tab string
			_ = withFileLock("concurrency-herdr", func() error {
				st := loadConcurrencyHerdr()
				for tid, ps := range st.Panes {
					for _, p := range ps {
						if p == pane {
							tab = tid
						}
					}
				}
				st.Next++
				id = fmt.Sprintf("w1:p%d", st.Next)
				if tab != "" {
					st.Panes[tab] = append(st.Panes[tab], id)
				}
				saveConcurrencyHerdr(st)
				return nil
			})
			if tab == "" {
				return Result{Code: 1, Stderr: "no pane " + pane}, nil
			}
			return reply(map[string]any{"pane": map[string]any{"pane_id": id, "tab_id": tab}}), nil
		case "pane close":
			_ = withFileLock("concurrency-herdr", func() error {
				st := loadConcurrencyHerdr()
				for tid, ps := range st.Panes {
					kept := ps[:0]
					for _, p := range ps {
						if p != args[2] {
							kept = append(kept, p)
						}
					}
					st.Panes[tid] = kept
				}
				saveConcurrencyHerdr(st)
				return nil
			})
			return Result{Stdout: `{"result":{}}`}, nil
		case "pane run":
			return Result{}, nil
		case "pane process-info":
			pane, _ := flag(args, "--pane")
			return reply(map[string]any{"process_info": map[string]any{
				"pane_id": pane, "shell_pid": 4000,
				"foreground_process_group_id": 0, "foreground_processes": []any{},
			}}), nil
		case "agent list":
			return reply(map[string]any{"agents": []any{}}), nil
		}
		return Result{Stdout: "{}"}, nil
	}
}

// ---------------------------------------------------------------------------
// helper processes
// ---------------------------------------------------------------------------

// TestConcurrencyHelperProcess is the helper entry point: the parent
// re-executes the test binary with -test.run=TestConcurrencyHelperProcess and
// WICK_CONCURRENCY_MODE set. A normal `go test` run has no mode and skips.
func TestConcurrencyHelperProcess(t *testing.T) {
	mode := os.Getenv("WICK_CONCURRENCY_MODE")
	if mode == "" {
		t.Skip("no helper mode")
	}
	statePath = os.Getenv("WICK_CONCURRENCY_STATE")
	t.Setenv("HERDR_WORKSPACE_ID", "w1")
	run := concurrencyHerdrExec()
	repo := os.Getenv("WICK_CONCURRENCY_REPO")
	id := os.Getenv("WICK_CONCURRENCY_ID")
	result := os.Getenv("WICK_CONCURRENCY_RESULT")

	writeResult := func(err error, extra map[string]string) {
		out := map[string]string{"id": id}
		if err != nil {
			out["error"] = err.Error()
		}
		for k, v := range extra {
			out[k] = v
		}
		raw, _ := json.Marshal(out)
		_ = os.WriteFile(result, append(raw, '\n'), 0o644)
	}

	switch mode {
	case "worker-spawn":
		ticket := filepath.Join(repo, ".scratch", id+".md")
		w, err := Spawn(run, SpawnInput{Ticket: ticket, Repo: repo, Base: "HEAD", Launch: []string{"true"}})
		if err != nil {
			writeResult(err, nil)
			t.Fatalf("helper %s spawn: %v", id, err)
		}
		writeResult(nil, map[string]string{"pane": w.Pane, "tab": w.Tab})
	case "worker-finish":
		_, err := Finish(run, id, false)
		if err != nil {
			writeResult(err, nil)
			t.Fatalf("helper %s finish: %v", id, err)
		}
		writeResult(nil, nil)
	default:
		t.Fatalf("unknown helper mode %q", mode)
	}
}

// runConcurrencyHelpers starts n helper processes with the same mode and per
// helper env, all at once, and waits for every one. It returns each helper's
// result file decoded. A helper that exits non-zero fails the test with its
// output attached.
func runConcurrencyHelpers(t *testing.T, mode string, ids []string, env map[string]string) []map[string]string {
	t.Helper()
	var wg sync.WaitGroup
	errs := make([]error, len(ids))
	outputs := make([]string, len(ids))
	for i, id := range ids {
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			result := filepath.Join(os.Getenv("WICK_CONCURRENCY_HERDR"), "result-"+id+".json")
			args := []string{"-test.run", "TestConcurrencyHelperProcess", "-test.count=1"}
			cmd := exec.Command(os.Args[0], args...)
			cmd.Env = append(os.Environ(),
				"WICK_CONCURRENCY_MODE="+mode,
				"WICK_CONCURRENCY_ID="+id,
				"WICK_CONCURRENCY_RESULT="+result,
			)
			for k, v := range env {
				cmd.Env = append(cmd.Env, k+"="+v)
			}
			out, err := cmd.CombinedOutput()
			outputs[i] = string(out)
			errs[i] = err
		}(i, id)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("helper %s (%s) failed: %v\n%s", ids[i], mode, err, outputs[i])
		}
	}
	out := make([]map[string]string, len(ids))
	for i, id := range ids {
		result := filepath.Join(os.Getenv("WICK_CONCURRENCY_HERDR"), "result-"+id+".json")
		raw, err := os.ReadFile(result)
		if err != nil {
			t.Fatalf("helper %s wrote no result: %v\n%s", id, err, outputs[i])
		}
		var m map[string]string
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("helper %s result is not JSON: %v", id, err)
		}
		if m["error"] != "" {
			t.Fatalf("helper %s reported: %s", id, m["error"])
		}
		out[i] = m
	}
	return out
}

// ---------------------------------------------------------------------------
// worker store test (spec stories 34 and 37)
// ---------------------------------------------------------------------------

// Several real processes spawn workers at once against the fake herdr: every
// requested worker must exist afterwards, own a distinct pane, and appear in
// the store, and no concurrent reader may ever see a half-written store.
// Several real processes then finish them at once: none may be lost either.
func TestWorkerStoreSurvivesSimultaneousSessions(t *testing.T) {
	oldState := statePath
	stateFile := filepath.Join(t.TempDir(), "workers.json")
	statePath = stateFile
	t.Cleanup(func() { statePath = oldState })

	herdrDir := t.TempDir()
	t.Setenv("WICK_CONCURRENCY_HERDR", herdrDir)
	t.Setenv("WICK_CONCURRENCY_STATE", stateFile)
	t.Setenv("HERDR_WORKSPACE_ID", "w1")

	repo := t.TempDir()
	gitOut(t, repo, "init", "-q", "-b", "main")
	gitOut(t, repo, "config", "user.email", "wick@test.local")
	gitOut(t, repo, "config", "user.name", "wick")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitOut(t, repo, "add", ".")
	gitOut(t, repo, "commit", "-qm", "base")
	if err := os.MkdirAll(filepath.Join(repo, ".scratch"), 0o755); err != nil {
		t.Fatal(err)
	}
	const n = 6
	ids := make([]string, n)
	for i := range ids {
		ids[i] = fmt.Sprintf("concurrent-%d", i+1)
		if err := os.WriteFile(filepath.Join(repo, ".scratch", ids[i]+".md"), []byte("# "+ids[i]+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("WICK_CONCURRENCY_REPO", repo)

	// Readers loop while writers write: any half-written file fails the JSON
	// decode and is reported as corruption.
	var corrupt int64
	stop := make(chan struct{})
	var readers sync.WaitGroup
	for range 4 {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-stop:
					return
				default:
					if _, err := load(); err != nil {
						atomic.AddInt64(&corrupt, 1)
						return
					}
				}
			}
		}()
	}

	runConcurrencyHelpers(t, "worker-spawn", ids, map[string]string{
		"WICK_CONCURRENCY_STATE": stateFile,
		"WICK_CONCURRENCY_REPO":  repo,
		"WICK_CONCURRENCY_HERDR": herdrDir,
	})
	close(stop)
	readers.Wait()
	if corrupt > 0 {
		t.Fatalf("a concurrent reader saw a partial worker store %d times", corrupt)
	}

	store, err := load()
	if err != nil {
		t.Fatalf("load after concurrent spawns: %v", err)
	}
	if len(store) != n {
		got := []string{}
		for id := range store {
			got = append(got, id)
		}
		t.Fatalf("worker store holds %d of %d workers (%v): an entry was lost", len(store), n, got)
	}
	seen := map[string]string{}
	for _, id := range ids {
		w, ok := store[id]
		if !ok {
			t.Fatalf("worker %q is missing after concurrent spawns", id)
		}
		if w.Pane == "" {
			t.Errorf("worker %q owns no pane", id)
		}
		if prev, dup := seen[w.Pane]; dup {
			t.Errorf("workers %q and %q share pane %q", prev, id, w.Pane)
		}
		seen[w.Pane] = id
	}

	// The fake records every herdr call, so later tickets can assert on
	// splits, closes and pane lists from the same log.
	raw, err := os.ReadFile(concurrencyHerdrCalls())
	if err != nil || len(raw) == 0 {
		t.Fatalf("the fake herdr recorded no calls: %v", err)
	}

	stop2 := make(chan struct{})
	corrupt = 0
	for range 4 {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-stop2:
					return
				default:
					if _, err := load(); err != nil {
						atomic.AddInt64(&corrupt, 1)
						return
					}
				}
			}
		}()
	}
	runConcurrencyHelpers(t, "worker-finish", ids, map[string]string{
		"WICK_CONCURRENCY_STATE": stateFile,
		"WICK_CONCURRENCY_REPO":  repo,
		"WICK_CONCURRENCY_HERDR": herdrDir,
	})
	close(stop2)
	readers.Wait()
	if corrupt > 0 {
		t.Fatalf("a concurrent reader saw a partial worker store during finish %d times", corrupt)
	}
	rest, err := load()
	if err != nil {
		t.Fatalf("load after concurrent finishes: %v", err)
	}
	if len(rest) != 0 {
		t.Fatalf("worker store holds %d workers after concurrent finishes, want 0", len(rest))
	}
}
