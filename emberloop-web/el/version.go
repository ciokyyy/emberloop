package el

import (
	"runtime/debug"
	"strings"
)

// Version and Commit name the running build. A release stamps them at link
// time; anything built by hand says so plainly and borrows what the
// toolchain recorded, because a blank in a bug report is worse than "dev".
//
//	go build -ldflags "-X example.com/emberloop-web/el.Version=v1.2.3 -X example.com/emberloop-web/el.Commit=abc1234" ./cmd/el
var (
	Version = "dev"
	Commit  = ""
)

// BuildVersion is the stamped version, the module version the toolchain
// recorded, or "dev" for an unstamped build. "(devel)" is the toolchain's
// own word for unstamped, so it never surfaces.
func BuildVersion() string {
	return firstNonEmpty(Version, moduleBuildVersion(), "dev")
}

// BuildCommit is the stamped commit (short), the checkout's own revision
// short, or "unknown" when neither exists.
func BuildCommit() string {
	return shortBuildCommit(firstNonEmpty(Commit, vcsBuildRevision(), "unknown"))
}

// VersionString is the one line `el --version` prints and the one an
// operator quotes in a bug report: el dev (abc1234).
func VersionString() string {
	return "el " + BuildVersion() + " (" + BuildCommit() + ")"
}

func moduleBuildVersion() string {
	bi, _ := debug.ReadBuildInfo()
	if bi == nil || bi.Main.Version == "" || bi.Main.Version == "(devel)" {
		return ""
	}
	return bi.Main.Version
}

func vcsBuildRevision() string {
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

// shortBuildCommit cuts a full sha to the characters a person reads, and
// leaves anything else whole: cutting "unknown" would turn a plain word
// into a plausible-looking sha.
func shortBuildCommit(commit string) string {
	if len(commit) <= 7 || strings.IndexFunc(commit, notHexRune) >= 0 {
		return commit
	}
	return commit[:7]
}

func notHexRune(r rune) bool { return !strings.ContainsRune("0123456789abcdefABCDEF", r) }

func firstNonEmpty(candidates ...string) string {
	for _, candidate := range candidates {
		if candidate != "" {
			return candidate
		}
	}
	return ""
}
