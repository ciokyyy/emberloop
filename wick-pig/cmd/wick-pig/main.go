// wick-pig starts an agent session: pig or claude, with wick's tools over MCP.
//
//	wick-pig settings [decision-model <id>|threshold <n>|max-workers <n>]  read and save settings
//	wick-pig settings models [--refresh] [--harness h]    list a harness's models
//	wick-pig settings role <slot> harness|model|effort <value>   set a launch slot
//	wick-pig settings tool <name> ... | mcp-server <name> ...    tune tools and MCP servers
//	wick-pig stop-gate                                         claude Stop hook gate
//	wick-pig pig-gate <event-json>                             pig extension stop gate
//	wick-pig exec <launch-file>                                run a launch file the pane typed
//	wick-pig skills install                                    fill the skill library
//	wick-pig install [--force]                               rebuild and install the wick binaries (refuses while agents are live)
//	wick-pig deps                                             check and install external dependencies
//	wick-pig stats prune [--run <slug>] [--older-than <age>] [--apply]
//	                                                         prune archived run data (dry run by default)
//	wick-pig tui                                            open the dashboard in the current pane
//	wick-pig [--pig|--claude] [--role full] [--model M] [--effort L] [-p] plan session
//	wick-pig implement <spec> [flags]                          run a spec on spec/<slug>
//	wick-pig <wayfinder|grill|research|...> [flags] [input...] one route skill
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
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
	if sub == "exec" {
		if len(argv) != 1 {
			return fmt.Errorf("usage: wick-pig exec <launch-file>")
		}
		return wick.RunExecFile(argv[0])
	}
	if sub == "stop-gate" {
		return wick.RunStopGate(os.Stdin, os.Stdout)
	}
	if sub == "pig-gate" {
		if len(argv) != 1 {
			return fmt.Errorf("usage: wick-pig pig-gate <event-json>")
		}
		return wick.RunPigGate(argv[0], os.Stdout)
	}
	if sub == "settings" {
		return runSettings(argv, os.Stdout, cachedModels)
	}
	if sub == "retry" {
		if len(argv) != 1 {
			return fmt.Errorf("usage: wick-pig retry <worker-id|pane-id>")
		}
		return wick.Retry(wick.OSExec, argv[0])
	}
	if sub == "stats" {
		return wick.RunStats(argv, os.Stdout)
	}
	if sub == "tui" {
		return wick.RunTUI(argv)
	}
	if sub == "deps" {
		return wick.RunDeps(argv, os.Stdout)
	}
	if sub == "install" {
		return wick.RunInstall(argv, os.Stdout)
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
	harness := fs.String("harness", "", "pig or claude (default: the orchestrator slot)")
	pig := fs.Bool("pig", false, "use pig")
	claude := fs.Bool("claude", false, "use claude")
	model := fs.String("model", "", "model override")
	effort := fs.String("effort", "", "reasoning effort for the harness")
	role := fs.String("role", "", "session role; default plan (use full for every tool)")
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
	id := wick.NewSessionID()
	s := wick.Session{ID: id, AgentID: id, Headless: *headless, Cwd: cwd}
	var implSpec *wick.SpecRun
	switch sub {
	case "":
		r, err := sessionRole(sub, *role)
		if err != nil {
			return err
		}
		s.Role, s.Prompt = r, rest
	case "implement":
		if rest == "" {
			return fmt.Errorf("usage: wick-pig implement <spec-file-or-dir>")
		}
		// Workers' results reach their parent as notices delivered into its
		// herdr pane, so an implement session without a pane could never hear
		// them. Refuse before anything is created or spawned.
		if os.Getenv("HERDR_PANE_ID") == "" {
			return fmt.Errorf("implement must run in a herdr pane so its workers' results can reach it; run it inside herdr")
		}
		if *headless {
			return fmt.Errorf("implement must run in a herdr pane; -p is not supported")
		}
		spec, err := wick.ResolveSpec(rest)
		if err != nil {
			return err
		}
		r, err := sessionRole(sub, *role)
		if err != nil {
			return err
		}
		created, err := wick.EnterIntegrationBranch(wick.OSExec, cwd, spec.Branch)
		if err != nil {
			return err
		}
		verb := "resuming on"
		if created {
			verb = "created"
		}
		fmt.Fprintf(os.Stderr, "wick-pig: %s %s\n", verb, spec.Branch)
		s.Role, s.Prompt, s.Spec = r, wick.ImplementPrompt(spec), spec.Spec
		implSpec = &spec
	default:
		found := false
		for _, e := range wick.SkillEntries() {
			if e.Name == sub {
				r, err := sessionRole(sub, *role)
				if err != nil {
					return err
				}
				s.Role, s.Prompt = r, wick.SkillPrompt(e.Skill, rest, e.Scout)
				found = true
			}
		}
		if !found {
			return fmt.Errorf("unknown command %q", sub)
		}
	}
	choice := topLevelChoice(wick.Choice{Harness: *harness, Model: *model, Effort: *effort})
	s.Model, s.Effort = choice.Model, choice.Effort
	// An implement run leaves a trace: one record per spec slug, updated on
	// re-run and stamped at exit. Readers decide "running" from the pane.
	runSlug := ""
	if implSpec != nil {
		rec, err := wick.RecordRun(wick.OSExec, *implSpec, cwd, s.AgentID, choice.Harness, choice.Model, choice.Effort)
		if err != nil {
			return err
		}
		runSlug = rec.Slug
	}
	launch, err := wick.PrepareLaunch(choice.Harness, s)
	if err != nil {
		return err
	}
	cmd, err := wick.BuildCommand(choice.Harness, withoutPrompt(s, launch))
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
			return fmt.Errorf("%s needs herdr to deliver its first message; run inside herdr", choice.Harness)
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
		if h := wick.Registered(choice.Harness); h != nil {
			go wick.Supervise(wick.OSExec, h, s, pane, stop)
		}
	}
	err = c.Run()
	if runSlug != "" {
		_ = wick.FinishRun(runSlug)
	}
	return err
}

// withoutPrompt drops the prompt from s when the harness takes it late.
func withoutPrompt(s wick.Session, l wick.Launch) wick.Session {
	if l.Late != "" {
		s.Prompt = ""
	}
	return s
}

// sessionRole is the role a launch gets. A bare session plans; the implement
// launcher starts an implement session; a route subcommand plans too. The role
// flag picks the bare session's role, which is how `full` -- every tool --
// stays reachable. A flag beside a subcommand is refused: the subcommand's
// prompt is written for its own role, and a role that cannot run the prompt
// only yields a broken session.
func sessionRole(sub, flagValue string) (string, error) {
	if flagValue != "" {
		if sub != "" {
			return "", fmt.Errorf("--role applies to a bare session; %q already picks its role", sub)
		}
		if _, err := wick.RoleTools(flagValue); err != nil {
			return "", err
		}
		return flagValue, nil
	}
	if sub == "implement" {
		return "implement", nil
	}
	return "plan", nil
}

// topLevelChoice resolves a top-level session's harness, model and effort
// through the orchestrator slot: the flags win, then the slot, then the
// built-in default. A claude session with no model of its own runs sonnet.
func topLevelChoice(flags wick.Choice) wick.Choice {
	c := wick.Resolve("orchestrator", flags, wick.Choice{})
	if c.Harness == "claude" && c.Model == "" {
		c.Model = "sonnet"
	}
	return c
}
