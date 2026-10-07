#!/bin/sh
# Probe-only stdio MCP shim (see ../README.md). Runs the sibling .py.
HERE=$(dirname "$0")
exec python3 "$HERE/mcp-shim.py"
