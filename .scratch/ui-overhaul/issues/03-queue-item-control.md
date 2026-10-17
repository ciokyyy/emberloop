# 03: Per-message queue control

**Type:** code

**What to build:** The Daemon lists queued messages with stable ids, replaces one item's text, and removes one item. Today only `DELETE /api/sessions/:id/queue` clears everything. Both the Daemon pending queue and pig's native queue honour the new calls. A delivered item returns 409. A claude Steer that degraded to a Follow-up reports the mode actually used. Client wrappers in `lib/api.js`. Re-read `daemon.go` (`queuedMsg`, `QueuedMessages`, `ClearQueued`, `appendPromptEvent`), `event.go` (`QueuedMessage`), `ui.go` route table, `pig.go` queue seam.

**Blocked by:** none

**Status:** open

- [ ] `GET .../queue` returns items with `id`, `mode`, `text`, `imageIds`
- [ ] `PATCH .../queue/<id>` replaces text; `DELETE .../queue/<id>` removes one; other items keep order
- [ ] Delivered or unknown id returns 409 or 404 with a clear message
- [ ] Pig native queue and Daemon pending queue both covered; claude degrade reports `followup`
- [ ] Owner and admin rules match other Session actions
- [ ] Daemon tests in `steer_test.go`/`api_test.go` with the fake Backend; `emberloop-web/AGENTS.md` states the contract; `.emberloop/check` passes
