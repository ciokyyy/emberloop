# Ticket map — workspace overhaul

All parts of the spec live here. Quota-tab tickets 03 to 16 moved in as Part 1 (04 to 16 here; the old quota docs ticket folded into 37). Quota-tab keeps only its done tickets 01 and 02.

| Ticket | Part | Blocked by |
|---|---|---|
| 01-run-test-cleanup-never-holds-lock | 7 Tests | None |
| 02-run-test-slot-watchdog-and-deadline | 7 Tests | 01 |
| 03-suite-time-target-and-check-timeout | 7 Tests | 01, 02 |
| 04-cached-quota-read | 1 Quota | 03 |
| 05-concurrent-fetch-isolation | 1 Quota | 04 |
| 06-percent-contract | 1 Quota | 03 |
| 07-claude-poller | 1 Quota | 06 |
| 08-chatgpt-plus-card | 1 Quota | 06 |
| 09-openrouter-card | 1 Quota | 06 |
| 10-live-topic-refresher | 1 Quota | 05, 06 |
| 11-refresh-min-interval | 1 Quota | 05 |
| 12-quota-card-redesign | 1 Quota | 03 |
| 13-quota-sidebar | 1 Quota | 03 |
| 14-quota-phone-drawer | 1 Quota | 13 |
| 15-quota-live-patching | 1 Quota | 10, 12, 13 |
| 16-quota-live-probe | 1 Quota | 07, 08 |
| 17-runs-list-summaries | 2 Speed | 03 |
| 18-runs-git-cache | 2 Speed | 17 |
| 19-ticket-session-index | 2 Speed | 17 |
| 20-coalesced-runs-refetch | 2 Speed | 17 |
| 21-non-blocking-boot | 2 Speed | 15, 20 |
| 22-providers-page-and-api-keys | 3 Providers | 21 |
| 23-chatgpt-device-code-connect | 3 Providers | 22 |
| 24-antigravity-paste-connect | 3 Providers | 22 |
| 25-claude-pty-connect | 3 Providers | 22 |
| 26-cli-only-rows-and-redirect-base | 3 Providers | 22 |
| 27-quota-links-to-providers | 3 Providers | 23, 24, 25 |
| 28-dashboard-payload | 4 Project scope | 21 |
| 29-project-gist | 4 Project scope | 03 |
| 30-project-scope-routes | 4 Project scope | 27, 28 |
| 31-project-runs-and-settings-shell | 4 Project scope | 29, 30 |
| 32-draft-project-pick-api | 5 Draft pick | 29 |
| 33-draft-pick-banner | 5 Draft pick | 30, 32 |
| 34-settings-hub | 6 Settings | 31 |
| 35-agents-chains-with-effort | 6 Settings | 34 |
| 36-runs-and-memory-settings-pages | 6 Settings | 34 |
| 37-docs-and-glossary | 1-7 Docs | 09, 11, 14, 15, 18, 19, 26, 33, 35, 36 |

## Waves

1. 01
2. 02
3. 03
4. 04, 06, 12, 13, 17, 29
5. 05, 07, 08, 09, 14, 18, 19, 20, 32
6. 10, 11, 16
7. 15
8. 21
9. 22, 28
10. 23, 24, 25, 26
11. 27
12. 30
13. 31, 33
14. 34
15. 35, 36
16. 37

The Rail, `areas.js` and `App.svelte` change in 13, 15, 21, 22, 30 and 34, so those run one after the other through the blocker chain. 12 and 13 build to the response contract with stubbed payloads, so they start before the Daemon side.
