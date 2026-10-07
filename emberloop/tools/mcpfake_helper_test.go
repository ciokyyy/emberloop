package tools

import (
	"testing"

	"example.com/emberloop/tools/mcpfake"
)

// TestMcpFakeHelperProcess is the fake MCP server's entry point: mcpfake
// re-executes this test binary with -test.run=TestMcpFakeHelperProcess and
// serves stdio there. A normal `go test` run has no mode and skips.
func TestMcpFakeHelperProcess(t *testing.T) {
	mcpfake.RunIfHelper(t)
}
