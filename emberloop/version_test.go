package el

import (
	"strings"
	"testing"
)

// An unstamped build says dev: the test binary carries no ldflags stamps,
// so the version falls back to the placeholder, never a blank.
func TestBuildVersionDefaultsToDev(t *testing.T) {
	old := Version
	Version = ""
	defer func() { Version = old }()
	if got := BuildVersion(); got != "dev" {
		t.Errorf("unstamped BuildVersion = %q, want dev", got)
	}
}

// A stamped version always wins over the toolchain's record.
func TestBuildVersionPrefersTheStamp(t *testing.T) {
	old := Version
	Version = "v1.2.3"
	defer func() { Version = old }()
	if got := BuildVersion(); got != "v1.2.3" {
		t.Errorf("stamped BuildVersion = %q, want v1.2.3", got)
	}
}

// The commit is never blank: stamped, the checkout's own revision, or the
// plain word unknown.
func TestBuildCommitIsNeverBlank(t *testing.T) {
	old := Commit
	Commit = ""
	defer func() { Commit = old }()
	if got := BuildCommit(); got == "" {
		t.Error("unstamped BuildCommit must not be blank")
	}
}

// A stamped commit wins, cut to the short characters a person reads.
func TestBuildCommitPrefersTheStampShort(t *testing.T) {
	old := Commit
	defer func() { Commit = old }()
	Commit = "abc1234def5678"
	if got := BuildCommit(); got != "abc1234" {
		t.Errorf("stamped BuildCommit = %q, want abc1234", got)
	}
	Commit = "unknown"
	if got := BuildCommit(); got != "unknown" {
		t.Errorf("a plain word must stay whole, got %q", got)
	}
}

// `el --version` prints one quotable line with no blanks.
func TestVersionStringHasNoBlanks(t *testing.T) {
	s := VersionString()
	if !strings.HasPrefix(s, "el ") || !strings.Contains(s, "(") || !strings.Contains(s, ")") {
		t.Errorf("VersionString = %q, want 'el <version> (<commit>)'", s)
	}
	if strings.Contains(s, "()") || strings.Contains(s, "el  ") {
		t.Errorf("VersionString must carry no blank field, got %q", s)
	}
}
