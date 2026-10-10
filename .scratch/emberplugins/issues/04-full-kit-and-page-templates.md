# 04: Full shadcn-svelte kit and page templates

**What to build:** The kit ships every shadcn-svelte component, styled in Emberloop's own way (ChatGPT plus Apple look, smooth motion), plus three page templates: dashboard, table plus detail, and form plus results. An agent composes a plugin page from these blocks and builds very little itself. A kit demo plugin shows every component in both themes.

**Blocked by:** 02 (Iframe on its own origin and kit v0)

**Status:** ready-for-agent

- [ ] Every shadcn-svelte component is available from the kit with Emberloop styling, following `web/README.md` "Design rules"
- [ ] The three page templates exist and render with sample data
- [ ] A demo plugin shows all components and templates in both themes
- [ ] The kit is versioned with Emberloop and served at `/plugin-kit/`
- [ ] Vitest covers each template and a sample of components
