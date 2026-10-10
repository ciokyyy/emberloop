# 12: Docs page, glossary and AGENTS updates

**What to build:** Agents inside Emberloop learn to build an Emberplugin from `el://docs/plugins`. The page covers the manifest format, permissions, the kit and templates, the API and tokens, storage, jobs, the server env-var contract, the CLI, logs, RAM rules, and a worked lead scraper example. `GLOSSARY.md` gets the **Emberplugin** term. The owning `AGENTS.md` and README files describe the new area.

**Blocked by:** 07 (Jobs and scheduler), 08 (Server process supervisor), 10 (Notify), 11 (CLI and shared toolchain)

**Status:** ready-for-agent

- [ ] `docs/agent/plugins.md` written; `el update` refreshes it like the other agent pages
- [ ] The page tells agents to use Go and Svelte, the kit and the page templates
- [ ] The worked example runs end to end when followed
- [ ] `GLOSSARY.md` has an **Emberplugin** entry with its avoided words
- [ ] `emberloop-web/AGENTS.md` and `web/README.md` describe plugins, the kit and the design rules applying to it
- [ ] Root and area `AGENTS.md` indexes are refreshed and resolve
- [ ] The repo check passes
