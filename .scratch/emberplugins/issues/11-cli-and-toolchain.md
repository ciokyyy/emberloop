# 11: CLI and shared toolchain

**What to build:** An agent in a Session scaffolds, builds and debugs an Emberplugin through bash with `emberloop-web plugin new|build|logs|status`. `new` creates a folder from a template using the kit. `build` runs one shared Vite and Svelte toolchain installed once under the state dir, with aliases to the kit and all shadcn-svelte components, and writes `dist/`. `logs` and `status` show build errors, runtime logs, job runs and process state. No MCP tool.

**Blocked by:** 04 (Full shadcn-svelte kit and page templates)

**Status:** ready-for-agent

- [ ] `plugin new <name>` scaffolds a manifest, a Svelte page from a template, and a Go server skeleton on request
- [ ] The shared toolchain installs once under the state dir and is reused by every plugin; no per-plugin `node_modules`
- [ ] `plugin build` compiles to `dist/` and prints build errors in a form an agent can fix
- [ ] `plugin logs` and `plugin status` read server logs, job runs and process state
- [ ] A clear message when node or bun is missing
- [ ] A built plugin appears in the sidebar through hot reload with no extra step
- [ ] Tests cover scaffold output, build of the template with a stubbed toolchain, and the error output shape
