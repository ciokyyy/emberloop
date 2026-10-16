# 09: A design Ticket gets a Designer

**What to build:** When a Ticket's effective type is `design`, its Worker starts from the `worker-design` Chain instead of `worker`. An empty `worker-design` Chain uses the `worker` Chain and records a warning on the Ticket. The Designer's first prompt is the usual Worker prompt (Ticket, spec, AGENTS.md chain, skills) plus a Designer section that says the Ticket builds on the merged `code` Ticket and owns layout, styling, visual states and copy, may change markup but not logic or test behaviour, and that the Ticket's own tests and `.emberloop/check` must still pass. The scope warning treats logic and test files as out of scope for a Designer, advisory like today's. Chain position, fallback to the next entry on quota failure, Switch and Resume behave as for `worker`. The `code` path is unchanged. The browser skill text is ticket 11; this ticket leaves a hook for it. Re-read `runs_exec.go` (`roleWorkerConfig`, `primaryRunRoleConfig`), `runs_inject.go` (`formatRunFirstPrompt`, `defaultRunSkills`), `prompts.go` (`workerTaskPrompt`) and `runs_reviewer_gates.go` (scope warning).

**Blocked by:** 02-ticket-type-parse, 08-design-chain-roles, 06-run-type-override-api

**Status:** ready-for-agent

- [ ] A `design` Ticket's Worker uses the `worker-design` primary; a `code` Ticket still uses `worker`
- [ ] Empty `worker-design` falls back to `worker` and records a Ticket-level warning in the Run snapshot
- [ ] The Designer prompt carries the Designer section; the `code` prompt is byte-identical to today
- [ ] Quota failure on the Designer advances the `worker-design` Chain with a Handoff; position persists across restart
- [ ] The Run override (06) changes routing before Start
- [ ] Run driver tests with the fake Backend cover routing, fallback warning, prompt section and scope warning; `.emberloop/check` passes
