package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	web "example.com/emberloop-web"
	"golang.org/x/term"
)

const claudeTokenUsage = "usage: emberloop-web claude-token set [-state-dir DIR]"

// runClaudeTokenCommand runs `emberloop-web claude-token set [-state-dir
// DIR]`. The token comes from stdin: a terminal asks once with echo off, a
// pipe supplies one line. It writes the 0600 shared-token file the Daemon
// loads as CLAUDE_CODE_OAUTH_TOKEN, and returns the exit status. It never
// prints the token.
func runClaudeTokenCommand(args []string, stdin *os.File, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "set" {
		fmt.Fprintln(stderr, claudeTokenUsage)
		return 2
	}
	fs := flag.NewFlagSet("claude-token set", flag.ContinueOnError)
	fs.SetOutput(stderr)
	stateDir := fs.String("state-dir", defaultStateDir(), "pig state directory the Daemon uses")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, claudeTokenUsage)
		return 2
	}
	token, err := readClaudeToken(stdin, stderr)
	if err != nil {
		fmt.Fprintln(stderr, "emberloop-web:", err)
		return 1
	}
	if err := web.SetClaudeOAuthToken(*stateDir, token); err != nil {
		fmt.Fprintln(stderr, "emberloop-web:", err)
		return 1
	}
	fmt.Fprintf(stdout, "claude token stored in %s; restart the Daemon\n", web.ClaudeOAuthTokenPath(*stateDir))
	return 0
}

// readClaudeToken reads the setup token: on a terminal once with echo off,
// otherwise one line from stdin. The value is trimmed; empties are refused by
// SetClaudeOAuthToken.
func readClaudeToken(stdin *os.File, stderr io.Writer) (string, error) {
	fd := int(stdin.Fd())
	if !term.IsTerminal(fd) {
		line, err := bufio.NewReader(stdin).ReadString('\n')
		if err != nil && err != io.EOF {
			return "", err
		}
		return strings.TrimSpace(line), nil
	}
	fmt.Fprint(stderr, "Claude setup token: ")
	token, err := term.ReadPassword(fd)
	fmt.Fprintln(stderr)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(token)), nil
}
