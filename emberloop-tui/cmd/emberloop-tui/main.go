// emberloop-tui opens the emberloop dashboard in the current terminal: specs grouped
// by repo with live run status and ticket progress, refreshed every second.
//
//	emberloop-tui [--state-dir D] [--repo R]
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"

	tui "example.com/emberloop-tui"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "emberloop-tui:", err)
		os.Exit(2)
	}
}

func run(argv []string) error {
	fs := flag.NewFlagSet("emberloop-tui", flag.ContinueOnError)
	stateDir := fs.String("state-dir", "", "emberloop state directory (default "+tui.DefaultStateDir()+")")
	repo := fs.String("repo", "", "current repo (default: the git top level)")
	if err := fs.Parse(argv); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("usage: emberloop-tui [--state-dir D] [--repo R]")
	}
	dir := *stateDir
	if dir == "" {
		dir = tui.DefaultStateDir()
	}
	current := *repo
	if current == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		current = tui.CurrentRepo(execRunner, cwd)
	}
	// One Reader across ticks: usage rows are read from the state database
	// past the row-id high-water mark, so the one-second poll only folds
	// what landed since the last tick.
	reader := new(tui.Reader)
	load := func() (tui.Snap, error) {
		return reader.Snapshot(dir, current, tui.HerdrPanes(execRunner))
	}
	model := tui.NewModel(load)
	model.SetSettingsBackend(tui.LiveBackend())
	model.SetJumpExec(execRunner)
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := p.Run()
	return err
}

// execRunner is the live CommandRunner: plain os/exec, stdout only.
func execRunner(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}
