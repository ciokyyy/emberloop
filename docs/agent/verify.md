# Verify emberloop

- Repo check, from the repo root: `.emberloop/check`. It runs `go vet` and the tests for `emberloop-web/` (including `el/`) and `installer/`; `TestLive*` are skipped.
- Web UI tests: `cd emberloop-web/web && npx vitest run`. Results land in `.vitest/json/output.json`.
- Real claude adapter probe (needs claude login, node and network): `cd emberloop-web && EL_LIVE=claude go test -tags live -run TestLiveClaudeBackend .`.
- After an update, `el --help` must run and `el-web` must answer on http://127.0.0.1:8787 (see `el://docs/webui`); report which of these you checked.
