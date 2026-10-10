# 02: Iframe on its own origin and kit v0

**What to build:** Opening an Emberplugin page renders its built `dist/` in a sandboxed iframe. The Daemon serves each plugin on its own origin, and the iframe omits `allow-same-origin` toward the main UI. The page mounts only while open and unmounts when the user leaves. The Daemon serves a first `/plugin-kit/` with design tokens, base CSS and theme sync, so a fixture plugin looks native and follows dark and light theme.

**Blocked by:** 01 (Registry and sidebar entry)

**Status:** ready-for-agent

- [ ] Each plugin is served from its own origin; a plugin page cannot read the main UI's origin state
- [ ] Iframe mounts on open and unmounts on leave; no plugin work stays in the browser after leaving
- [ ] `/plugin-kit/` serves tokens, base CSS and a theme-sync script
- [ ] Theme changes in the host reach the iframe live
- [ ] Fixture plugin renders with kit styles in both themes
- [ ] API-seam tests cover plugin asset serving, content types, unknown plugin 404 and disabled plugin refusal
- [ ] Vitest covers mount and unmount
