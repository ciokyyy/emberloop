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
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
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
func concurrencyHerdrExec() Exec {
	// WICK_CONCURRENCY_DELAY_MS slows every fake herdr call, so helpers that
	// start together still overlap in the caller's check-then-act window: a
	// fast helper would otherwise finish before a slow one arrives and no
	// race would show.
	delayMS, _ := strconv.Atoi(os.Getenv("WICK_CONCURRENCY_DELAY_MS"))
	delay := time.Duration(delayMS) * time.Millisecond
	return func(cmd string, args []string, opts ExecOpts) (Result, error) {
		if delay > 0 && cmd == herdrBin() {
			time.Sleep(delay)
		}
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
		case "pane process-info":
			pane := paneArg(args)
			found := false
			_ = withFileLock("concurrency-herdr", func() error {
				st := loadConcurrencyHerdr()
				for _, ps := range st.Panes {
					for _, p := range ps {
						if p == pane {
							found = true
						}
					}
				}
				return nil
			})
			if !found {
				return Result{Code: 1, Stdout: `{"error":{"code":"pane_not_found"}}`}, nil
			}
			return livePaneProcs(pane), nil
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
	// The start barrier holds every helper until they have all arrived, so
	// every helper overlaps every other one in the work below. Without it a
	// fast helper finishes before a slow one starts and a check-then-act
	// race never shows.
	if os.Getenv("WICK_CONCURRENCY_BARRIER") != "" {
		waitConcurrencyBarrier(t, id)
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
	case "reviewer-spawn":
		// One reviewer spawn through the spawn tool path, the way a worker
		// spawns its reviewers. The tree and type come from the parent; the
		// barrier holds every helper until all are started, so the
		// check-then-launch windows overlap and a missing lock shows as two
		// panes for one key.
		toolBudget = time.Second
		pollEvery = 5 * time.Millisecond
		spawnGap = 0
		barrier := filepath.Join(concurrencyHerdrDir(), "barrier")
		_ = os.MkdirAll(barrier, 0o755)
		_ = os.WriteFile(filepath.Join(barrier, "ready-"+id), []byte("ready"), 0o644)
		if want := os.Getenv("WICK_CONCURRENCY_WANT"); want != "" {
			wantN, _ := strconv.Atoi(want)
			deadline := time.Now().Add(10 * time.Second)
			for {
				entries, _ := os.ReadDir(barrier)
				if len(entries) >= wantN {
					break
				}
				if time.Now().After(deadline) {
					writeResult(fmt.Errorf("barrier: only %d of %s helpers ready", len(entries), want), nil)
					t.Fatalf("helper %s barrier: only %d of %s helpers ready", id, len(entries), want)
				}
				time.Sleep(5 * time.Millisecond)
			}
		}
		tree := os.Getenv("WICK_CONCURRENCY_TREE")
		typ := os.Getenv("WICK_CONCURRENCY_TYPE")
		types := mustLoadAgentTypes(repo)
		rt, ok := types[typ]
		if !ok {
			writeResult(fmt.Errorf("unknown reviewer type %q", typ), nil)
			t.Fatalf("helper %s: unknown reviewer type %q", id, typ)
		}
		_ = rt
		if _, err := spawnAgents(run, tree, types, []spawnRequest{{Type: typ}}); err != nil {
			writeResult(err, nil)
			t.Fatalf("helper %s reviewer spawn: %v", id, err)
		}
		writeResult(nil, nil)
	case "grid-place":
		// One grid placement in the shared tab, the way a scout or reviewer
		// launch places its pane. The result carries the slot the session
		// took, so the parent can tell two sessions that took the same one.
		tab := os.Getenv("WICK_CONCURRENCY_TAB")
		overflow := os.Getenv("WICK_CONCURRENCY_OVERFLOW")
		if overflow == "" {
			overflow = "scouts"
		}
		placed, err := placeInGrid(run, tab, overflow, repo)
		if err != nil {
			writeResult(err, nil)
			t.Fatalf("helper %s place: %v", id, err)
		}
		writeResult(nil, map[string]string{
			"pane": placed.Pane,
			"tab":  placed.Tab,
			"slot": strconv.Itoa(placed.Slot),
		})
	case "spacing":
		// Two processes calling spaceSpawns at the same instant must take
		// successive slots: the gap comes from the env so the test controls it.
		if g := os.Getenv("WICK_CONCURRENCY_GAP"); g != "" {
			if d, err := time.ParseDuration(g); err == nil {
				spawnGap = d
			}
		}
		spaceSpawns()
		writeResult(nil, map[string]string{"at": strconv.FormatInt(time.Now().UnixNano(), 10)})
	default:
		t.Fatalf("unknown helper mode %q", mode)
	}
}

// waitConcurrencyBarrier signals this helper's arrival and waits until the
// parent releases every helper at once.
func waitConcurrencyBarrier(t *testing.T, id string) {
	t.Helper()
	dir := concurrencyHerdrDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("helper %s barrier: %v", id, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ready-"+id), []byte("ready\n"), 0o644); err != nil {
		t.Fatalf("helper %s ready: %v", id, err)
	}
	deadline := time.Now().Add(60 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(dir, "start")); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("helper %s timed out waiting for the start barrier", id)
		}
		time.Sleep(2 * time.Millisecond)
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

// runConcurrencyHelpersTogether starts n helper processes that all begin
// their work at the same moment: each helper signals readiness and waits for
// the parent to release the start barrier, so every helper overlaps every
// other one in the mode's work. A staggered start would let a fast helper
// finish before a slow one arrives and a check-then-act race would never
// show. tabFor optionally overrides WICK_CONCURRENCY_TAB per helper, so one
// run can place from several tabs at once.
func runConcurrencyHelpersTogether(t *testing.T, mode string, ids []string, env map[string]string, tabFor map[string]string) []map[string]string {
	t.Helper()
	dir := os.Getenv("WICK_CONCURRENCY_HERDR")
	_ = os.Remove(filepath.Join(dir, "start"))
	for _, id := range ids {
		_ = os.Remove(filepath.Join(dir, "ready-"+id))
	}
	var wg sync.WaitGroup
	errs := make([]error, len(ids))
	outputs := make([]string, len(ids))
	for i, id := range ids {
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			result := filepath.Join(dir, "result-"+id+".json")
			args := []string{"-test.run", "TestConcurrencyHelperProcess", "-test.count=1"}
			cmd := exec.Command(os.Args[0], args...)
			cmd.Env = append(os.Environ(),
				"WICK_CONCURRENCY_MODE="+mode,
				"WICK_CONCURRENCY_ID="+id,
				"WICK_CONCURRENCY_RESULT="+result,
				"WICK_CONCURRENCY_BARRIER=1",
			)
			for k, v := range env {
				cmd.Env = append(cmd.Env, k+"="+v)
			}
			if tab, ok := tabFor[id]; ok {
				cmd.Env = append(cmd.Env, "WICK_CONCURRENCY_TAB="+tab)
			}
			out, err := cmd.CombinedOutput()
			outputs[i] = string(out)
			errs[i] = err
		}(i, id)
	}
	// Release the barrier once every helper is waiting at it.
	deadline := time.Now().Add(60 * time.Second)
	for _, id := range ids {
		for {
			if _, err := os.Stat(filepath.Join(dir, "ready-"+id)); err == nil {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("helper %s never reached the barrier", id)
			}
			time.Sleep(2 * time.Millisecond)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "start"), []byte("go\n"), 0o644); err != nil {
		t.Fatalf("release the start barrier: %v", err)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("helper %s (%s) failed: %v\n%s", ids[i], mode, err, outputs[i])
		}
	}
	out := make([]map[string]string, len(ids))
	for i, id := range ids {
		result := filepath.Join(dir, "result-"+id+".json")
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

// ---------------------------------------------------------------------------
// reviewer race test (spec story 35)
// ---------------------------------------------------------------------------

// Two simultaneous spawns of the same reviewer type for one worktree must
// produce one reviewer, not two: the second re-prompts the one already
// starting. The check-then-launch runs under a cross-process lock per worktree
// and type, so the record cannot be overwritten and leave an orphan pane.
//
// Seams (pre-agreed per the spec's Testing Decisions): the spawn tool path
// (spawnAgents, the way a worker spawns its reviewers) and the shared stores
// (reviewersOf plus the file-backed fake herdr's pane list and call log).
// Good tests assert behaviour through the tool, not internals.
func TestConcurrentReviewerSpawnProducesOneReviewer(t *testing.T) {
	oldState := statePath
	stateFile := filepath.Join(t.TempDir(), "workers.json")
	statePath = stateFile
	t.Cleanup(func() { statePath = oldState })

	herdrDir := t.TempDir()
	t.Setenv("WICK_CONCURRENCY_HERDR", herdrDir)
	t.Setenv("WICK_CONCURRENCY_STATE", stateFile)
	t.Setenv("HERDR_WORKSPACE_ID", "w1")
	resolverSettings(t, "")
	resetGrid()

	oldGap, oldPoll, oldBudget := spawnGap, pollEvery, toolBudget
	spawnGap, pollEvery, toolBudget = 0, 5*time.Millisecond, time.Second
	t.Cleanup(func() { spawnGap, pollEvery, toolBudget = oldGap, oldPoll, oldBudget })

	repo := t.TempDir()
	gitOut(t, repo, "init", "-q", "-b", "main")
	gitOut(t, repo, "config", "user.email", "wick@test.local")
	gitOut(t, repo, "config", "user.name", "wick")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitOut(t, repo, "add", ".")
	gitOut(t, repo, "commit", "-qm", "base")
	base := gitOut(t, repo, "rev-parse", "HEAD")

	mkWorktree := func(id string) string {
		tree := filepath.Join(repo, ".wick-worktrees", id)
		gitOut(t, repo, "worktree", "add", "-q", "-b", "implement/"+id, tree, "main")
		writeFile(t, filepath.Join(tree, "a.go"), "package a\n\nfunc A() {}\n")
		gitOut(t, tree, "add", ".")
		gitOut(t, tree, "commit", "-qam", "work "+id)
		return tree
	}
	tree1 := mkWorktree("07-x")
	tree2 := mkWorktree("08-y")
	if err := save(map[string]Worker{
		"07-x": {ID: "07-x", Ticket: "/t/07-x.md", Spec: "/t/spec.md", Repo: repo, Base: base, Target: "main",
			Branch: "implement/07-x", Worktree: tree1, Pane: "w1:p1", Tab: "w1:t1", Workspace: "w1",
			AgentType: "worker", Started: "2026-10-05T00:00:00Z"},
		"08-y": {ID: "08-y", Ticket: "/t/08-y.md", Spec: "/t/spec.md", Repo: repo, Base: base, Target: "main",
			Branch: "implement/08-y", Worktree: tree2, Pane: "w1:p2", Tab: "w1:t2", Workspace: "w1",
			AgentType: "worker", Started: "2026-10-05T00:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}
	saveConcurrencyHerdr(concurrencyHerdrState{
		Tabs:   map[string]string{"07-x": "w1:t1", "08-y": "w1:t2"},
		Labels: map[string]string{"w1:t1": "07-x", "w1:t2": "08-y"},
		Panes:  map[string][]string{"w1:t1": {"w1:p1"}, "w1:t2": {"w1:p2"}},
		Next:   2,
	})

	splits := func() int {
		raw, _ := os.ReadFile(concurrencyHerdrCalls())
		return strings.Count(string(raw), "pane split")
	}
	assertNoOrphan := func(tree, tab, root string) {
		t.Helper()
		st := loadConcurrencyHerdr()
		have := reviewersOf(tree)
		stored := map[string]bool{}
		for _, r := range have {
			stored[r.Pane] = true
		}
		raw, _ := os.ReadFile(concurrencyHerdrCalls())
		for _, pane := range st.Panes[tab] {
			if pane == root {
				continue
			}
			if stored[pane] {
				continue
			}
			if strings.Contains(string(raw), "pane close "+pane) {
				continue
			}
			t.Fatalf("pane %s in %s is orphaned: not in the reviewers record and never closed (panes %v, stored %v)",
				pane, tab, st.Panes[tab], stored)
		}
	}

	// Phase 1: the race. Two processes spawn reviewer-standards for tree1 at
	// once, behind a barrier so their check-then-launch windows overlap.
	runConcurrencyHelpers(t, "reviewer-spawn", []string{"rv-a", "rv-b"}, map[string]string{
		"WICK_CONCURRENCY_STATE": stateFile,
		"WICK_CONCURRENCY_REPO":  repo,
		"WICK_CONCURRENCY_HERDR": herdrDir,
		"WICK_CONCURRENCY_TREE":  tree1,
		"WICK_CONCURRENCY_TYPE":  reviewerStandards,
		"WICK_CONCURRENCY_WANT":  "2",
	})
	have := reviewersOf(tree1)
	if len(have) != 1 {
		t.Fatalf("two simultaneous %s spawns for one worktree must leave one reviewer, got %d (%v)",
			reviewerStandards, len(have), have)
	}
	r, ok := have[reviewerStandards]
	if !ok || r.Pane == "" {
		t.Fatalf("the one reviewer must be %s with a pane, got %v", reviewerStandards, have)
	}
	if got := splits(); got != 1 {
		t.Fatalf("two simultaneous spawns of one reviewer type must split one pane, got %d splits", got)
	}
	// The second spawn re-prompted the one already starting instead of
	// launching: the follow-up nudge is on the wire.
	if raw, _ := os.ReadFile(concurrencyHerdrCalls()); !strings.Contains(string(raw), "agent prompt") &&
		!strings.Contains(string(raw), "pane send-text") {
		t.Fatalf("the second simultaneous spawn must re-prompt the reviewer already starting (a nudge), got:\n%s", raw)
	}
	assertNoOrphan(tree1, "w1:t1", "w1:p1")

	// Phase 2: a different type for the same worktree is unaffected: it gets
	// its own reviewer beside the first.
	types := mustLoadAgentTypes(repo)
	if _, err := spawnAgents(concurrencyHerdrExec(), tree1, types, []spawnRequest{{Type: reviewerSpec}}); err != nil {
		t.Fatalf("spawn %s for the same worktree: %v", reviewerSpec, err)
	}
	have = reviewersOf(tree1)
	if len(have) != 2 {
		t.Fatalf("a different reviewer type must get its own reviewer, got %v", have)
	}
	if got := splits(); got != 2 {
		t.Fatalf("the second type must split its own pane, got %d splits total", got)
	}
	assertNoOrphan(tree1, "w1:t1", "w1:p1")

	// Phase 3: the same type for a different worktree is unaffected: it gets
	// its own reviewer under its own key.
	if _, err := spawnAgents(concurrencyHerdrExec(), tree2, types, []spawnRequest{{Type: reviewerStandards}}); err != nil {
		t.Fatalf("spawn %s for another worktree: %v", reviewerStandards, err)
	}
	have2 := reviewersOf(tree2)
	if len(have2) != 1 || have2[reviewerStandards].Pane == "" {
		t.Fatalf("the same type for another worktree must get its own reviewer, got %v", have2)
	}
	if have2[reviewerStandards].Pane == reviewersOf(tree1)[reviewerStandards].Pane {
		t.Fatalf("different worktrees must not share a reviewer pane: %v vs %v", have2, reviewersOf(tree1))
	}
	assertNoOrphan(tree2, "w1:t2", "w1:p2")
}

// Two processes launching at the same instant are separated by the configured
// pause: the spacing timestamp is shared under a lock, not per process (spec
// stories 36 and 37).
func TestLaunchesAreSpacedAcrossSessions(t *testing.T) {
	oldState, oldGap := statePath, spawnGap
	stateFile := filepath.Join(t.TempDir(), "workers.json")
	statePath = stateFile
	spawnGap = time.Second
	t.Cleanup(func() { statePath, spawnGap = oldState, oldGap })
	herdrDir := t.TempDir()
	t.Setenv("WICK_CONCURRENCY_HERDR", herdrDir)
	t.Setenv("WICK_CONCURRENCY_STATE", stateFile)
	results := runConcurrencyHelpers(t, "spacing", []string{"a", "b"}, map[string]string{
		"WICK_CONCURRENCY_STATE": stateFile,
		"WICK_CONCURRENCY_HERDR": herdrDir,
		"WICK_CONCURRENCY_GAP":   "1s",
	})
	at := make([]int64, 2)
	for i, r := range results {
		n, err := strconv.ParseInt(r["at"], 10, 64)
		if err != nil {
			t.Fatalf("helper %d reported no timestamp: %v", i, r)
		}
		at[i] = n
	}
	diff := at[0] - at[1]
	if diff < 0 {
		diff = -diff
	}
	if diff < int64(800*time.Millisecond) {
		t.Fatalf("two simultaneous launches are %v apart, want at least the 1s pause", time.Duration(diff))
	}
}
