// wick-pig starts an agent session: pig or claude, with wick's tools over MCP.
//
//	wick-pig settings [decision-model <id>|threshold <n>|models]  read and save settings
//	wick-pig skills install                                    fill the skill library
//	wick-pig [--pig|--claude] [--model M] [-p]                 full session
//	wick-pig implement <path> [flags]                          restricted merger
//	wick-pig <wayfinder|grill|research|...> [flags] [input...] one route skill
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"example.com/wick"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "wick-pig:", err)
		os.Exit(2)
	}
}

func run(argv []string) error {
	sub := ""
	if len(argv) > 0 && !strings.HasPrefix(argv[0], "-") {
		sub, argv = argv[0], argv[1:]
	}
	if sub == "settings" {
		return runSettings(argv, os.Stdout, listModels)
	}
	if sub == "retry" {
		if len(argv) != 1 {
			return fmt.Errorf("usage: wick-pig retry <worker-id|pane-id>")
		}
		return wick.Retry(wick.OSExec, argv[0])
	}
	if sub == "skills" {
		if len(argv) != 1 || argv[0] != "install" {
			return fmt.Errorf("usage: wick-pig skills install")
		}
		if err := wick.InstallSkills(wick.OSExec); err != nil {
			return err
		}
		fmt.Printf("installed %s into %s\n", "mattpocock/skills", wick.SkillLibraryDir())
		return nil
	}
	fs := flag.NewFlagSet("wick-pig", flag.ContinueOnError)
	harness := fs.String("harness", "pig", "pig or claude")
	pig := fs.Bool("pig", false, "use pig")
	claude := fs.Bool("claude", false, "use claude")
	model := fs.String("model", "", "model override")
	headless := fs.Bool("p", false, "print one answer and exit")
	if err := fs.Parse(argv); err != nil {
		return err
	}
	if *pig {
		*harness = "pig"
	}
	if *claude {
		*harness = "claude"
	}
	rest := strings.Join(fs.Args(), " ")

	cwd, _ := os.Getwd()
	s := wick.Session{ID: wick.NewSessionID(), Model: *model, Headless: *headless, Cwd: cwd}
	switch sub {
	case "":
		s.Role, s.Prompt, s.Skills = "full", rest, nil
	case "implement":
		if rest == "" {
			return fmt.Errorf("usage: wick-pig implement <ticket-dir-or-file>")
		}
		target, err := filepath.Abs(rest)
		if err != nil {
			return err
		}
		s.Role, s.Prompt, s.Skills = "implement", wick.ImplementPrompt(target), wick.RouteSkills()
	default:
		found := false
		for _, e := range wick.SkillEntries() {
			if e.Name == sub {
				s.Role, s.Prompt, s.Skills = "plan", wick.SkillPrompt(e.Skill, rest, e.Scout), wick.RouteSkills()
				found = true
			}
		}
		if !found {
			return fmt.Errorf("unknown command %q", sub)
		}
	}

	launch, err := wick.PrepareLaunch(*harness, s)
	if err != nil {
		return err
	}
	cmd, err := wick.BuildCommand(*harness, withoutPrompt(s, launch))
	if err != nil {
		return err
	}
	// The agent reads its MCP config at startup, so the file is dead weight
	// once it exits.
	defer wick.CleanupSession(s)
	c := exec.Command(cmd.Bin, cmd.Args...)
	c.Env = append(os.Environ(), cmd.Env...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	if launch.Late != "" {
		pane := os.Getenv("HERDR_PANE_ID")
		if pane == "" {
			return fmt.Errorf("%s needs herdr to deliver its first message; run inside herdr", *harness)
		}
		go func() {
			if err := wick.DeliverLate(wick.OSExec, pane, launch.Late); err != nil {
				fmt.Fprintln(os.Stderr, "wick-pig:", err)
			}
		}()
	}
	if pane := os.Getenv("HERDR_PANE_ID"); pane != "" && !*headless {
		stop := make(chan struct{})
		defer close(stop)
		if h := wick.Registered(*harness); h != nil {
			go wick.Supervise(wick.OSExec, h, s, pane, stop)
		}
	}
	return c.Run()
}

// withoutPrompt drops the prompt from s when the harness takes it late.
func withoutPrompt(s wick.Session, l wick.Launch) wick.Session {
	if l.Late != "" {
		s.Prompt = ""
	}
	return s
}
