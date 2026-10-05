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
	if err := mcp.Serve(context.Background(), os.Stdin, os.Stdout, root, served); err != nil {
		fmt.Fprintln(os.Stderr, "wick-mcp:", err)
		os.Exit(1)
	}
}
