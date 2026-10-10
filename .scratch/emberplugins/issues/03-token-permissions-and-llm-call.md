# 03: Token, permissions and LLM call

**What to build:** An Emberplugin page can call the Daemon API with a per-plugin token and run a one-shot LLM completion (no Session) on the user's harness and model. The Daemon checks the manifest permission groups on every route the token reaches. The default grant is read plus LLM; groups the manifest does not declare are refused with a clear error. The fixture plugin proves it end to end with the fake Backend.

**Blocked by:** 02 (Iframe on its own origin and kit v0)

**Status:** ready-for-agent

- [ ] Each plugin gets its own token, delivered to its page and never to another plugin
- [ ] A permission middleware maps routes to permission groups (`llm`, `sessions`, `runs`, `files`, `sudo`, `notify`, storage) and refuses undeclared groups with 403
- [ ] A one-shot LLM route returns a completion without creating a Session
- [ ] The kit API client wraps token and base URL
- [ ] Tests cover: token for plugin A refused on plugin B's routes, undeclared group 403, LLM call success on the fake Backend, a disabled plugin's token refused
