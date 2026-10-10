# 06: Storage

**What to build:** An Emberplugin keeps data without running a server. The Daemon offers per-plugin KV and a SQLite file in the plugin's data dir through the API, gated by a storage permission. A plugin page can save and load data, and the data survives a Daemon restart.

**Blocked by:** 03 (Token, permissions and LLM call)

**Status:** ready-for-agent

- [ ] KV get, put, delete and list routes, scoped to the calling plugin
- [ ] A SQLite database per plugin, reachable through a query route or a documented file path for plugin processes
- [ ] One plugin cannot read or write another plugin's data
- [ ] Deleting a plugin from the UI asks whether to keep or remove its data
- [ ] Tests cover scoping, persistence across restart, and permission refusal
