package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	web "example.com/emberloop-web"
	"golang.org/x/term"
)

const userUsage = "usage: emberloop-web user set-password [-state-dir DIR] <name>"

// runUserCommand runs `emberloop-web user set-password [-state-dir DIR] NAME`.
// The new password comes from stdin: a terminal asks twice with echo off, a
// pipe supplies one line. A running Daemon signs the User out on its next
// request. It returns the exit status.
func runUserCommand(args []string, stdin *os.File, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "set-password" {
		fmt.Fprintln(stderr, userUsage)
		return 2
	}
	fs := flag.NewFlagSet("user set-password", flag.ContinueOnError)
	fs.SetOutput(stderr)
	stateDir := fs.String("state-dir", defaultStateDir(), "pig state directory the Daemon uses")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, userUsage)
		return 2
	}
	name := fs.Arg(0)
	password, err := readNewPassword(stdin, stderr)
	if err != nil {
		fmt.Fprintln(stderr, "emberloop-web:", err)
		return 1
	}
	if err := web.SetUserPassword(*stateDir, name, password); err != nil {
		fmt.Fprintln(stderr, "emberloop-web:", err)
		return 1
	}
	fmt.Fprintf(stdout, "password reset for %s; its sessions are signed out\n", name)
	return 0
}

// readNewPassword reads the new password: on a terminal, twice with echo off;
// otherwise one line from stdin.
func readNewPassword(stdin *os.File, stderr io.Writer) (string, error) {
	fd := int(stdin.Fd())
	if !term.IsTerminal(fd) {
		line, err := bufio.NewReader(stdin).ReadString('\n')
		if err != nil && err != io.EOF {
			return "", err
		}
		return strings.TrimRight(line, "\r\n"), nil
	}
	fmt.Fprint(stderr, "New password: ")
	first, err := term.ReadPassword(fd)
	fmt.Fprintln(stderr)
	if err != nil {
		return "", err
	}
	fmt.Fprint(stderr, "Repeat password: ")
	second, err := term.ReadPassword(fd)
	fmt.Fprintln(stderr)
	if err != nil {
		return "", err
	}
	if string(first) != string(second) {
		return "", errors.New("passwords do not match")
	}
	return string(first), nil
}
