// wick-mcp serves wick's tools over MCP stdio. The role (flag or WICK_ROLE)
// picks the tool set, and the working directory is the workspace root.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"example.com/wick"
	"example.com/wick/mcp"
)

func main() {
	role := flag.String("role", wick.RoleFromEnv(), "implement, review, scout, work, plan, agent or full")
	flag.Parse()

	served, err := wick.RoleTools(*role)
	if err != nil {
		fmt.Fprintln(os.Stderr, "wick-mcp:", err)
		os.Exit(2)
	}
	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "wick-mcp:", err)
		os.Exit(1)
	}
	// The memory sampler records this session's harness process tree every 5
	// seconds into the agent's archive log, whether or not the TUI is open.
	// It ends with this process, which is the session's end.
	memCtx, stopMem := context.WithCancel(context.Background())
	defer stopMem()
	go wick.StartMemorySampler(memCtx, os.Getenv("WICK_AGENT_ID"), wick.MemoryRunSlug(), wick.MemorySampleInterval)
	// A harness session tails its own transcript into its agent's usage log
	// while it serves; when stdin closes the harness has settled, so one
	// last pass catches requests written after the final tick. The pass runs
	// however Serve ends: a broken transport still settles the session.
	tailCfg, tailing := wick.UsageTailFromEnv()
	stopTail := func() {}
	if tailing {
		stop := make(chan struct{})
		stopTail = func() { close(stop) }
		go wick.RunUsageTail(tailCfg, stop)
	}
	serveErr := mcp.Serve(context.Background(), os.Stdin, os.Stdout, root, served)
	stopTail()
	if tailing {
		if err := wick.TailUsageFile(tailCfg); err != nil {
			fmt.Fprintln(os.Stderr, "wick-mcp: final usage pass:", err)
		}
	}
	if serveErr != nil {
		fmt.Fprintln(os.Stderr, "wick-mcp:", serveErr)
		os.Exit(1)
	}
}
