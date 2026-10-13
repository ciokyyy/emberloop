# Projects, Providers, Settings and Speed

## Problem Statement

Everything the Quota spec leaves out of the design agreed with the owner on 2026-10-13 is still unbuilt.

- The sidebar lists every Project by itself and the Runs page needs a folder typed in. There is no way to see what runs in which Project, and a Session started outside a Project lands in the User's home directory under "Other".
- Accounts cannot be connected from the browser. ChatGPT, Antigravity and Claude logins and pig API keys need the pig CLI on the host, and the Settings page never writes `auth.json`.
- The Runs page and the first load are slow. `GET /api/runs` runs two `git` calls per Run and one `git show` per Ticket, each Ticket scans every Session, every Run change refetches the whole list from both `App.svelte` and `RunsSidebar.svelte`, and boot waits for all lists even on `/quota`.
- Settings is one long page of lists. The Run model fallbacks have no thinking effort and live apart from the role models, icons are inconsistent, and phones have no hub.

## Solution

Five deliverables, built in this order after the Quota spec has merged (they share the Rail, `areas.js` and `App.svelte` with it):

1. **Speed.** Run list summaries, a short git cache, a Session index and a non-blocking boot.
2. **Providers.** A global `/providers` page where an admin connects accounts and adds API keys.
3. **Project scope.** A Project switcher, a Dashboard at `/`, and `/p/<id>/...` URLs.
4. **Draft project pick.** When the first prompt is sent from "No project", jev suggests a Project and a banner asks to confirm.
5. **Settings redesign.** A hub with one page per concern, and one merged Agents page with a model chain per role.

Design references: `.scratch/settings-redesign/mock/index.html` (served at `https://kobo-scintilla-workplace.tailef9ae.ts.net/settings-mock/`) for the shell, Settings hub, Agents, Providers and phone layout; `.scratch/quota-tab/spec.md` for Quota (done there, not repeated here); `.scratch/project-suggest/jev-prompt-experiment.md` for the jev measurements.

## User Stories

Speed

1. As a user, I want the Runs page to open without waiting on `git` for every Run, so that it loads in well under a second.
2. As a user, I want the Runs list to carry summaries only, with Ticket bodies loaded when I open a Run, so that the list stays small.
3. As a user, I want git results cached for a short time, so that repeated list loads do not re-run `git`.
4. As a user, I want Ticket-to-Session lookups served from an index, so that cost does not grow with every Session.
5. As a user, I want the sidebar and the Runs page to share one refetch that coalesces bursts of Run events, so that one change causes one request.
6. As a user, I want the first route to render without waiting for the full Session and Run lists, so that `/quota` and `/runs` open fast on a cold Daemon.
7. As an operator, I want before and after timings recorded for `GET /api/runs` and boot, so that the speed claim is measured.

Providers

8. As an admin, I want a `/providers` page listing every pig provider with its connection status (stored key, environment variable, OAuth, or none), so that I see what is connected.
9. As an admin, I want to paste an API key for any API-key provider, so that I add OpenRouter, OpenCode-Go, Gemini direct, DeepSeek or OpenAI without the CLI.
10. As an admin, I want a stored key never sent back to the browser, only a "set" flag, so that keys cannot leak through the UI.
11. As an admin, I want to remove a stored key, so that I revoke a provider.
12. As an admin, I want ChatGPT to connect with a device code and a verification link, so that login works from a browser on another machine.
13. As an admin, I want Antigravity to connect by pasting the callback URL or code, so that the failing localhost page is not a blocker.
14. As an admin, I want Claude to connect through `claude auth login` run by the Daemon in a pty, with the link shown and the code pasted back, so that the host `~/.claude` login is written by Claude Code itself.
15. As an admin, I want the Claude login written only to `~/.claude` and never to pig, so that the two credentials stay separate.
16. As an admin, I want other OAuth providers to show the exact CLI command (for example `pig login <provider>`) with a copy button, so that nothing is a dead end.
17. As an admin, I want cloud-auth providers (Bedrock, Vertex, Cloudflare) listed as "configure via CLI", so that the list is complete.
18. As an admin, I want a configurable redirect base URL for browser OAuth, off by default and probed per provider, so that I can try a tailnet redirect without changing code.
19. As an admin, I want the card on `/quota` to refresh after a successful connect, so that I see the effect at once.
20. As a non-admin user, I want to see connection status read-only with Connect buttons locked, so that I know the state without changing the host login.
21. As a user, I want a "not connected" Quota card to link to Providers, so that I fix it in one click.
22. As a phone user, I want Providers to follow the phone layout of the mock, so that connect panels work on a small screen.

Project scope

23. As a user, I want one Project switcher instead of a sidebar listing every Project, so that the sidebar stays short.
24. As a user, I want an "All projects" Dashboard at `/` showing running and needs-you Sessions and Runs grouped by Project, so that I see what is active everywhere.
25. As a user, I want the Dashboard to hide idle history, so that it shows only what needs attention.
26. As a user, I want `/p/<projectId>` to scope the sidebar, Sessions and Runs to that Project, so that the URL says where I am.
27. As a user, I want `/p/<projectId>/runs` to list that Project's Runs with no folder to type, so that Runs follow the scope.
28. As a user, I want `/s/<id>` to stay canonical and set the scope from the Session's Project, so that existing links keep working.
29. As a user, I want URLs built from the stable Project id, so that renaming a Project breaks no link.
30. As a user, I want `/p/none` as a built-in scope for Sessions in my home directory, so that they stay reachable and visible on the Dashboard.
31. As a user, I want `/quota`, `/providers`, Memory, Users and account Settings to stay global, so that account pages do not depend on the Project.
32. As a user, I want `/p/<projectId>/settings` to show rename, remove and the editable Project gist, so that a settings page exists for later overrides.
33. As a phone user, I want the Project switcher and Dashboard in the phone drawer, so that scope works on phones.

Draft project pick

34. As a user, I want the first prompt sent from "No project" checked against my Projects before the Session starts, so that the Session starts in the right folder.
35. As a user, I want a banner "Run this in <Project>?" when jev is confident, so that one click puts the Session in that Project's path.
36. As a user, I want no banner when jev is unsure or the prompt is general, so that wrong suggestions stay rare.
37. As a user, I want each Project to have a one-line gist derived from its `AGENTS.md` or README, cached, so that jev has context.
38. As a user, I want to edit a Project's gist on its settings page, with my edit winning over the derived one, so that I correct a bad summary.
39. As a user, I want a Session that is already started never moved by this feature, so that the working directory never changes mid-Session.

Settings

40. As a user, I want `/settings` to be a hub with one page per concern, so that no page is a long list.
41. As a user, I want Workspace pages (Agents, Runs, Context and names, Memory, Users and access) and You pages (Profile, Notifications, Sound and voice), so that settings group by who they affect.
42. As a user, I want a scope pill on each page (Global, Admin, You), so that I know who a change affects.
43. As a user, I want one Agents page where each role has one ordered chain, so that role models and Run fallbacks are no longer separate lists.
44. As a user, I want every chain entry to set harness, model and thinking effort together, so that fallbacks carry an effort like the primary does.
45. As a user, I want entry 1 labelled Primary and the rest Fallback, with reorder, remove and add, and at most three entries, so that the chain is easy to read and edit.
46. As a user, I want the effort choices to list only what the chosen model supports, so that I cannot pick an invalid effort.
47. As a user, I want roles grouped as Session agents (Orchestrator, Subagent, Scout) and Run agents (Ticket worker, Spec reviewer, Standards reviewer), so that I find a role by where it runs.
48. As a user, I want the Runs page to label the cap "Maximum tickets per Run (spec)" and explain it, so that the scope of the cap is clear.
49. As a user, I want the Memory page to show the decider as a segmented control and keep the custom Jev endpoint collapsed (empty means OpenRouter), so that the common path is simple.
50. As a user, I want a save bar that appears on the first change with Revert and Save, so that edits are explicit.
51. As a user, I want saved settings from before the redesign to keep working, so that no role silently loses its model.
52. As a phone user, I want the hub list and a "< Settings" link on each sub-page, so that Settings works on phones.

## Implementation Decisions

Speed

- `GET /api/runs` returns summaries (id, spec, status, counts, repo, Project id, updated time). Ticket bodies and acceptance text load from a per-Run endpoint when the Run opens. This changes the list response shape, so `App.svelte`, `RunsSidebar.svelte` and `RunsView.svelte` change with it.
- A short TTL cache (seconds) holds `git` results per Run and per Ticket, invalidated by the Run's own events.
- The Ticket-to-Session lookup becomes an index built when a Session is attached to a Ticket, replacing the per-Ticket scan of all Sessions.
- The sidebar and the Runs page share one coalesced refetch. A burst of Run events triggers one request.
- Boot no longer awaits the Session and Run lists before the first route renders. Each view loads its own data.
- Measure `GET /api/runs`, Run-list render and cold boot before any change and record the numbers in the first ticket. Success is a recorded improvement, not an assumed one.
- Quota non-blocking reads are owned by the Quota spec and not repeated here.

Providers

- A global `/providers` route and a Providers entry in the Quota second sidebar (the mock groups Quota and Providers under one rail icon). Writes are admin-only; other Users see status read-only.
- The list comes from the pig SDK provider list. Each row reports connected via stored key, environment variable or OAuth. API-key rows write through `AuthStorage.Set` into the host pig `auth.json`; remove deletes the credential. Keys are never returned to the browser.
- Connect flows run as short-lived login sessions in the Daemon:
  - ChatGPT: the pig device-code login. The UI shows the user code and the verification link, then the card refreshes.
  - Antigravity: the existing prompt/callback flow in `pig-antigravity-auth/oauth.go`. The UI takes the pasted callback URL or code.
  - Claude: `claude auth login` in a pty. The UI shows the link and sends back the pasted code. The result lives in `~/.claude` only. Claude OAuth never goes to pig in this spec.
  - OpenCode-Go and other API-key providers: a password field.
  - Other OAuth and cloud-auth providers: the CLI command with a copy button, no flow.
- A redirect base URL is a config setting (a settings file key, default empty). When set, it is used only for providers where a probe shows the redirect is accepted. It is never the default. Providers register their own redirect URIs, so a tailnet redirect may be rejected; the probe decides.
- The mock groups rows as subscription logins (Claude, ChatGPT, Gemini via Antigravity), API keys (OpenRouter, OpenCode-Go, Gemini direct, DeepSeek, OpenAI) and CLI-only providers. Providers with no usage endpoint show connection status only.
- Providers stays a separate spec from Quota: `/quota` is read-only.

Project scope

- URL table: `/` Dashboard; `/p/<id>` Project scope; `/p/<id>/runs` Project Runs; `/p/<id>/settings` settings shell; `/p/none` home-directory Sessions; `/s/<id>` canonical Session URL that sets the scope; `/quota`, `/providers`, Memory, Users and account Settings global.
- A Run belongs to a Project through its repo path (deepest containing Project, as `owners.js` and the Daemon already map it). Memory stays one Bank per User.
- The Dashboard lists only running or needs-you Sessions and Runs, grouped by Project. Idle history stays in each Project view.
- The Project switcher has an "All projects" entry (the Dashboard) and filters the sidebar, Sessions and Runs. It replaces the per-Project sidebar sections.
- `/p/<id>/settings` ships as a shell: rename, remove (existing behavior) and the editable gist. No override fields.
- Project gist: derived from `AGENTS.md` or README, cached, stored with the Project; an edited gist wins and is never overwritten by the derivation.
- GLOSSARY.md: add Provider, Dashboard and quota percent next to the existing Project entry.

Draft project pick

- Trigger: the first prompt sent from a draft with no Project, before the Session starts. The Daemon asks jev (OpenRouter `systemone`, model `typesafe/jev-1.13`, client `emberloop/tools/systemone/systemone.go`, key from `systemone.APIKey()`) one `noul` question per Project plus one "none" question, in one call. No new key setting is needed.
- Wording per Project: name plus its one-line gist. Measured on 25 prompts and 4 Projects: names only 64% top-1; name plus path 64%; name plus gist 92% top-1 with 0% false picks on "none" prompts. Paths add nothing.
- Gate: accept only if top probability is at least 0.5 and the lead over the second is at least 0.2. Measured: 100% correct on 80% coverage. Misses fall to "none", which is the safe direction; below the gate no banner appears. Latency is about 300ms per call, so the banner can appear before the first turn.
- `memory.ValidateQuestions` allows only the five miner questions, so a project-pick question needs its own question type or a direct wire call. Thresholds are valid for `typesafe/jev-1.13` and must be re-measured if the model changes.
- Yes on the banner creates the Session with that Project's path. The Session never changes working directory afterwards.
- Without an OpenRouter key, no suggestion is made and the draft works as today.
- The existing live probe `emberloop-web/jev_projectpick_live_test.go` stays gated behind environment variables and out of the check.

Settings

- `/settings` becomes a hub. Workspace: Agents, Runs, Context and names, Memory, Users and access. You: Profile, Notifications, Sound and voice. Providers and Quota keep their own pages. Each page has its own URL; on phones `/settings` shows the hub list and each page a "< Settings" link.
- Agents: role rows and Run Chains merge into one chain per role. Each entry has harness, model and effort. The mock uses Default, Low, Medium, High and Max; the real page lists only the efforts each model supports, which must be checked per model. Entry 1 is Primary; others are Fallback 1 and 2; maximum three entries; reorder, remove and add.
- The mock renames roles: Worker becomes Subagent and Implementer becomes Ticket worker. The saved settings keys must keep working. A migration from the old role and chain keys to the merged form is part of the Agents ticket and ships with a test on a pre-redesign settings file.
- Runs page: "Maximum tickets per Run (spec)". `max_tickets` stays global in Settings, caps each Run, and a spec value can only lower it. There is no Daemon-wide total cap and no per-Project cap.
- Memory page: decider as a segmented control; custom Jev endpoint collapsed, empty meaning OpenRouter.
- Shell and colors follow the mock inside the real shell: rail, second sidebar, main; tokens only from `src/app.css`. Pills, white primary buttons, blue `#0a84ff` for focus only; 44px touch targets and 16px inputs on phones.
- Docs: update `emberloop-web/AGENTS.md`, `emberloop-web/web/README.md`, `GLOSSARY.md`, `.scratch/settings-redesign/AGENTS.md` and the root index.

## Testing Decisions

- Test external behavior only: HTTP shapes, live payloads, rendered text, route results.
- Speed: Go tests at the `/api/runs` seam asserting the summary shape, the per-Run detail endpoint, git cache hits and invalidation, and the index; a bounded-time test on a fixture with many Runs. JS tests for the coalesced refetch and a boot that renders before the lists arrive. Prior art: `runs_test.go`, `App.test.js`.
- Providers: Go tests with local stubs for each login flow and a temp pig dir. Cover the provider list and status sources, key set and remove (and that no response contains a key), admin-only writes, the device-code, paste and pty flows (a fake `claude` for the pty), and the redirect-base probe. JS tests for the page states and phone layout.
- Project scope: JS router tests for every URL in the table, scope from `/s/<id>`, `/p/none`, and rename-stable links. Go tests for the Dashboard payload (running and needs-you only) and the gist derivation and edit precedence.
- Draft pick: Go tests with a stubbed `systemone` endpoint covering the gate (accept, below top, below margin, none), wording per Project, no key, and a Session that starts in the chosen path. JS tests for the banner.
- Settings: vitest on the hub routes and each page; chain editing with effort per model; the old-to-new settings migration on a fixture; the save bar.
- No live tests in the check; the gated jev probe stays under the test file's environment guard.

## Out of Scope

- Per-Project overrides of roles, Chains, caps or `run_skills`. `/p/<id>/settings` is a shell only.
- Per-Project caps, a Daemon-wide `max_tickets_total`, and per-Project Rules.
- Moving an already-started Session to another Project, and an agent tool that changes the working directory mid-Session (with `AGENTS.md` injection). Revisit after seeing how often the draft pick misses.
- Per-User provider accounts; connecting pig's own Anthropic login as a second Claude row.
- Running two specs in the same Project in parallel (to be grilled in a later session).
- Quota page work (see `.scratch/quota-tab/spec.md`).

## Further Notes

- Order: Speed, then Providers, then Project scope, then Draft pick. The Settings hub can run in parallel with Speed after the Quota spec merges, but not with Project scope, since both rewrite the Rail and `App.svelte`.
- Delivery risk: this spec and the Quota spec both edit the Rail, `areas.js` and `App.svelte`. Run them one after the other, not together.
- Facts to carry: pig has the ChatGPT and Antigravity logins (the earlier claim that it did not was wrong). Host Claude login has `user:profile`; the setup-token does not.
- Known gap: the no-prompt way to refresh an expired host Claude login is not found. Quota shows "login expired" and Providers' Claude Connect covers it.
