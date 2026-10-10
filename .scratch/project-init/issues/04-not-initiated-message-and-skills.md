# 04: Not-initiated message, skills and Runs discovery

**What to build:** Tools that need spec or ticket paths return "project not initiated: ask the user, then run `el init`" once per Session when the settings are missing, and read the settings fresh on every call. The Matt skills (`to-spec`, `to-tickets`, `implement`, `wayfinder`) read `specs` and `tickets` from the settings. The Runs view lists specs from the `specs` directory when it is set.

**Blocked by:** 02

**Status:** todo

- [ ] A tool call without settings returns the message the first time per Session and not again
- [ ] The same Session reacts on the next call after the file appears
- [ ] The four skills read the settings and keep a fallback only for `pipeline: false` projects
- [ ] `discoverSpecs` uses `specs` when set and keeps the current scan otherwise
- [ ] Tests cover the once-per-Session message and the discovery switch
