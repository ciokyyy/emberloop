// Command emberloop-web runs the Enderbloom Web Daemon: it owns Sessions,
// serves the embedded UI and streams Session Events over SSE. It binds the
// loopback interface only and rejects foreign Host and Origin headers.
package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"

	web "example.com/emberloop-web"
)

func main() {
	// Spawn-server mode serves the Daemon-hosted spawn MCP tool over stdio
	// for one claude Session: the adapter starts this per Session with the
	// parent and Daemon URL in its environment.
	if len(os.Args) > 1 && os.Args[1] == "spawn-server" {
		parent := os.Getenv(web.SpawnParentEnv)
		daemonURL := os.Getenv(web.SpawnDaemonEnv)
		if parent == "" || daemonURL == "" {
			fmt.Fprintln(os.Stderr, "spawn-server: "+web.SpawnParentEnv+" and "+web.SpawnDaemonEnv+" are required")
			os.Exit(2)
		}
		if err := web.ServeSpawnMCP(os.Stdin, os.Stdout, daemonURL, parent); err != nil {
			fmt.Fprintln(os.Stderr, "spawn-server:", err)
			os.Exit(1)
		}
		return
	}
	addr := flag.String("addr", "127.0.0.1:0", "loopback address to bind")
	stateDir := flag.String("state-dir", defaultStateDir(), "pig state directory (Sessions live in its enderbloom subtree)")
	flag.Parse()

	host, _, err := net.SplitHostPort(*addr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bad addr:", err)
		os.Exit(1)
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		if host != "localhost" {
			fmt.Fprintln(os.Stderr, "refusing non-loopback bind")
			os.Exit(1)
		}
	}
	// Listen first: the claude spawn server calls the Daemon back over
	// loopback HTTP, so backends need the bound address before serving.
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "listen:", err)
		os.Exit(1)
	}
	daemonURL := "http://" + ln.Addr().String()
	d := web.NewDaemon(*stateDir, web.NewFakeBackend())
	pig := web.NewPigBackend()
	pig.Spawn = func(parentID string, agents []web.SpawnAgent) (string, []web.SessionMeta, error) {
		gid, kids, code, msg := d.Spawn(parentID, agents)
		if code != 0 {
			return "", nil, fmt.Errorf("spawn: %s", msg)
		}
		return gid, kids, nil
	}
	d.RegisterBackend("pig", pig)
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "executable:", err)
		os.Exit(1)
	}
	claude := web.NewClaudeBackend()
	claude.SetSpawnServer([]string{exe, "spawn-server"}, daemonURL)
	d.WithClaudeBackend(claude)
	fmt.Println("emberloop-web on " + daemonURL)
	if err := http.Serve(ln, d.Handler()); err != nil {
		fmt.Fprintln(os.Stderr, "serve:", err)
		os.Exit(1)
	}
}

func defaultStateDir() string {
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "pig")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, ".local", "share", "pig")
}
