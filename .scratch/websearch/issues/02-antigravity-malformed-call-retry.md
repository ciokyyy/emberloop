# 02: Retry Antigravity turns that end in a malformed function call

**What to build:** When an Antigravity model's turn ends with `MALFORMED_FUNCTION_CALL` (or an equivalent invalid-tool-call finish reason) and nothing has been emitted yet, the provider re-sends the same request up to 2 more times with a short backoff, so one bad tool call no longer kills a reviewer run. If every attempt fails, the error names the finish reason and shows the provider's finish message instead of "returned an empty response". A genuinely empty `STOP` keeps the existing error. No retry once any content or tool call was emitted for the turn.

**Blocked by:** None (can start immediately)

**Status:** done

- [x] Malformed call then good response succeeds, with one stream of events to the agent
- [x] Three malformed responses give the specific error including the finish message
- [x] Empty `STOP` still gives "empty response"
- [x] No retry after partial content was emitted
- [x] The session is marked complete only on the successful attempt
- [x] Fixture-based tests, no live calls; the extension's tests and the repo check pass
