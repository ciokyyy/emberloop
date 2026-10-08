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
	addr := flag.String("addr", "127.0.0.1:0", "loopback address to bind")
	stateDir := flag.String("state-dir", defaultStateDir(), "pig state directory (Sessions live in its enderbloom subtree)")
	flag.Parse()

	d := web.NewDaemon(*stateDir, web.NewFakeBackend())
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "listen:", err)
		os.Exit(1)
	}
	if !ln.Addr().(*net.TCPAddr).IP.IsLoopback() {
		fmt.Fprintln(os.Stderr, "refusing non-loopback bind")
		os.Exit(1)
	}
	fmt.Println("emberloop-web on http://" + ln.Addr().String())
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
