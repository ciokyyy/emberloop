# 23: Reviewer submit live probe follow-up

**What to build:** Diagnose why the neutral real Pig spec Reviewer in ticket 21's acceptance run did not emit a typed `submit` Event. Verify the tool loadout and Session Event log, then fix the product or probe if needed. Re-run once with a neutral fixture and a tool-capable real Pig model; the fixture must not prescribe the verdict or instruct the Reviewer to use a particular tool.

**Blocked by:** 02-reviewer-submit-tool, 21-live-probes

**Status:** ready-for-agent

- [ ] Record the root cause of the missing typed `submit` Event in run `16e46102db827c573ee056012ef3f047`.
- [ ] If a product or probe change is needed, implement it and add appropriate offline coverage.
- [ ] Make one neutral-fixture live attempt; record the Pig model, Run ID, typed Event and whether its verdict is fresh at the current tip. Keep the issue open if behavior remains unproven.
- [ ] Keep the live probe out of `.emberloop/check`.

Ticket 21's later exploratory retries are not acceptance evidence: they used different models and included a fixture that explicitly prescribed `submit` with `pass`. This ticket owns an unbiased follow-up attempt.

A later exploratory Run `8d41e16e41f3fad57d0a3abc05dcde14` with Pig `openrouter/google/gemini-2.5-flash` emitted a typed `pass` at tip `f4003264c3d504668021fb9f81a702bd1accb21b`, but only after the fixture explicitly required calling `submit` with `pass`; it is not acceptance evidence. The neutral retry is still required.