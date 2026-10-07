// emberloop-mcp serves emberloop's tools over MCP stdio. The role (flag or EMBERLOOP_ROLE)
// picks the tool set, and the working directory is the workspace root. There
// is no default role: started with none, the server refuses with a message
// naming the roles. Ask for the full tool set explicitly, with --role full or
// EMBERLOOP_ROLE=full.
// It is also the session's own emberloop process: at startup it snapshots the
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
	"strings"

	"example.com/emberloop"
	"example.com/emberloop/mcp"
	"example.com/emberloop/tools"
)

func main() {
	os.Exit(run())
}

// run serves the session and returns its exit code. The registry's Close is
// deferred here, so every MCP server child dies however the session ends:
// an os.Exit in main would skip it.
func run() int {
	role := flag.String("role", emberloop.RoleFromEnv(), "one of: "+strings.Join(emberloop.RoleNames(), ", "))
	flag.Parse()

	if *role == "" {
		fmt.Fprintln(os.Stderr, "emberloop-mcp:", emberloop.MissingRoleError())
		return 2
	}
	// The role resolves before any MCP server starts: an unknown role must
	// not spawn a server per configured entry only to discard them. This
	// resolution is side-effect free; the MCP listing below re-resolves with
	// the snapshot included.
	if _, err := emberloop.RoleTools(*role); err != nil {
		fmt.Fprintln(os.Stderr, "emberloop-mcp:", err)
		return 2
	}
	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "emberloop-mcp:", err)
		return 1
	}
	// The registry holds one client per configured MCP server for this
	// process, plus the synthesized graph server when it is enabled and
	// installed. It starts no server until the snapshot lists it; a server
	// that fails is warned about and skipped.
	mcpReg := tools.NewRegistry(tools.WithGraphServer(tools.EffectiveSettings(root)).MCPServers)
	defer mcpReg.Close()
	served, err := emberloop.RoleToolsWithMCP(*role, emberloop.MCPSnapshotWithGraph(mcpReg, root))
	if err != nil {
		fmt.Fprintln(os.Stderr, "emberloop-mcp:", err)
		return 2
	}
	// The memory sampler records this session's harness process tree every 5
	// seconds into the agent's archive log, whether or not the TUI is open.
	// It ends with this process, which is the session's end.
	memCtx, stopMem := context.WithCancel(context.Background())
	defer stopMem()
	go emberloop.StartMemorySampler(memCtx, os.Getenv("EMBERLOOP_AGENT_ID"), emberloop.MemoryRunSlug(), emberloop.MemorySampleInterval)
	if err := emberloop.SnapshotFromEnv(served); err != nil {
		fmt.Fprintln(os.Stderr, "emberloop-mcp: archive:", err)
	}
	// A harness session tails its own transcript into its agent's usage log
	// while it serves; when stdin closes the harness has settled, so one
	// last pass catches requests written after the final tick. The pass runs
	// however Serve ends: a broken transport still settles the session.
	tailCfg, tailing := emberloop.UsageTailFromEnv()
	stopTail := func() {}
	if tailing {
		stop := make(chan struct{})
		stopTail = func() { close(stop) }
		go emberloop.RunUsageTail(tailCfg, stop)
	}
	stopArchive := emberloop.StartArchiveLoop()
	serveErr := mcp.Serve(context.Background(), os.Stdin, os.Stdout, root, served)
	stopTail()
	if tailing {
		if err := emberloop.TailUsageFile(tailCfg); err != nil {
			fmt.Fprintln(os.Stderr, "emberloop-mcp: final usage pass:", err)
		}
	}
	stopArchive()
	if _, err := emberloop.CopyFromEnv(); err != nil {
		fmt.Fprintln(os.Stderr, "emberloop-mcp: archive:", err)
	}
	if serveErr != nil {
		fmt.Fprintln(os.Stderr, "emberloop-mcp:", serveErr)
		return 1
	}
	return 0
}
