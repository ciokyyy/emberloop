# 04: agy harness: stripped agent, MCP, models, transcript

**What to build:** `agy` is a third harness any role can use. emberloop generates a stripped custom agent per role and tool set under `~/.emberloop/agy/` (or the probe's fallback location): no default components, only emberloop's MCP server, emberloop's system prompt. The harness implements the full harness interface: command (launched through `emberloop exec`), session root, last reply, turn state, trust, late prompt. `Models()` parses `agy models` and groups effort-suffixed ids into one base model with those efforts; ungrouped models get `--effort` levels. At launch a grouped model is sent as `<base>-<effort>`, otherwise `--model` plus `--effort`.

**Blocked by:** 02, 03

**Status:** wontfix (superseded: agy is a pig provider, see spec.md)

- [ ] `agy` in the harness registry; every-harness command tests pass for it
- [ ] Stripped-agy test: agent definition path, no default components, only emberloop MCP, emberloop system prompt
- [ ] Agent definitions written without races between concurrent sessions
- [ ] User's global agy config never modified
- [ ] Last reply and turn state tested against probe fixtures
- [ ] `Models()` grouping and launch model/effort tested against captured `agy models` output
- [ ] Opt-in live test: an agy scout answers a question through emberloop's tools
