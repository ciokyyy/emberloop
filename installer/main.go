// The el installer: a small TUI that picks the harnesses, builds the
// binaries, and writes every config el needs.
//
//	install.sh [--yes] [--pig=false] [--claude=false] [--graph=false]
//	           [--bin-dir DIR] [--dry-run]
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "installer:", err)
		os.Exit(1)
	}
}

func run() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	repo := flag.String("repo", "", "repo root (set by install.sh)")
	binDir := flag.String("bin-dir", filepath.Join(home, ".local", "bin"), "where the binaries go")
	yes := flag.Bool("yes", false, "skip the TUI and install the defaults")
	dry := flag.Bool("dry-run", false, "print what would happen, change nothing")
	pig := flag.Bool("pig", true, "install the pig parts")
	claude := flag.Bool("claude", true, "install the claude parts")
	graph := flag.Bool("graph", true, "enable the code graph MCP")
	flag.Parse()
	if *repo == "" {
		wd, _ := os.Getwd()
		*repo = filepath.Dir(wd)
	}
	if _, err := os.Stat(filepath.Join(*repo, "emberloop-web", "go.mod")); err != nil {
		return fmt.Errorf("%s is not the el repo root", *repo)
	}
	env := &Env{Repo: *repo, Home: home, BinDir: *binDir, DryRun: *dry}
	choices := Choices{Pig: *pig, Claude: *claude, Graph: *graph}
	env.Choices = choices

	if *yes || !term.IsTerminal(int(os.Stdin.Fd())) {
		if err := runPlain(env); err != nil {
			return err
		}
	} else {
		m := newModel(env, choices)
		if _, err := tea.NewProgram(m).Run(); err != nil {
			return err
		}
		if m.quit || m.failed {
			if m.failed {
				return fmt.Errorf("install failed")
			}
			return nil
		}
	}
	return finish(env)
}

// runPlain runs every step without the TUI and prints one line per step.
func runPlain(env *Env) error {
	for _, s := range Steps(env.Choices) {
		detail, err := s.Run(env)
		switch {
		case err == nil:
			fmt.Printf("ok    %s: %s\n", s.Name, detail)
		default:
			if w, ok := err.(warning); ok {
				fmt.Printf("warn  %s: %s\n", s.Name, w.msg)
				continue
			}
			return fmt.Errorf("%s: %w", s.Name, err)
		}
	}
	return nil
}

// finish runs the commands that may need a password prompt on the plain
// terminal, then prints the next steps.
func finish(env *Env) error {
	for _, argv := range FinalCommands(env) {
		fmt.Println("\n$", argv[0], argv[1:])
		if env.DryRun {
			continue
		}
		cmd := exec.Command(argv[0], argv[1:]...)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		cmd.Env = append(os.Environ(), "PATH="+env.pathWithBin()+string(os.PathListSeparator)+filepath.Join(env.Home, ".bun", "bin"))
		if err := cmd.Run(); err != nil {
			fmt.Printf("warning: %v (rerun it by hand)\n", err)
		}
	}
	fmt.Printf("\nDone. Make sure %s is on your PATH, then run `el`.\n", env.BinDir)
	return nil
}
