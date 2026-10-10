package web

import (
	"net/http"
	"runtime/debug"
	"strings"
)

// Version, Commit and UIHash name the running Daemon build. A release
// stamps them at link time; anything built by hand borrows what the
// toolchain recorded for the commit and serves an empty UI hash, which the
// UI reads as "unknown" and never raises the update pill for.
//
//	go build -ldflags "-X example.com/emberloop-web.Version=v1.2.3 -X example.com/emberloop-web.Commit=abc1234 -X example.com/emberloop-web.UIHash=0123456789ab" ./cmd/emberloop-web
//
// UIHash is the short hash of the web source the embedded UI was built
// from, stamped by `el update` from the same hash the UI bundle carries as
// a build-time constant: equal hashes mean the browser runs the build the
// Daemon serves.
var (
	Version = "dev"
	Commit  = ""
	UIHash  = ""
)

// VersionInfo is what GET /api/version answers: which Daemon serves, and
// which UI build it embeds.
type VersionInfo struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	UIHash  string `json:"uiHash"`
}

// Current reports this Daemon's build identity, falling back to the
// toolchain's own record for whatever the linker left unstamped.
func Current() VersionInfo {
	return VersionInfo{
		Version: firstNonEmpty(Version, moduleVersion(), "dev"),
		Commit:  shortCommit(firstNonEmpty(Commit, vcsRevision(), "unknown")),
		UIHash:  UIHash,
	}
}

// VersionString is the one line `el-web --version` prints: the Daemon's own
// stamp plus the UI build it embeds.
func VersionString() string {
	c := Current()
	return "el-web " + c.Version + " (" + c.Commit + ", ui " + c.UIHash + ")"
}

// handleVersion answers which Daemon serves, without a login. It reads no
// state at all: a build that will not open its database is exactly the
// build whose operator asks this.
func (d *Daemon) handleVersion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, Current())
}

// handleHealth is the update gate's probe: 200 when the Daemon serves.
// `el update` boots the new binary on a throwaway port and rolls the
// install back when this never answers.
func (d *Daemon) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// moduleVersion is the module version the toolchain recorded, or nothing.
// "(devel)" is the toolchain's own word for "unstamped", so it never
// surfaces: the UI would read it as a stranger.
func moduleVersion() string {
	bi, _ := debug.ReadBuildInfo()
	if bi == nil || bi.Main.Version == "" || bi.Main.Version == "(devel)" {
		return ""
	}
	return bi.Main.Version
}

// vcsRevision is the commit the toolchain stamped for the checkout this was
// built from, which is what makes an unstamped dev build identifiable.
func vcsRevision() string {
	bi, _ := debug.ReadBuildInfo()
	if bi == nil {
		return ""
	}
	for _, setting := range bi.Settings {
		if setting.Key == "vcs.revision" && setting.Value != "" {
			return setting.Value
		}
	}
	return ""
}

// shortCommit cuts a full sha to the characters a person reads, and leaves
// anything else whole: cutting "unknown" would turn a plain word into a
// plausible-looking sha.
func shortCommit(commit string) string {
	if len(commit) <= 7 || strings.IndexFunc(commit, notHex) >= 0 {
		return commit
	}
	return commit[:7]
}

func notHex(r rune) bool { return !strings.ContainsRune("0123456789abcdefABCDEF", r) }

func firstNonEmpty(candidates ...string) string {
	for _, candidate := range candidates {
		if candidate != "" {
			return candidate
		}
	}
	return ""
}
