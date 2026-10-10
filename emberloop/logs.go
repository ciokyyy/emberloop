package el

import (
	goflag "flag"
	"fmt"
	"io"
	"os"
	"os/exec"
)

const daemonUnit = "emberloop-web"

// RunLogs implements `el logs`: it prints the prod Daemon's journal
// (systemd user unit emberloop-web). The dev Daemon runs in a foreground
// pane and has no journal; read that pane instead.
func RunLogs(argv []string, out io.Writer) error {
	args, err := logsArgs(argv)
	if err != nil {
		return err
	}
	cmd := exec.Command("journalctl", args...)
	cmd.Stdout = out
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("el logs: journalctl: %w", err)
	}
	return nil
}

// logsArgs turns `el logs` flags into journalctl arguments.
func logsArgs(argv []string) ([]string, error) {
	const usage = "usage: el logs [-n N] [-f] [--since TIME] [--grep PATTERN]"
	fs := goflag.NewFlagSet("el logs", goflag.ContinueOnError)
	fs.SetOutput(io.Discard)
	n := fs.Int("n", 50, "number of recent lines")
	follow := fs.Bool("f", false, "follow new lines")
	since := fs.String("since", "", "only entries since TIME (journalctl syntax, e.g. 10m ago, 12:00)")
	grep := fs.String("grep", "", "only entries matching PATTERN")
	if err := fs.Parse(argv); err != nil || fs.NArg() != 0 || *n < 0 {
		return nil, fmt.Errorf(usage)
	}
	args := []string{"--user", "-u", daemonUnit, "--no-pager", "-n", fmt.Sprint(*n)}
	if *follow {
		args = append(args, "-f")
	}
	if *since != "" {
		args = append(args, "--since", *since)
	}
	if *grep != "" {
		args = append(args, "--grep", *grep)
	}
	return args, nil
}
