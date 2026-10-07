# 06: agy stop gate

**What to build:** An agy agent spawned by emberloop cannot end without calling `submit`, as firmly as agy allows. If the probe found a hook that can block a stop or inject a message, it runs emberloop's stop-gate decision (block, remind, give up) like claude's Stop hook. Otherwise agy uses the nudge-and-give-up path the stop gate already has for weaker harnesses. agy's capability is declared in the existing stop-gate capability table.

**Blocked by:** 04

**Status:** wontfix (superseded: agy is a pig provider, see spec.md)

- [ ] agy row in the stop-gate capability test
- [ ] Gate wiring in the generated agent definition (or nudge path) covered by tests mirroring the claude stop-gate tests
- [ ] Opt-in live test: an agy worker that tries to stop without submitting is reminded, then gives up with a notice after the configured attempts
