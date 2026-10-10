# 01: Registry and sidebar entry

**What to build:** A user (or an agent in a Session) drops an Emberplugin folder with a manifest into the global plugin directory under the state dir. The Daemon notices it without restart and the UI sidebar shows a new entry that opens an empty plugin page. Disabling or deleting a plugin from the UI removes it from the sidebar, and the enabled state persists across a Daemon restart. A bad manifest shows as an error row, never crashes the Daemon.

**Blocked by:** None (can start immediately).

**Status:** ready-for-agent

- [ ] Manifest fields parsed and validated: name, title, icon, sidebar entry, permissions (declared only), optional config fields
- [ ] A watcher picks up new, changed and removed plugin folders with no Daemon restart
- [ ] `GET /api/plugins` lists plugins with enabled state and any manifest error
- [ ] Enable, disable and delete work through the API and the UI, and persist across restart
- [ ] The sidebar entry opens an empty plugin page
- [ ] API-seam tests with a temp plugin dir cover add, change, remove, bad manifest and restart reload
- [ ] Vitest covers the sidebar entry and plugin list view
