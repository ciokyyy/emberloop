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
	if err := mcp.Serve(context.Background(), os.Stdin, os.Stdout, root, served); err != nil {
		fmt.Fprintln(os.Stderr, "wick-mcp:", err)
		os.Exit(1)
	}
}
