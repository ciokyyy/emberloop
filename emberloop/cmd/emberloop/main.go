// emberloop starts an agent session: pig or claude, with emberloop's tools over MCP.
//
//	emberloop settings [decision-model <id>|threshold <n>|max-workers <n>]  read and save settings
//	emberloop settings models [--refresh] [--harness h]    list a harness's models
//	emberloop settings role <slot> harness|model|effort <value>   set a launch slot
//	emberloop settings tool <name> ... | mcp-server <name> ...    tune tools and MCP servers
//	emberloop stop-gate                                         claude Stop hook gate
//	emberloop pig-gate <event-json>                             pig extension stop gate
//	emberloop claude-status                                     claude statusline command: stores the 5h and weekly usage
//	emberloop voice-reminder [hook]                             per-turn register reminder (pig extension, claude hook)
//	emberloop exec <launch-file>                                run a launch file the pane typed
//	emberloop wrap --harness h -- <bin> [args...]              run a command under the header strip
//	emberloop skills install                                    fill the skill library
//	emberloop install [--force]                               rebuild and install the emberloop binaries (refuses while agents are live)
//	emberloop deps                                             check and install external dependencies
//	emberloop context [--harness h] [--dir d] <session-id>   print one session’s context numbers
//	emberloop compress [--force] <pane>                   summarize the worker in pane and restart it there
//	emberloop request-compress <pane>                      the herdr compress key's signal for the focused pane
//	emberloop stats prune [--run <slug>] [--older-than <age>] [--apply]
//	                                                         prune archived run data (dry run by default)
//	emberloop tui                                            open the dashboard in the current pane
//	emberloop [--pig|--claude] [--role full] [--model M] [--effort L] [-p] plan session
//	emberloop implement [--driver [--check]|--llm] <spec>  run a spec on spec/<slug>
//	emberloop <wayfinder|grill|research|...> [flags] [input...] one route skill
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"example.com/emberloop"
	"example.com/emberloop/tools"
	"golang.org/x/term"
)

// driverPaneTTY reports whether the run pane opens: a terminal on stdout
// gets the live ticket table, a pipe keeps the plain report (probes and
// scripts). A variable so tests drive both legs.
var driverPaneTTY = func() bool { return term.IsTerminal(int(os.Stdout.Fd())) }

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "emberloop:", err)
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
			return fmt.Errorf("usage: emberloop exec <launch-file>")
		}
		return emberloop.RunExecFile(argv[0])
	}
	if sub == "wrap" {
		return runWrap(argv, os.Stdin, os.Stdout)
	}
	if sub == "stop-gate" {
		return emberloop.RunStopGate(os.Stdin, os.Stdout)
	}
	if sub == "claude-status" {
		return emberloop.RunClaudeStatus(os.Stdin, "")
	}
	if sub == "voice-reminder" {
		return emberloop.RunVoiceReminder(argv, os.Stdout)
	}
	if sub == "pig-gate" {
		if len(argv) != 1 {
			return fmt.Errorf("usage: emberloop pig-gate <event-json>")
		}
		return emberloop.RunPigGate(argv[0], os.Stdout)
	}
	if sub == "settings" {
		return runSettings(argv, os.Stdout, cachedModels)
	}
	if sub == "context" {
		return runContext(argv, os.Stdout)
	}
	if sub == "compress" {
		return runCompress(argv, os.Stdout)
	}
	if sub == "request-compress" {
		return runRequestCompress(argv, os.Stdout)
	}
	if sub == "retry" {
		if len(argv) != 1 {
			return fmt.Errorf("usage: emberloop retry <worker-id|pane-id>")
		}
		return emberloop.Retry(emberloop.OSExec, argv[0])
	}
	if sub == "stats" {
		return emberloop.RunStats(argv, os.Stdout)
	}
	if sub == "tui" {
		return emberloop.RunTUI(argv)
	}
	if sub == "deps" {
		return emberloop.RunDeps(argv, os.Stdout)
	}
	if sub == "install" {
		return emberloop.RunInstall(argv, os.Stdout)
	}
	if sub == "skills" {
		if len(argv) != 1 || argv[0] != "install" {
			return fmt.Errorf("usage: emberloop skills install")
		}
		if err := emberloop.InstallSkills(emberloop.OSExec); err != nil {
			return err
		}
		fmt.Printf("installed %s into %s\n", "mattpocock/skills", emberloop.SkillLibraryDir())
		return nil
	}
	fs := flag.NewFlagSet("emberloop", flag.ContinueOnError)
	harness := fs.String("harness", "", "pig or claude (default: the orchestrator slot)")
	pig := fs.Bool("pig", false, "use pig")
	claude := fs.Bool("claude", false, "use claude")
	model := fs.String("model", "", "model override")
	effort := fs.String("effort", "", "reasoning effort for the harness")
	role := fs.String("role", "", "session role; default plan (use full for every tool)")
	headless := fs.Bool("p", false, "print one answer and exit")
	driverMode := fs.Bool("driver", false, "run the deterministic driver (implement only)")
	llmMode := fs.Bool("llm", false, "run the LLM orchestrator (implement default)")
	checkMode := fs.Bool("check", false, "with --driver, only check the ticket graph and print start order")
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
	id := emberloop.NewSessionID()
	s := emberloop.Session{ID: id, AgentID: id, Headless: *headless, Cwd: cwd}
	// The driver flags belong to implement: any other session refuses them
	// rather than ignore what the user asked for.
	if sub != "implement" && (*driverMode || *llmMode || *checkMode) {
		return fmt.Errorf("--driver, --llm and --check apply to emberloop implement")
	}
	var implSpec *emberloop.SpecRun
	switch sub {
	case "":
		r, err := sessionRole(sub, *role)
		if err != nil {
			return err
		}
		s.Role, s.Prompt = r, rest
	case "implement":
		if *driverMode && *llmMode {
			return fmt.Errorf("emberloop implement: --driver and --llm exclude each other")
		}
		if *checkMode && !*driverMode {
			return fmt.Errorf("emberloop implement: --check needs --driver")
		}
		if rest == "" {
			return fmt.Errorf("usage: emberloop implement [--driver [--check]|--llm] <spec-file-or-dir>")
		}
		// The graph check reads files and starts no worker, so it runs
		// anywhere: no herdr pane and no integration branch needed.
		if *checkMode {
			spec, err := emberloop.ResolveSpec(rest)
			if err != nil {
				return err
			}
			return emberloop.RunDriverCheck(spec, os.Stdout)
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
		spec, err := emberloop.ResolveSpec(rest)
		if err != nil {
			return err
		}
		if *driverMode {
			// A bad graph fails before anything is created or spawned.
			if err := emberloop.RunDriverCheck(spec, io.Discard); err != nil {
				return err
			}
			created, err := emberloop.EnterIntegrationBranch(emberloop.OSExec, cwd, spec.Branch)
			if err != nil {
				return err
			}
			verb := "resuming on"
			if created {
				verb = "created"
			}
			fmt.Fprintf(os.Stderr, "emberloop: %s %s\n", verb, spec.Branch)
			if driverPaneTTY() {
				return emberloop.RunDriverPane(rest, cwd)
			}
			return emberloop.RunDriverLive(spec, cwd, os.Stdout)
		}
		r, err := sessionRole(sub, *role)
		if err != nil {
			return err
		}
		created, err := emberloop.EnterIntegrationBranch(emberloop.OSExec, cwd, spec.Branch)
		if err != nil {
			return err
		}
		verb := "resuming on"
		if created {
			verb = "created"
		}
		fmt.Fprintf(os.Stderr, "emberloop: %s %s\n", verb, spec.Branch)
		s.Role, s.Prompt, s.Spec = r, emberloop.ImplementPrompt(spec), spec.Spec
		implSpec = &spec
	default:
		found := false
		for _, e := range emberloop.SkillEntries() {
			if e.Name == sub {
				r, err := sessionRole(sub, *role)
				if err != nil {
					return err
				}
				s.Role, s.Prompt = r, emberloop.SkillPrompt(e.Skill, rest, e.Scout)
				found = true
			}
		}
		if !found {
			return fmt.Errorf("unknown command %q", sub)
		}
	}
	choice := topLevelChoice(emberloop.Choice{Harness: *harness, Model: *model, Effort: *effort})
	s.Model, s.Effort = choice.Model, choice.Effort
	// An implement run leaves a trace: one record per spec slug, updated on
	// re-run and stamped at exit. Readers decide "running" from the pane.
	runSlug := ""
	if implSpec != nil {
		rec, err := emberloop.RecordRun(emberloop.OSExec, *implSpec, cwd, s.AgentID, choice.Harness, choice.Model, choice.Effort)
		if err != nil {
			return err
		}
		runSlug = rec.Slug
	}
	launch, err := emberloop.PrepareLaunch(choice.Harness, s)
	if err != nil {
		return err
	}
	cmd, err := emberloop.BuildCommand(choice.Harness, withoutPrompt(s, launch))
	if err != nil {
		return err
	}
	// The agent reads its MCP config at startup, so the file is dead weight
	// once it exits.
	defer emberloop.CleanupSession(s)
	if launch.Late != "" {
		pane := os.Getenv("HERDR_PANE_ID")
		if pane == "" {
			return fmt.Errorf("%s needs herdr to deliver its first message; run inside herdr", choice.Harness)
		}
		go func() {
			if err := emberloop.DeliverLate(emberloop.OSExec, pane, launch.Late); err != nil {
				fmt.Fprintln(os.Stderr, "emberloop:", err)
			}
		}()
	}
	// The top-level session records its own agent state, so the herdr
	// compress key reaches it like every spawned pane: the key resolves the
	// focused pane to this agent and restarts it from this record. Outside
	// herdr there is no pane and no record. A failed write is silent: the
	// session runs, only the key stays deaf.
	if pane := os.Getenv("HERDR_PANE_ID"); pane != "" && !*headless {
		_ = emberloop.WriteTopLevelState(emberloop.TopLevelSession{
			AgentID: s.AgentID, Pane: pane, Harness: choice.Harness,
			Model: choice.Model, Effort: choice.Effort, Role: s.Role,
			Cwd: cwd, SessionID: s.ID,
		})
	}
	// The top-level session names its own pane orchestrator, so the herdr
	// sidebar names it like every spawned pane. Outside herdr there is no
	// pane and the call does nothing; a failed rename is silent.
	emberloop.NameOrchestratorPane(emberloop.OSExec)
	if pane := os.Getenv("HERDR_PANE_ID"); pane != "" && !*headless {
		stop := make(chan struct{})
		defer close(stop)
		if h := emberloop.Registered(choice.Harness); h != nil {
			go emberloop.Supervise(emberloop.OSExec, h, s, pane, stop)
		}
		// The orchestrator supervises its workers' context: a pane past its
		// threshold compresses on its own at a tool-call boundary or at
		// idle, never during a running tool call.
		go emberloop.SuperviseAutoCompress(emberloop.OSExec, func() tools.Settings {
			return tools.EffectiveSettings(cwd)
		}, stop)
	}
	// The top-level session runs under the header strip too: same binary,
	// argv and env, on a terminal sized for the strip. Without a terminal
	// (or with the strip disabled) the session runs exactly as before. The
	// strip wrapper is the child's parent here, so a bad exit still lands
	// in the agent's state, without a stderr tail the wrapper owns.
	err = emberloop.WrapTopLevel(cmd, emberloop.HeaderSource{
		AgentID: s.AgentID, AgentType: s.AgentType, AgentName: emberloop.TopLevelChipName(s.AgentType, s.Role),
		Harness: choice.Harness, SessionID: s.ID, SessionDir: s.Dir,
	})
	if exit, ok := emberloop.ChildExitOf(err); ok {
		_ = emberloop.RecordChildExit(s.AgentID, exit)
	}
	if runSlug != "" {
		_ = emberloop.FinishRun(runSlug)
	}
	return err
}

// withoutPrompt drops the prompt from s when the harness takes it late.
func withoutPrompt(s emberloop.Session, l emberloop.Launch) emberloop.Session {
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
		if _, err := emberloop.RoleTools(flagValue); err != nil {
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
func topLevelChoice(flags emberloop.Choice) emberloop.Choice {
	c := emberloop.Resolve("orchestrator", flags, emberloop.Choice{})
	if c.Harness == "claude" && c.Model == "" {
		c.Model = "sonnet"
	}
	return c
}
