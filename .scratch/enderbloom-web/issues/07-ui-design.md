# 07: Design pass with shadcn-svelte

**What to build:** Replace the unstyled UI with the designed one: session tree, single-column transcript of cards (messages, tool calls with collapsible output, plan checklist, diffs, errors), floating composer, collapsible details pane, new-Session dialog with harness, model and working directory. shadcn-svelte on Tailwind, a ChatGPT-and-Apple look, system light or dark theme, reconnect indicator.

**Blocked by:** 02

**Status:** ready-for-agent

- [ ] Every Event kind renders as a card
- [ ] Tool output is collapsed by default and expandable
- [ ] The new-Session dialog selects harness, model and working directory, with models filtered by harness
- [ ] The UI follows the system light or dark theme
- [ ] Status per Session is visible in the list
- [ ] The build is a static bundle with a configurable API base URL and no server rendering
- [ ] Component tests cover composer mode and Event-to-card mapping
