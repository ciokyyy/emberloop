# 02: Pig summarizer and compress model setting

**What to build:** The Handoff is written by a one-off pig call with no tools, using the pinned recipe (Pocock handoff sections, caveman ultra). The user picks `compressModel` in the WebUI settings from the authed pig catalog. Bad output walks the fallback chain: repair retries, lenient wrap, then the deterministic Handoff from ticket 01.

**Blocked by:** 01

**Status:** ready-for-agent

- [ ] The WebUI settings panel has a compress model choice limited to authed pig models
- [ ] With the model unset, or no pig auth, Compress is disabled with a reason
- [ ] The summarizer runs as a one-off with no tools, outside the Session
- [ ] The existing `compress_instructions` setting is added to the summarizer prompt
- [ ] The last user prompt appears verbatim in the Handoff; an open task gets a continue line
- [ ] Empty or wrong-format output triggers up to 2 repair retries with a prompt that restates the sections
- [ ] After the retries, non-empty text is wrapped under the required headings
- [ ] After that, the deterministic Handoff is used; the `compressed` Event names the step used
- [ ] Tests with a fake summarizer cover good, empty and wrong-format output
