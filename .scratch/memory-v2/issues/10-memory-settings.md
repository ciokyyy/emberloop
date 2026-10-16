# 10: Memory settings

**What to build:** Settings → Memory, admin-only and host-global, saved through the Revert/Save bar: `memory_extract_model` (default `inclusionai/ling-3.0-flash`), `memory_extract_effort` (`off`, `low`, `medium`; default `low`), `memory_embedding_model` (default from ticket 04), `memory_recall_weights`, and an automatic-extraction switch (default on when an OpenRouter key exists, off otherwise). The page states that redacted user turns leave the host for the extractor and the embedder. Update the findsettings registry so the settings are searchable.

**Blocked by:** 02, 04, 06

**Status:** done

- [x] A non-admin cannot read or write the settings
- [x] The switch default follows OpenRouter key presence
- [x] Weights outside 0 to 1 or summing to 0 are rejected
- [x] Changes take effect without a Daemon restart
- [x] Settings search finds each new control
- [x] Owning `AGENTS.md` files and root index updated; repo check passes
