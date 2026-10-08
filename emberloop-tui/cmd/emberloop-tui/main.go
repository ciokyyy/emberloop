// emberloop-tui opens the emberloop dashboard in the current terminal: specs grouped
// by repo with live run status and ticket progress, refreshed every second.
//
//	emberloop-tui [--state-dir D] [--repo R]
//	emberloop-tui --driver-run <spec> [--repo R] [--state-dir D]
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	emberloop "example.com/emberloop"
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
	driverRun := fs.String("driver-run", "", "run a spec through the deterministic driver with the live ticket table")
	if err := fs.Parse(argv); err != nil {
		return err
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
	if *driverRun != "" {
		if fs.NArg() != 0 {
			return fmt.Errorf("usage: emberloop-tui --driver-run <spec> [--repo R] [--state-dir D]")
		}
		return runDriverPane(*driverRun, current, dir)
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("usage: emberloop-tui [--state-dir D] [--repo R]")
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

// runDriverPane runs a spec through the deterministic driver with the live
// ticket table: the loop runs in a goroutine against the shared run state
// and the pane reads snapshots of it every second. Quitting the pane stops
// the loop with it, so a quit can land mid-pass; the workers keep running in
// herdr and starting again resumes the run from the tickets, worker records
// and git. The loop's final report prints on exit only when the loop finished
// first.
func runDriverPane(specArg, repo, stateDir string) error {
	spec, err := emberloop.ResolveSpec(specArg)
	if err != nil {
		return err
	}
	files, err := emberloop.ReadDriverFiles(spec)
	if err != nil {
		return err
	}
	run := emberloop.NewDriverRun(nil)
	world := emberloop.LiveDriverWorld(spec, repo, files)
	world.Run = run
	world.Sleep = time.Sleep
	reader := new(tui.Reader)
	load := func() ([]tui.RunTicket, error) {
		snap, err := reader.Snapshot(stateDir, repo, tui.HerdrPanes(execRunner))
		if err != nil {
			return nil, err
		}
		return tui.RunLiveRows(run.Snapshot(), snap), nil
	}
	model := tui.NewRunModel(load)
	model.SetJumpExec(execRunner)
	model.SetPark(func(path string) error { return emberloop.ParkDriverTicketFile(path) })
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	done := make(chan tui.RunDoneMsg, 1)
	go func() {
		res, err := emberloop.RunDriver(spec, world)
		msg := tui.RunDoneMsg{Err: err}
		if err == nil {
			msg.Report = res.Report
		}
		done <- msg
		p.Send(msg)
	}()
	if _, err := p.Run(); err != nil {
		return err
	}
	select {
	case msg := <-done:
		if msg.Err != nil {
			return msg.Err
		}
		fmt.Fprint(os.Stdout, msg.Report)
		return nil
	default:
		fmt.Fprintln(os.Stderr, "emberloop: run pane closed; loop stopped, workers keep running, start again to resume")
		return nil
	}
}

// execRunner is the live CommandRunner: plain os/exec, stdout only.
func execRunner(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}
