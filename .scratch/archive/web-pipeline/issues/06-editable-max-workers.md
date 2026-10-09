# 06: Editable max_workers in settings

**What to build:** The settings endpoint reads and writes max_workers next to roles. A write changes the live Session cap on the running Daemon with no restart. The environment variable still wins; when it sets the value, the read response says so and the settings page shows the field locked with a note. Run Sessions, Reviewers included, share the one cap with chat Sessions.

**Blocked by:** 02

**Status:** done

- [x] The settings read returns max_workers and whether the environment overrides it
- [x] The settings write validates and saves max_workers without losing roles
- [x] A write changes the live cap; over-cap behaviour is unchanged
- [x] The settings page edits the field, and locks it with a note under an environment override
- [x] API and view tests cover edit, validation and the locked state
