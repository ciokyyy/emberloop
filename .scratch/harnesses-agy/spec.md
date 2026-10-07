# Spec: agy as a pig provider, and MCP tool calls that stay in the foreground

Status: partly delivered (see Delivered)

## Problem Statement

I have an Antigravity subscription and want its quota used, instead of letting it sit unused while my Claude quota runs down. The first plan drove the `agy` CLI as a third emberloop harness. It was dropped: agy backgrounds tool calls, herdr's screen detection decides the agent finished while it still works, and agy can't be controlled the way pig and claude can (hooks, stop gate, transcript, usage all needed their own workaround). omp already talks to Antigravity's Cloud Code Assist endpoints directly. pig can do the same through a provider extension, and then every pig feature (settings, stop gate, transcripts, usage logs, herdr reporting) works unchanged.

Separately, claude moves a running MCP call to the background after 120 seconds, or as soon as text is typed into its pane. That breaks emberloop, because `spawn` and other long calls are MCP calls, and notice delivery types into the pane.

## Solution

agy is a model provider for pig, not a harness. The extension `pig-antigravity-auth` registers the `google-antigravity` provider: Google OAuth login, token refresh, project discovery, and a streaming client for the Cloud Code Assist endpoint. It lists one entry per model; reasoning effort is pig's own thinking level, which the extension maps to Antigravity's variants. For claude, the harness turns background tasks off, so MCP calls always stay in the foreground.

## Delivered

- Claude keeps MCP calls in the foreground (`CLAUDE_CODE_DISABLE_BACKGROUND_TASKS=1`, `CLAUDE_CODE_MCP_AUTO_BACKGROUND_MS=0`): tickets 01.
- Agents receive their prompt directly through `emberloop exec` launch files: ticket 02 (user story 15 of the old spec).
- A harness override drops the other harness's model and effort: ticket 08.
- agy provider for pig: `pig-antigravity-auth/` (Go extension, no ticket). Login, refresh, model list, streaming, tool calls, usage, tests.
- Ticket 03 probe of the agy CLI: kept as a record in `testdata/agy/` and its ticket. Not built on.

## Implementation Decisions

- The extension is a port of `@cortexkit/pi-antigravity-auth` (pi) and omp's Antigravity OAuth, using PiG's Go SDK provider hooks: `RegisterProvider` with an `*OAuthProvider` and a `streamSimple` handler.
- Catalog is the public Antigravity text-model list, one entry per base model (`antigravity-gemini-3.1-pro`, `-3.5/3.6/3.7/3.8-flash`, Claude Sonnet and Opus 4.6 thinking, GPT-OSS 120B). Pig's thinking level selects the wire variant (`-low`, `-medium`, `-high`, `gemini-pro-agent`) and thinking budget; Claude and GPT-OSS ignore the level.
- Login is the paste flow: a consent URL is shown, the callback URL or code is pasted back. The access token and project id travel to the stream handler as one JSON api key.
- Requests carry the agy CLI identity (user agent, request id, labels, session id), as the pi extension does.
- Usage reaches emberloop through pig's own transcript and the run-records pig parser; no agy-specific parser exists.

## Testing Decisions

- Unit tests in the extension: model resolution, schema and message conversion, OAuth URL and callback parsing, refresh, request envelope, streaming events, errors, session identity.
- Verified with pig: `pig install --validate-only`, `pig login --list`, `pig --list-models`.
- Claude: the stripped-claude test asserts both environment variables. Launch tests assert the `emberloop exec` line and launch file contents.

## Out of Scope

- Driving the agy CLI as a harness (dropped; see `archive/spec-agy-cli-harness.md` and `archive/04-agy-harness-wip.patch`, plus branch `archive/agy-usage-parser`).
- A local OAuth callback server for login (paste only for now).
- Multi-account rotation and quota display.
- Matching the agy CLI's exact HTTP header order (standard Go HTTP client).
- Web search tool: needs its own grill session.

## Further Notes

Terms: using Antigravity this way breaks Google's terms of service and can get the account suspended. Spec 2 of 5; the other specs no longer depend on an agy harness.
