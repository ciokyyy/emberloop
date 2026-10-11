// Command el-web runs the Emberloop Daemon: it owns Sessions,
// serves the embedded UI and streams Session Events over SSE. It binds the
// loopback interface only and rejects foreign Host and Origin headers.
package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"example.com/emberloop/tools"

	web "example.com/emberloop-web"
)

func main() {
	// Shim mode is the pi stand-in pi-acp starts for a pig Session: it runs
	// pig and relays RPC, and takes native steer commands on a socket.
	if len(os.Args) > 2 && os.Args[1] == web.PigShimMode {
		os.Exit(web.RunPigShim(os.Args[2], os.Args[3:], os.Getenv("EL_PIG_STEER_SOCK")))
	}
	addr := flag.String("addr", "127.0.0.1:0", "loopback address to bind")
	stateDir := flag.String("state-dir", defaultStateDir(), "pig state directory (Sessions live in its enderbloom subtree)")
	restartDrain := flag.Duration("restart-drain", 5*time.Minute, "longest wait for Sessions to go idle after a restart request")
	restartCheck := flag.Duration("restart-check", time.Minute, "how often to look for a rebuilt binary and restart when idle (0 turns it off)")
	allowHosts := flag.String("allow-host", os.Getenv("EL_WEB_ALLOW_HOSTS"), "comma-separated extra Host names to accept, for a local reverse proxy such as tailscale serve (still binds loopback)")
	tailnet := flag.Bool("tailnet", false, "also listen on this machine's Tailscale IP (same port as -addr), reachable by your tailnet only")
	showVersion := flag.Bool("version", false, "print the running build's version stamp and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(web.VersionString())
		return
	}

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
	d.SetAllowedHosts(strings.Split(*allowHosts, ","))
	pig := web.NewPigBackend()
	pig.SharedTools = true
	pig.DaemonURL = daemonURL
	pig.LogDir = filepath.Join(*stateDir, "enderbloom", "pig-logs")
	pig.Spawn = func(parentID string, agents []web.SpawnAgent, deliver string) (string, []web.SessionMeta, error) {
		gid, kids, code, msg := d.Spawn(parentID, agents, deliver)
		if code != 0 {
			return "", nil, fmt.Errorf("spawn: %s", msg)
		}
		return gid, kids, nil
	}
	pig.SteerChild = func(parentID, childID, text string) (string, error) {
		outcome, code, msg := d.SteerChild(parentID, childID, text)
		if code != 0 {
			return "", fmt.Errorf("steer: %s", msg)
		}
		return outcome, nil
	}
	pig.CheckChild = func(parentID, childID string) (string, error) {
		var outcome string
		var code int
		var msg string
		if childID == "" {
			outcome, code, msg = d.CheckChildren(parentID)
		} else {
			outcome, code, msg = d.CheckChild(parentID, childID)
		}
		if code != 0 {
			return "", fmt.Errorf("check: %s", msg)
		}
		return outcome, nil
	}
	pig.CancelChild = func(parentID, childID string, kill bool) (string, error) {
		outcome, code, msg := d.CancelChild(parentID, childID, kill)
		if code != 0 {
			return "", fmt.Errorf("cancel: %s", msg)
		}
		return outcome, nil
	}
	pig.SudoGate = func(sessionID string) func(context.Context, string, string) (string, bool) {
		return tools.HTTPSudoGate(daemonURL, sessionID)
	}
	d.RegisterBackend("pig", pig)
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "executable:", err)
		os.Exit(1)
	}
	// Pig Sessions run through pi-acp (one adapter process each) unless
	// EL_PIG_SDK=1 keeps the in-process SDK Backend registered above.
	if os.Getenv("EL_PIG_SDK") != "1" {
		acp, err := web.NewPigACPBackend(*stateDir, exe, daemonURL)
		if err != nil {
			fmt.Fprintln(os.Stderr, "el-web: pig over ACP unavailable, using the SDK:", err)
		} else {
			d.RegisterBackend("pig", acp)
		}
	}
	// Grok Sessions run `grok agent stdio` on the user's Grok login; the
	// harness stays unregistered when grok or el-mcp is missing.
	if grok, err := web.NewGrokACPBackend(*stateDir, daemonURL); err != nil {
		fmt.Fprintln(os.Stderr, "el-web: grok harness unavailable:", err)
	} else {
		d.RegisterBackend("grok", grok)
	}
	claude := web.NewClaudeBackend()
	claude.SetDaemonAddr(daemonURL)
	d.WithClaudeBackend(claude)
	go watchForUpdate(d, exe, *restartCheck)
	go watchForRestartRequest(d, exe, *restartDrain)
	go signalRestart(d)
	d.ResumeAfterRestart()
	if *tailnet {
		d.AllowTailnet(true)
		_, port, _ := net.SplitHostPort(ln.Addr().String())
		go serveTailnet(d.Handler(), port)
	}
	fmt.Println("el-web on " + daemonURL)
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

// tailnetIP returns this machine's Tailscale IPv4 address, or nil.
func tailnetIP() net.IP {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	for _, a := range addrs {
		if ipn, ok := a.(*net.IPNet); ok && web.IsTailnetIP(ipn.IP) {
			return ipn.IP
		}
	}
	return nil
}

// serveTailnet serves the Daemon handler on the Tailscale address until it
// works. Tailscale may come up after the Daemon at boot, so it retries.
func serveTailnet(h http.Handler, port string) {
	for {
		if ip := tailnetIP(); ip != nil {
			addr := net.JoinHostPort(ip.String(), port)
			ln, err := net.Listen("tcp", addr)
			if err == nil {
				fmt.Println("el-web on tailnet http://" + addr)
				err = http.Serve(ln, h)
			}
			fmt.Fprintln(os.Stderr, "tailnet listener:", err)
		}
		time.Sleep(10 * time.Second)
	}
}
