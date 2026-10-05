// wick-mcp serves wick's tools over MCP stdio. The role (flag or WICK_ROLE)
// picks the tool set, and the working directory is the workspace root. There
// is no default role: started with none, the server refuses with a message
// naming the roles. Ask for the full tool set explicitly, with --role full or
// WICK_ROLE=full.
// It is also the session's own wick process: at startup it snapshots the
// session's system prompt, served tool definitions and harness flags into the
// blob store, tails the harness transcript into the archive and the usage log
// while it serves, samples its harness process tree's memory, and takes final
// copies when the harness goes away, so the transcript outlives harness
// cleanup.
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

	if *role == "" {
		fmt.Fprintln(os.Stderr, "wick-mcp:", wick.MissingRoleError())
		os.Exit(2)
	}
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
	if err := wick.SnapshotFromEnv(served); err != nil {
		fmt.Fprintln(os.Stderr, "wick-mcp: archive:", err)
	}
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
	stopArchive := wick.StartArchiveLoop()
	serveErr := mcp.Serve(context.Background(), os.Stdin, os.Stdout, root, served)
	stopTail()
	if tailing {
		if err := wick.TailUsageFile(tailCfg); err != nil {
			fmt.Fprintln(os.Stderr, "wick-mcp: final usage pass:", err)
		}
	}
	stopArchive()
	if _, err := wick.CopyFromEnv(); err != nil {
		fmt.Fprintln(os.Stderr, "wick-mcp: archive:", err)
	}
	if serveErr != nil {
		fmt.Fprintln(os.Stderr, "wick-mcp:", serveErr)
		os.Exit(1)
	}
}
