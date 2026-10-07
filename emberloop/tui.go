package emberloop

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// `emberloop tui` opens the dashboard in the current pane. The dashboard lives
// in its own Go module (emberloop-tui, bubbletea) so this core stays
// dependency-free — decided in tui ticket 01. This launcher execs the
// installed emberloop-tui binary beside emberloop (the pair `emberloop install`
// writes), falling back to PATH, and never builds anything itself.

// TUIBinaryName is the dashboard binary RunTUI execs.
const TUIBinaryName = "emberloop-tui"

// tuiStarter execs the dashboard binary interactively, inheriting the
// terminal. A variable so tests start nothing.
var tuiStarter = func(path string, argv []string) error {
	c := exec.Command(path, argv...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return c.Run()
}

// FindTUIBinary locates the dashboard: the emberloop-tui beside the running
// emberloop first (the installed pair), then PATH. A missing binary fails
// naming install, which is the only way to get one.
func FindTUIBinary() (string, error) {
	if exe, err := os.Executable(); err == nil && exe != "" {
		beside := filepath.Join(filepath.Dir(exe), TUIBinaryName)
		if info, err := os.Stat(beside); err == nil && !info.IsDir() {
			return beside, nil
		}
	}
	if path, err := exec.LookPath(TUIBinaryName); err == nil {
		return path, nil
	}
	return "", fmt.Errorf("no %s found beside emberloop nor on PATH; run `emberloop install` to build it", TUIBinaryName)
}

// RunTUI serves `emberloop tui`: exec the dashboard binary in this pane. The
// dashboard resolves its own state dir and repo; no arguments are taken.
func RunTUI(argv []string) error {
	if len(argv) != 0 {
		return fmt.Errorf("usage: emberloop tui")
	}
	path, err := FindTUIBinary()
	if err != nil {
		return err
	}
	return tuiStarter(path, nil)
}
