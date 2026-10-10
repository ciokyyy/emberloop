# 05: Plugin Sessions

**What to build:** An Emberplugin with the `sessions` permission creates and drives full Sessions and Subagents on the user's harnesses, and reads Events, Runs, models and quota. Every Session it starts appears in the main Session list tagged `plugin:<name>`, is stoppable from the main UI, and is read-only to the plugin after it finishes. Nothing a plugin runs is hidden.

**Blocked by:** 03 (Token, permissions and LLM call)

**Status:** ready-for-agent

- [ ] Plugin token creates a Session, sends prompts, reads Events and spawns Subagents through the normal routes
- [ ] The Session meta records the plugin tag and the UI list shows `plugin:<name>`
- [ ] The main UI can stop or kill a plugin Session
- [ ] A finished plugin Session refuses plugin writes
- [ ] Plugin Sessions share the `max_workers` cap; over-cap is 429 as today
- [ ] Read routes for Runs, models and quota work under the read grant
- [ ] API-seam tests cover create, tag, stop, over-cap and read-only after finish
