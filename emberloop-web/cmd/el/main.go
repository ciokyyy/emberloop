// el is the operator CLI for the Emberloop Daemon: settings, updates, logs
// and dependencies, plus the small helpers Daemon sessions call.
//
//	el settings [decision-model <id>|threshold <n>|max-tickets <n>]  read and save settings
//	el settings models [--refresh] [--harness h]    list a harness's models
//	el settings role <slot> harness|model|effort <value>   set a launch slot
//	el settings tool <name> ... | mcp-server <name> ...    tune tools and MCP servers
//	el claude-status                                     claude statusline command: stores the 5h and weekly usage
//	el voice-reminder [hook]                             per-turn register reminder (claude hook)
//	el skills install                                    fill the skill library
//	el update                                          rebuild the UI unless unchanged, rebuild and install el, el-mcp and el-web; el install is an alias
//	el logs [-n N] [-f] [--since TIME] [--grep PATTERN]   print the prod Daemon journal
//	el restart                                         restart the prod Daemon
//	el version|--version                              print the running build's version stamp
//	el deps                                             check and install external dependencies
package main

import (
	"fmt"
	"os"
	"strings"

	"example.com/emberloop-web/el"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "el:", err)
		os.Exit(2)
	}
}

func run(argv []string) error {
	sub := ""
	if len(argv) > 0 && !strings.HasPrefix(argv[0], "-") {
		sub, argv = argv[0], argv[1:]
	}
	switch sub {
	case "claude-status":
		return el.RunClaudeStatus(os.Stdin, "")
	case "voice-reminder":
		return el.RunVoiceReminder(argv, os.Stdout)
	case "settings":
		return runSettings(argv, os.Stdout, cachedModels)
	case "deps":
		return el.RunDeps(argv, os.Stdout)
	case "update", "install":
		return el.RunUpdate(argv, os.Stdout)
	case "logs":
		return el.RunLogs(argv, os.Stdout)
	case "restart":
		return el.RunRestart(argv, os.Stdout)
	case "version":
		fmt.Println(el.VersionString())
		return nil
	case "skills":
		if len(argv) != 1 || argv[0] != "install" {
			return fmt.Errorf("usage: el skills install")
		}
		if err := el.InstallSkills(el.OSExec); err != nil {
			return err
		}
		fmt.Printf("installed %s into %s\n", "mattpocock/skills", el.SkillLibraryDir())
		return nil
	case "":
		for _, a := range argv {
			if a == "--version" || a == "-version" {
				fmt.Println(el.VersionString())
				return nil
			}
		}
		return fmt.Errorf("usage: el <settings|update|logs|restart|version|deps|skills|claude-status|voice-reminder>")
	default:
		return fmt.Errorf("unknown command %q", sub)
	}
}
