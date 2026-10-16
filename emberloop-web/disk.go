package web

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

// Disk hygiene: the Daemon trims what parallel Runs leave behind (Go build
// cache, stale temp dirs, abandoned worktrees) and refuses new work when
// the disk is nearly full. Tunables live in the global settings file under
// disk_go_cache_cap_gb (default 8, GiB, 0 disables cache trimming).

const (
	defaultGoCacheCapGB = 8
	diskJanitorEvery    = 10 * time.Minute
	diskTempMaxAge      = 6 * time.Hour
	diskWorktreeMaxAge  = 24 * time.Hour
	cacheYoungGuard     = 10 * time.Minute
	diskWarnPct         = 20.0
	diskCritPct         = 10.0
	diskCritMinBytes    = 5 << 30
	diskWarnEvery       = 6 * time.Hour
)

// diskFS is the janitor's one seam: filesystem, clock and git behind fields
// so tests inject fakes without touching the host disk.
type diskFS struct {
	now     func() time.Time
	tmpDir  func() string
	cacheDr func() string
	remove  func(path string) error
	runGit  func(dir string, args ...string) (string, error)
	logf    func(format string, args ...any)
}

func defaultDiskFS() diskFS {
	return diskFS{
		now:     time.Now,
		tmpDir:  os.TempDir,
		cacheDr: goCacheDir,
		remove:  removeDiskPath,
		runGit:  runGit,
		logf:    log.Printf,
	}
}

func removeDiskPath(path string) error {
	_ = filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.Mode().Perm()&0o200 == 0 {
			_ = os.Chmod(p, info.Mode().Perm()|0o200)
		}
		return nil
	})
	return os.RemoveAll(path)
}

// goCacheDir reports the Go build cache: GOCACHE wins, then `go env
// GOCACHE`, then ~/.cache/go-build.
func goCacheDir() string {
	if v := strings.TrimSpace(os.Getenv("GOCACHE")); v != "" {
		return v
	}
	if out, err := exec.Command("go", "env", "GOCACHE").Output(); err == nil {
		if v := strings.TrimSpace(string(out)); v != "" {
			return v
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".cache", "go-build")
}

// diskGoCacheCapGB reads the global cap: unset means 8 GiB, 0 turns cache
// trimming off. Negative reads as off.
func diskGoCacheCapGB() int64 {
	all, err := readSettingsMap()
	if err != nil {
		return defaultGoCacheCapGB
	}
	raw, ok := all["disk_go_cache_cap_gb"]
	if !ok {
		return defaultGoCacheCapGB
	}
	var gb int64
	if err := json.Unmarshal(raw, &gb); err != nil {
		return defaultGoCacheCapGB
	}
	if gb < 0 {
		return 0
	}
	return gb
}

// diskFree reports free and total bytes of the filesystem holding path.
func diskFree(path string) (free, total uint64, err error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, 0, err
	}
	bsize := uint64(st.Bsize)
	return st.Bavail * bsize, st.Blocks * bsize, nil
}

func diskCritBytes(total uint64) uint64 {
	pct := uint64(float64(total) * diskCritPct / 100)
	if pct < diskCritMinBytes {
		return diskCritMinBytes
	}
	return pct
}

func diskLow(free, total uint64) (warn, crit bool) {
	if total == 0 {
		return false, false
	}
	freePct := float64(free) * 100 / float64(total)
	warn = freePct < diskWarnPct
	crit = free < diskCritBytes(total)
	return warn, crit
}

// dirBytes sums regular file sizes under root (best effort).
func dirBytes(root string) int64 {
	var n int64
	_ = filepath.Walk(root, func(_ string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		n += info.Size()
		return nil
	})
	return n
}

type cacheFile struct {
	path  string
	size  int64
	mtime time.Time
}

// goCacheSize sums the cache dir.
func goCacheSize(dir string) int64 {
	if dir == "" {
		return 0
	}
	return dirBytes(dir)
}

// pruneGoCache deletes cache files oldest-mtime-first until under 80% of
// the cap, never touching files younger than 10 minutes. It returns bytes
// freed and files removed.
func pruneGoCache(fs diskFS, dir string, capBytes int64) (freed int64, removed int) {
	if dir == "" || capBytes <= 0 {
		return 0, 0
	}
	var files []cacheFile
	_ = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		files = append(files, cacheFile{path: p, size: info.Size(), mtime: info.ModTime()})
		return nil
	})
	var total int64
	for _, f := range files {
		total += f.size
	}
	if total <= capBytes {
		return 0, 0
	}
	target := capBytes * 80 / 100
	sort.Slice(files, func(i, j int) bool { return files[i].mtime.Before(files[j].mtime) })
	now := fs.now()
	for _, f := range files {
		if total <= target {
			break
		}
		if now.Sub(f.mtime) < cacheYoungGuard {
			continue
		}
		if err := fs.remove(f.path); err == nil {
			total -= f.size
			freed += f.size
			removed++
		}
	}
	fs.logf("el-web: disk janitor trimmed go cache: freed %d bytes in %d files (%s)", freed, removed, dir)
	return freed, removed
}

var diskTempPrefixes = []string{"go-build", "el-web-test-", "el-test-"}

// cleanTempDirs deletes stale temp entries with the janitor's prefixes
// older than 6h. It returns dirs removed and bytes freed (sized before
// removal, best effort).
func cleanTempDirs(fs diskFS) (dirs int, freed int64) {
	tmp := fs.tmpDir()
	entries, err := os.ReadDir(tmp)
	if err != nil {
		return 0, 0
	}
	now := fs.now()
	for _, e := range entries {
		name := e.Name()
		matched := false
		for _, p := range diskTempPrefixes {
			if strings.HasPrefix(name, p) {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		full := filepath.Join(tmp, name)
		info, err := e.Info()
		if err != nil {
			continue
		}
		if now.Sub(info.ModTime()) < diskTempMaxAge {
			continue
		}
		freed += dirBytes(full)
		if err := fs.remove(full); err == nil {
			dirs++
			fs.logf("el-web: disk janitor removed stale temp dir %s", full)
		}
	}
	return dirs, freed
}

func (d *Daemon) cleanBrowserScreenshotDirs(fs diskFS) int {
	root := filepath.Join(d.stateDir, "agent-browser", "screenshots")
	entries, err := os.ReadDir(root)
	if err != nil {
		return 0
	}
	active := map[string]bool{}
	d.mu.Lock()
	for id, session := range d.sessions {
		if session.live {
			active[id] = true
		}
	}
	d.mu.Unlock()
	removed := 0
	for _, entry := range entries {
		if !entry.IsDir() || active[entry.Name()] {
			continue
		}
		info, err := entry.Info()
		if err != nil || fs.now().Sub(info.ModTime()) < diskTempMaxAge {
			continue
		}
		if fs.remove(filepath.Join(root, entry.Name())) == nil {
			removed++
		}
	}
	return removed
}

// janitorResult is the last janitor pass, served at GET /api/disk.
type janitorResult struct {
	At            time.Time `json:"at"`
	CacheFreed    int64     `json:"cacheFreed"`
	CacheRemoved  int       `json:"cacheRemoved"`
	CacheBytes    int64     `json:"cacheBytes"`
	TempDirs      int       `json:"tempDirs"`
	TempFreed     int64     `json:"tempFreed"`
	Worktrees     int       `json:"worktrees"`
	WorktreesFree int64     `json:"worktreesFreed"`
	BrowserDirs   int       `json:"browserDirs"`
	Error         string    `json:"error,omitempty"`
}

// diskStatus is the GET /api/disk shape.
type diskStatus struct {
	FreeBytes  uint64        `json:"freeBytes"`
	TotalBytes uint64        `json:"totalBytes"`
	CacheBytes int64         `json:"cacheBytes"`
	Low        bool          `json:"low"`
	Critical   bool          `json:"critical"`
	Last       janitorResult `json:"last"`
}

// worktreeCand is one worktree the janitor may reap: a path that Run state
// positively names as belonging to a finished Run, never a directory guessed
// from a listing.
type worktreeCand struct{ repo, path string }

// finishedWorktrees lists the worktrees of done or abandoned Runs that no
// live Session and no running Run still names.
func (d *Daemon) finishedWorktrees() []worktreeCand {
	d.mu.Lock()
	defer d.mu.Unlock()
	busy := map[string]bool{}
	for _, rs := range d.runs {
		if rs.Status == runStatusDone || rs.Status == runStatusAbandoned {
			continue
		}
		for _, wt := range rs.Worktrees {
			busy[wt] = true
		}
	}
	for _, s := range d.sessions {
		if s.live && s.meta.WorkDir != "" {
			busy[s.meta.WorkDir] = true
		}
	}
	var out []worktreeCand
	for _, rs := range d.runs {
		if rs.Status != runStatusDone && rs.Status != runStatusAbandoned {
			continue
		}
		for _, wt := range rs.Worktrees {
			if wt != "" && rs.Repo != "" && !busy[wt] {
				out = append(out, worktreeCand{repo: rs.Repo, path: wt})
			}
		}
	}
	return out
}

// reapWorktrees removes the candidate worktrees older than 24h with
// `git worktree remove` (never --force) and prunes. A worktree with
// uncommitted or untracked work, an unreadable status, or a repo with a
// driving Run is left alone: losing someone's unmerged work is worse than
// a full disk. checkActive reports a repo with a check executing.
func reapWorktrees(fs diskFS, cands []worktreeCand, checkActive func(repo string) bool) (n int, freed int64) {
	now := fs.now()
	pruned := map[string]bool{}
	for _, c := range cands {
		if checkActive != nil && checkActive(c.repo) {
			continue
		}
		info, err := os.Stat(c.path)
		if err != nil || !info.IsDir() {
			continue
		}
		if now.Sub(info.ModTime()) < diskWorktreeMaxAge {
			continue
		}
		out, err := fs.runGit(c.path, "status", "--porcelain")
		if err != nil || strings.TrimSpace(out) != "" {
			continue
		}
		size := dirBytes(c.path)
		if _, err := fs.runGit(c.repo, "worktree", "remove", c.path); err != nil {
			continue
		}
		freed += size
		n++
		pruned[c.repo] = true
		fs.logf("el-web: disk janitor reaped worktree %s", c.path)
	}
	for repo := range pruned {
		_, _ = fs.runGit(repo, "worktree", "prune")
	}
	return n, freed
}

// runDiskJanitorOnce runs one full pass: cache trim, temp sweep, worktree
// reap. It records the result for /api/disk and warns once per 6h below
// 20% free.
func (d *Daemon) runDiskJanitorOnce(fs diskFS) janitorResult {
	res := janitorResult{At: fs.now()}
	if capGB := diskGoCacheCapGB(); capGB > 0 {
		dir := fs.cacheDr()
		if f, n := pruneGoCache(fs, dir, capGB<<30); true {
			res.CacheFreed, res.CacheRemoved = f, n
		}
	}
	res.TempDirs, res.TempFreed = cleanTempDirs(fs)
	res.BrowserDirs = d.cleanBrowserScreenshotDirs(fs)
	n, freed := reapWorktrees(fs, d.finishedWorktrees(), d.worktreeCheckActive)
	res.Worktrees, res.WorktreesFree = n, freed
	res.CacheBytes = goCacheSize(fs.cacheDr())
	d.diskMu.Lock()
	d.diskLast = res
	d.diskMu.Unlock()
	if free, total, err := diskFree(d.stateDir); err == nil {
		if warn, _ := diskLow(free, total); warn {
			d.maybeWarnDiskLocked(fs, free, total)
		}
	}
	return res
}

// worktreeCheckActive reports a repo with a Worker/Reviewer check running:
// any driving Run on that repo. The janitor never reaps under a check.
func (d *Daemon) worktreeCheckActive(repo string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, rs := range d.runs {
		if rs.Repo == repo && rs.driving {
			return true
		}
	}
	return false
}

// maybeWarnDiskLocked publishes one low-disk warning per 6h through the
// push path and the live hub so the UI banner refetches /api/disk.
func (d *Daemon) maybeWarnDiskLocked(fs diskFS, free, total uint64) {
	now := fs.now()
	d.diskMu.Lock()
	since := now.Sub(d.diskWarnedAt)
	if since < diskWarnEvery {
		d.diskMu.Unlock()
		return
	}
	d.diskWarnedAt = now
	d.diskMu.Unlock()
	msg := fmt.Sprintf("disk low: %s free of %s", humanBytes(int64(free)), humanBytes(int64(total)))
	payload := pushPayload(pushMessage{Title: "Disk space low", Body: msg, Tag: "disk-low", Cue: "blocked", Key: "disk-low", Path: "/settings"})
	go d.push.notify(payload)
	d.live.publish("disk")
	fs.logf("el-web: %s", msg)
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit && exp < 3; v /= unit {
		div *= unit
		exp++
	}
	units := []string{"KiB", "MiB", "GiB", "TiB"}
	return fmt.Sprintf("%.1f %s", float64(n)/float64(div), units[exp])
}

// diskBlocked reports whether the disk refuses new Runs/Tickets/worktrees:
// free below 10% or 5 GiB, whichever is larger.
func (d *Daemon) diskBlocked() (bool, string) {
	free, total, err := diskFree(d.stateDir)
	if err != nil {
		return false, ""
	}
	_, crit := diskLow(free, total)
	if !crit {
		return false, ""
	}
	d.diskMu.Lock()
	cache := d.diskLast.CacheBytes
	d.diskMu.Unlock()
	msg := fmt.Sprintf("disk critically low: %s free of %s; the janitor can free ~%s from the Go build cache — free space or lower disk_go_cache_cap_gb, then retry",
		humanBytes(int64(free)), humanBytes(int64(total)), humanBytes(cache))
	return true, msg
}

// StartDiskJanitor runs the 10-minute pass until stop closes.
// The binary starts it beside the nightly loop; tests call
// runDiskJanitorOnce directly with a fake diskFS.
func (d *Daemon) StartDiskJanitor(stop <-chan struct{}) {
	fs := defaultDiskFS()
	t := time.NewTicker(diskJanitorEvery)
	defer t.Stop()
	// One pass soon after boot, so a disk that filled while the Daemon was
	// down is trimmed without waiting a full interval.
	first := time.After(30 * time.Second)
	for {
		select {
		case <-stop:
			return
		case <-first:
			d.runDiskJanitorOnce(fs)
		case <-t.C:
			d.runDiskJanitorOnce(fs)
		}
	}
}

// handleDisk serves GET /api/disk: free/total bytes, cache size and the
// last janitor result. Admin only; health stays the plain {"ok":true}.
func (d *Daemon) handleDisk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	who, ok := d.requesterOf(r)
	if !ok || who.user == nil || !who.user.Admin {
		http.Error(w, "only the admin reads disk status", http.StatusForbidden)
		return
	}
	free, total, _ := diskFree(d.stateDir)
	warn, crit := diskLow(free, total)
	d.diskMu.Lock()
	last := d.diskLast
	d.diskMu.Unlock()
	cache := last.CacheBytes
	writeJSON(w, http.StatusOK, diskStatus{FreeBytes: free, TotalBytes: total, CacheBytes: cache, Low: warn || crit, Critical: crit, Last: last})
}
