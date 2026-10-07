# 05: agy reports real state to herdr; emberloop tools never background

**What to build:** An agy pane shows working while the agent works and idle only when its turn ends, regardless of what's on screen. The generated agent's hooks call `herdr pane report-agent` (working/idle) and `release-agent` at exit, like the herdr-report extension does for pig. emberloop's MCP tools are configured with `background_mode: off` (wherever the probe found it goes).

**Blocked by:** 04

**Status:** wontfix (superseded: agy is a pig provider, see spec.md)

- [ ] Hooks present in the generated agent definition and reporting through herdr
- [ ] Every emberloop tool has background mode off
- [ ] Opt-in live test: an agy worker running a long tool call is reported working by herdr throughout, and `Wait`/state checks don't see it as settled
