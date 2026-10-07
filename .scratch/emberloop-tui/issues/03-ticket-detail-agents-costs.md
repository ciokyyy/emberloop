# 03: Ticket detail with agents, tokens, cost and memory

**What to build:** Enter or double-click on a ticket opens its detail: title and Markdown body rendered with glamour, scrollable; below it every agent attached (worker, code reviewer, spec reviewer, scouts) with harness, `model · effort`, status, tokens (input, output, cache), reported cost or "no cost reported", find/jev calls, and current RSS; plus ticket totals. Spec rows and the orchestrator also show live tokens, reported cost and RSS, and the header shows machine-wide RSS of all running agents. Usage logs are read incrementally.

**Blocked by:** 02, run-records 03, run-records 04, run-records 06

**Status:** done

- [x] Detail view opens by Enter and double-click; Esc goes back
- [x] Per-agent and per-ticket totals from usage logs; per-spec totals include the orchestrator
- [x] Missing cost shown as tokens plus "no cost reported", never estimated
- [x] Latest memory sample per agent; machine-wide total in the header
- [x] `Snapshot` tests over fixture usage and memory logs; incremental reads don't double-count
