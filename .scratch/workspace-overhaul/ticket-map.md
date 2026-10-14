# Ticket map — workspace overhaul

Part 1 (Quota) is not cut here: `.scratch/quota-tab/issues/03-*` to `16-*` cover it. Start this Run after the quota-tab spec has merged to master, because tickets 09, 17 and 21 build on its Quota shell.

| Ticket | Part | Blocked by |
|---|---|---|
| 01-run-test-cleanup-never-holds-lock | 7 | None |
| 02-run-test-slot-watchdog-and-deadline | 7 | 01 |
| 03-suite-time-target-and-check-timeout | 7 | 01, 02 |
| 04-runs-list-summaries | 2 Speed | 03 |
| 05-runs-git-cache | 2 Speed | 04 |
| 06-ticket-session-index | 2 Speed | 04 |
| 07-coalesced-runs-refetch | 2 Speed | 04 |
| 08-non-blocking-boot | 2 Speed | 07 |
| 09-providers-page-and-api-keys | 3 Providers | 08 |
| 10-chatgpt-device-code-connect | 3 Providers | 09 |
| 11-antigravity-paste-connect | 3 Providers | 09 |
| 12-claude-pty-connect | 3 Providers | 09 |
| 13-cli-only-rows-and-redirect-base | 3 Providers | 09 |
| 14-quota-links-to-providers | 3 Providers | 10, 11, 12 |
| 15-dashboard-payload | 4 Project scope | 08 |
| 16-project-gist | 4 Project scope | 03 |
| 17-project-scope-routes | 4 Project scope | 14, 15 |
| 18-project-runs-and-settings-shell | 4 Project scope | 16, 17 |
| 19-draft-project-pick-api | 5 Draft pick | 16 |
| 20-draft-pick-banner | 5 Draft pick | 17, 19 |
| 21-settings-hub | 6 Settings | 18 |
| 22-agents-chains-with-effort | 6 Settings | 21 |
| 23-runs-and-memory-settings-pages | 6 Settings | 21 |
| 24-docs-and-glossary | 2-7 | 05, 06, 13, 20, 22, 23 |

## Waves

1. 01
2. 02
3. 03
4. 04, 16
5. 05, 06, 07, 19
6. 08
7. 09, 15
8. 10, 11, 12, 13
9. 14
10. 17
11. 18, 20
12. 21
13. 22, 23
14. 24

The Rail, `areas.js` and `App.svelte` change in 08, 09, 17 and 21, so those run one after the other through the blocker chain.
