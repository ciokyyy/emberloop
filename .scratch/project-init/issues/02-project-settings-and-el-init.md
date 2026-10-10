# 02: Project settings and `el init`

**What to build:** Read `specs`, `tickets` and `pipeline` from `.emberloop/settings.json`, with the project file overriding `~/.emberloop/settings.json` per key. Add `el init --specs <dir> --tickets <name>`: it validates the values, merges them into the project file and keeps all other keys. Reads happen fresh on every call.

**Blocked by:** None

**Status:** todo

- [ ] The project file overrides the global file per key, and unknown keys round-trip
- [ ] `el init` refuses a missing or non-directory `--specs` and a `--tickets` that is not a plain folder name
- [ ] `el init` keeps `orchestrator_paths` and every other existing key
- [ ] A change to the file shows on the next read with no restart
- [ ] The `el` usage header lists `el init`
- [ ] Tests use a temp project directory and check the written bytes
