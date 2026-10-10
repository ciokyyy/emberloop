# 03: Validate spec files on write and at turn end

**What to build:** Append `ValidateSpec` warnings to the `write` and `edit` tool results for paths under the specs directory. The warning never blocks the write. At turn end, re-validate the spec files edited in the turn. On errors, re-prompt once with the list. If errors remain, move the Session to needs-you. `"pipeline": false` turns all of it off.

**Blocked by:** 01, 02

**Status:** todo

- [ ] A bad ticket written under the specs directory returns a warning naming file and line, and the file is still written
- [ ] A write outside the specs directory gets no warning
- [ ] A turn that edited a bad spec file gets exactly one re-prompt with the errors
- [ ] Errors left after the re-prompt move the Session to needs-you
- [ ] Cross-file errors (cycle, unknown blocker, duplicate number) surface at turn end
- [ ] `"pipeline": false` silences the warning and the gate
- [ ] API tests with the fake Backend cover each case
