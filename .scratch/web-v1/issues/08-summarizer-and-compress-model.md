# 08: Pig summarizer and compress model setting

**What to build:** The Handoff is written by a one-off pig call with no tools, using the pinned recipe (Pocock handoff sections, caveman ultra). The user picks `compressModel` in the WebUI settings from the authed pig catalog. Bad output walks the fallback chain: repair retries, lenient wrap, deterministic Handoff. The Handoff writer drops fenced memory blocks.

**Blocked by:** 05

**Status:** done

- [x] The settings panel has a compress model choice limited to authed pig models
- [x] With the model unset, or no pig auth, Compress is disabled with a reason
- [x] The summarizer runs as a one-off with no tools, outside the Session
- [x] `compress_instructions` is added to the summarizer prompt
- [x] The last user prompt appears verbatim; an open task gets a continue line; fenced memory blocks are dropped
- [x] Empty or wrong-format output triggers up to 2 repair retries that restate the sections
- [x] After the retries, non-empty text is wrapped under the required headings, then the deterministic Handoff is used; the `compressed` Event names the step
- [x] Tests with a fake summarizer cover good, empty and wrong-format output
- [x] Owning `AGENTS.md` files and root index updated; repo check passes
