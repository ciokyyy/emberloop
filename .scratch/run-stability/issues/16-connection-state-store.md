# 16: One connection state

**What to build:** One connection state for the whole UI: ok, reconnecting or unreachable. It derives from the live stream and failed refreshes, clears on any successful request or stream hello, and reports unreachable after the Daemon has not answered for 15 seconds, while background retry continues. The per-view connection copies are removed: the App "Daemon unreachable" error, the Ticket view reconnecting line and the Runs view reconnecting line. Errors that are not about connectivity keep their own display. The header status glyph stays.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] A failed Session list refresh moves the state to reconnecting, and a later success returns it to ok
- [ ] The state reaches unreachable after 15 seconds without an answer, and clears on the next success or stream hello
- [ ] The three per-view connection messages are gone and nothing else shows "Daemon unreachable" in the shell
- [ ] Non-connectivity errors such as a chat that could not start still show
- [ ] `npm test`, `npm run build` pass; `.emberloop/check` passes
