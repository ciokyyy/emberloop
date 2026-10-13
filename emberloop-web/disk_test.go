package web

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testDiskFS(tmp, cache string, now time.Time) diskFS {
	return diskFS{
		now:     func() time.Time { return now },
		tmpDir:  func() string { return tmp },
		cacheDr: func() string { return cache },
		remove:  removeDiskPath,
		runGit: func(dir string, args ...string) (string, error) {
			return runGit(dir, args...)
		},
		logf: func(string, ...any) {},
	}
}

func writeSized(t *testing.T, path string, n int, mod time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, n), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mod, mod); err != nil {
		t.Fatal(err)
	}
}

func TestPruneGoCacheOldestFirst(t *testing.T) {
	t.Parallel()
	dir := tempDir(t)
	now := time.Now()
	old := now.Add(-time.Hour)
	writeSized(t, filepath.Join(dir, "a"), 6<<10, old.Add(-time.Hour))
	writeSized(t, filepath.Join(dir, "b"), 6<<10, old)
	writeSized(t, filepath.Join(dir, "young"), 6<<10, now)
	fs := testDiskFS("", dir, now)
	freed, removed := pruneGoCache(fs, dir, 10<<10)
	if removed != 2 {
		t.Fatalf("removed = %d, want 2 (both old files, young guarded)", removed)
	}
	if freed != 12<<10 {
		t.Fatalf("freed = %d, want %d", freed, 12<<10)
	}
	if _, err := os.Stat(filepath.Join(dir, "young")); err != nil {
		t.Fatal("young file must survive the 10-minute guard")
	}
}

func TestPruneGoCacheOffWhenZero(t *testing.T) {
	t.Parallel()
	dir := tempDir(t)
	writeSized(t, filepath.Join(dir, "a"), 1<<20, time.Now().Add(-time.Hour))
	fs := testDiskFS("", dir, time.Now())
	if freed, removed := pruneGoCache(fs, dir, 0); freed != 0 || removed != 0 {
		t.Fatalf("cap 0 pruned %d bytes in %d files, want nothing", freed, removed)
	}
}

func TestCleanTempDirsOnlyStalePrefixed(t *testing.T) {
	t.Parallel()
	tmp := tempDir(t)
	now := time.Now()
	old := now.Add(-7 * time.Hour)
	fresh := now.Add(-time.Hour)
	for _, name := range []string{"el-web-test-old", "el-test-old", "go-build-old"} {
		p := filepath.Join(tmp, name)
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(p, "f"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}
	keep := filepath.Join(tmp, "el-web-test-fresh")
	if err := os.MkdirAll(keep, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(keep, fresh, fresh); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(tmp, "keepme")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(other, old, old); err != nil {
		t.Fatal(err)
	}
	fs := testDiskFS(tmp, "", now)
	n, _ := cleanTempDirs(fs)
	if n != 3 {
		t.Fatalf("removed %d dirs, want 3", n)
	}
	for _, name := range []string{"keepme", "el-web-test-fresh"} {
		if _, err := os.Stat(filepath.Join(tmp, name)); err != nil {
			t.Fatalf("%s removed, want kept", name)
		}
	}
}

func TestDiskLowThresholds(t *testing.T) {
	t.Parallel()
	if warn, crit := diskLow(30<<30, 100<<30); warn || crit {
		t.Fatal("30% free must be neither warn nor crit")
	}
	if warn, crit := diskLow(15<<30, 100<<30); !warn || crit {
		t.Fatalf("15%% free: warn=%v crit=%v, want warn only", warn, crit)
	}
	// 10% of a 100GiB disk is 10GiB > 5GiB floor: crit below 10GiB.
	if _, crit := diskLow(9<<30, 100<<30); !crit {
		t.Fatal("9% of 100GiB must be critical")
	}
	// On a small disk the 5GiB floor wins over 10%: 4GiB free of 20GiB
	// is 20% free yet critical, while 6GiB free is fine.
	if _, crit := diskLow(4<<30, 20<<30); !crit {
		t.Fatal("4GiB free of 20GiB must be critical via the 5GiB floor")
	}
	if _, crit := diskLow(6<<30, 20<<30); crit {
		t.Fatal("6GiB free of 20GiB clears the 5GiB floor")
	}
}

// reapRepo builds a repo with one commit and returns it with a helper that
// adds a registered worktree whose directory mtime reads 25h old.
func reapRepo(t *testing.T) (repo string, add func(name string) string) {
	t.Helper()
	repo = tempDir(t)
	for _, args := range [][]string{{"init", "-b", "main", "."}, {"commit", "--allow-empty", "-m", "init"}} {
		if out, err := runGit(repo, args...); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	add = func(name string) string {
		p := filepath.Join(repo, ".el-worktrees", name)
		if out, err := runGit(repo, "worktree", "add", "-b", name, p); err != nil {
			t.Fatalf("worktree add: %v %s", err, out)
		}
		old := time.Now().Add(-25 * time.Hour)
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
		return p
	}
	return repo, add
}

func TestReapWorktreesCleanCandidateOnly(t *testing.T) {
	t.Parallel()
	repo, add := reapRepo(t)
	clean, dirty, stranger := add("clean"), add("dirty"), add("stranger")
	if err := os.WriteFile(filepath.Join(dirty, "wip.txt"), []byte("unsaved"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-25 * time.Hour)
	_ = os.Chtimes(dirty, old, old)
	fs := testDiskFS("", "", time.Now())
	// stranger is not named by any finished Run: it is never a candidate.
	n, _ := reapWorktrees(fs, []worktreeCand{{repo, clean}, {repo, dirty}}, nil)
	if n != 1 {
		t.Fatalf("reaped %d, want 1 (only the clean candidate)", n)
	}
	if _, err := os.Stat(clean); !os.IsNotExist(err) {
		t.Fatal("clean candidate survives")
	}
	if _, err := os.Stat(filepath.Join(dirty, "wip.txt")); err != nil {
		t.Fatal("worktree with uncommitted work was removed")
	}
	if _, err := os.Stat(stranger); err != nil {
		t.Fatal("a worktree no finished Run names was removed")
	}
}

func TestReapWorktreesSkipsYoungAndActiveRepo(t *testing.T) {
	t.Parallel()
	repo, add := reapRepo(t)
	old := add("old")
	young := add("young")
	now := time.Now()
	_ = os.Chtimes(young, now, now)
	fs := testDiskFS("", "", now)
	if n, _ := reapWorktrees(fs, []worktreeCand{{repo, old}}, func(string) bool { return true }); n != 0 {
		t.Fatalf("reaped %d under an active check, want 0", n)
	}
	if n, _ := reapWorktrees(fs, []worktreeCand{{repo, young}}, nil); n != 0 {
		t.Fatalf("reaped %d young worktrees, want 0", n)
	}
	if _, err := os.Stat(old); err != nil {
		t.Fatal("worktree under an active check was removed")
	}
}

func TestDiskEndpointAdminOnly(t *testing.T) {
	t.Parallel()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	h := d.Handler()
	admin := signedIn(t, h, "admin")
	rec := authDo(t, h, http.MethodGet, "/api/disk", "", admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin GET /api/disk = %d: %s", rec.Code, rec.Body.String())
	}
	var st diskStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	// Nobody signed in: 401 through the requester gate.
	rec = authDo(t, h, http.MethodGet, "/api/disk", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("anon GET /api/disk = %d, want 401", rec.Code)
	}
	// Non-admin User: 403.
	other := signedIn(t, h, "bob")
	rec = authDo(t, h, http.MethodGet, "/api/disk", "", other)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin GET /api/disk = %d, want 403", rec.Code)
	}
}

func TestHealthUnchanged(t *testing.T) {
	t.Parallel()
	h := NewDaemon(tempDir(t), NewFakeBackend()).Handler()
	admin := signedIn(t, h, "admin")
	rec := authDo(t, h, http.MethodGet, "/api/health", "", admin)
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != `{"ok":true}` {
		t.Fatalf("health = %d %q, want 200 {\"ok\":true}", rec.Code, rec.Body.String())
	}
}

func TestDiskCapSettingRoundTrip(t *testing.T) {
	settings := filepath.Join(tempDir(t), "settings.json")
	if err := os.WriteFile(settings, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EL_SETTINGS", settings)
	d := NewDaemon(tempDir(t), NewFakeBackend())
	h := d.Handler()
	admin := signedIn(t, h, "admin")
	rec := authDo(t, h, http.MethodGet, "/api/settings", "", admin)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"disk_go_cache_cap_gb":8`) {
		t.Fatalf("default settings = %d %s, want disk_go_cache_cap_gb 8", rec.Code, rec.Body.String())
	}
	rec = authDo(t, h, http.MethodPut, "/api/settings", `{"disk_go_cache_cap_gb":4}`, admin)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"disk_go_cache_cap_gb":4`) {
		t.Fatalf("PUT cap = %d %s, want 4", rec.Code, rec.Body.String())
	}
	rec = authDo(t, h, http.MethodPut, "/api/settings", `{"disk_go_cache_cap_gb":-1}`, admin)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("PUT negative cap = %d, want 400", rec.Code)
	}
	other := signedIn(t, h, "bob")
	rec = authDo(t, h, http.MethodPut, "/api/settings", `{"disk_go_cache_cap_gb":4}`, other)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin PUT cap = %d, want 403", rec.Code)
	}
}

func TestJanitorPassCleansTemp(t *testing.T) {
	t.Parallel()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	tmp := tempDir(t)
	old := time.Now().Add(-7 * time.Hour)
	p := filepath.Join(tmp, "el-web-test-stale")
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p, old, old); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	fs := testDiskFS(tmp, tempDir(t), now)
	res := d.runDiskJanitorOnce(fs)
	if res.TempDirs != 1 {
		t.Fatalf("janitor removed %d temp dirs, want 1: %+v", res.TempDirs, res)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("stale temp dir survives the janitor")
	}
}
