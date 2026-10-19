# 13: Per-area position, Project last Session, instant lists and prefetch

**Type:** code

**What to build:** Each area (Chats, Runs, Quota, Memory, Settings) remembers its last position and restores it on return. Entering a Project opens its last Session. Lists render their last payload at once and refresh behind it. Rows prefetch on hover or touchstart. Back and forward follow one rule on every area, with Back closing an open sheet first. Route shape stays `/p/<id>/...`.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] Switching away from an area and back restores its position
- [ ] Entering a Project opens its last Session; with none it opens the Project page
- [ ] A revisited list shows its last payload before the refresh lands
- [ ] Back and forward tests cover every area and the sheet rule
- [ ] `.emberloop/check` passes

Spec: `../spec.md`.
