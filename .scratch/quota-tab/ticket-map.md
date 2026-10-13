# Quota tab tickets

Spec: `spec.md` (decides conflicts). Status vocabulary: `done`, `ready-for-agent`.

| NN | Title | Blocked by | Status |
|----|-------|------------|--------|
| 01 | Quota API with manual refresh | None | done |
| 02 | Quota tab UI | 01 | done |
| 03 | Cached non-blocking quota read with measured baseline | None | ready-for-agent |
| 04 | Per-provider concurrent fetch isolation | 03 | ready-for-agent |
| 05 | Percent contract with word labels and states | None | ready-for-agent |
| 06 | Claude read-only host-login poller | 05 | ready-for-agent |
| 07 | ChatGPT Plus quota card | 05 | ready-for-agent |
| 08 | OpenRouter credits card | 05 | ready-for-agent |
| 09 | Live quota topic with subscriber-gated background refresher | 04, 05 | ready-for-agent |
| 10 | Minimum interval on manual refresh | 04 | ready-for-agent |
| 11 | Quota card redesign with secondary detail and states | None | ready-for-agent |
| 12 | Quota second sidebar with Live status list | None | ready-for-agent |
| 13 | Phone drawer list with one-column cards | 12 | ready-for-agent |
| 14 | Live patching of open Quota pages | 09, 11, 12 | ready-for-agent |
| 15 | Quota docs and index updates | 08, 10, 13, 14 | ready-for-agent |
| 16 | Gated live probe of real quota endpoints | 06, 07 | ready-for-agent |

## Parallel frontier waves

Tickets that change the same code are chained (03, 04, 10 share the quota read and refresh path; 11, 14 share the page; 12, 13 share the shell), so parallel waves do not collide.

- Wave 1 (no blockers): 03, 05, 11, 12
- Wave 2: 04 (after 03); 06, 07, 08 (after 05); 13 (after 12)
- Wave 3: 09 (after 04, 05); 10 (after 04); 16 (after 06, 07)
- Wave 4: 14 (after 09, 11, 12)
- Wave 5: 15 (after 08, 10, 13, 14)

UI tickets 11 and 12 build to the spec response contract with stubbed payloads and a mocked channel, so they run in wave 1 without waiting for the Daemon side.
