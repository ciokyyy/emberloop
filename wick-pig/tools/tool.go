// Package tools holds the agent tools both hosts serve: pig through an
// extension adapter, Claude through the MCP server. It imports nothing from
// pig, so a plain go build of cmd/wick-mcp works.
package tools

import (
	"context"
	"encoding/json"
)

// Tool is one capability exposed by JSON schema. root is the workspace the
// call runs in: pig only knows it per call (ctx.Cwd()), MCP knows it from the
// directory the server was launched in.
type Tool interface {
	Name() string
	Description() string
	Schema() map[string]any
	Execute(ctx context.Context, root string, args json.RawMessage) (string, error)
}
