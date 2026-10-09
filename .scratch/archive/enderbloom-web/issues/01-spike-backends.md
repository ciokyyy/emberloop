# 01: Spike the two Backends

**What to build:** A small probe program, committed under `el-web/spike/` and never imported by the product, that proves both Backends can run stripped and stream Events. For claude: start the ACP adapter, create a session with MCP-only tools, send a prompt, print every update, try a prompt mid-turn, and note whether session load works. For pig: use the Go SDK to create a session with MCP-only tools, subscribe, send a prompt, print every event, and call the steer and follow-up queues mid-run. Write the answers to `.scratch/archive/enderbloom-web/spike-findings.md` (committed) and propose edits to tickets 03 to 05 if a finding changes them. The probes must actually run against the installed `claude` and `pig`; an investigation from source alone does not complete this ticket.

**Blocked by:** None (can start immediately)

**Status:** done

- [x] The probe programs are committed and re-runnable with one command each
- [x] Each of the seven open questions in the spec's Spike Findings has a written answer with evidence (captured output)
- [x] The claude run shows no native tools offered, only the el MCP tools
- [x] The pig run shows no native tools offered, only the el MCP tools
- [x] A recommendation is recorded: adapter or fallback for claude, with the reason
- [x] Any change to tickets 03 to 05 is written into those files
