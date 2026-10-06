# 13: Installing wick refuses while a run has live agents

**What to build:** Wick binaries are installed through one command. It refuses while any agent of a run is live (worker, reviewer or scout panes known to the stores and the terminal multiplexer), names them, and changes nothing. A force option installs anyway and prints that running sessions keep the old binary until they restart. Nothing replaces a binary any other way, and the project's docs say so.

**Blocked by:** None (can start immediately)

**Status:** done

- [x] With a live agent in the stores and the fake multiplexer, install exits non-zero, names the agent and replaces nothing
- [x] With none it installs
- [x] Force installs and prints the warning
- [x] Spec stories 40, 41, 42

Spec: `.scratch/retro-hardening/spec.md`. Read it for the decisions this ticket implements, and its Hard limits section, which binds you.
