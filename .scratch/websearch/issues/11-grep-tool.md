# 11: `grep` tool

**What to build:** A native `grep` tool in the tool layer: ripgrep (`rg`, resolved from the emberloop bin directory then `PATH`) when available, a Go fallback otherwise. Arguments: `pattern` (required), `fixed` (bool), `path`, `glob`, `context` (lines), `max_results` (default 100), `ignore_case`. Respects `.gitignore`, skips binary files, confined to the working directory like `find`. Output is `path:line:text` lines with a total count and a "more results" note when capped. It is `ReadOnly`. Its default placement is `execute` (ticket 10), so it is reachable as `tools.grep(...)` once ticket 16 lands; until then it is registered but listed only if settings place it in `main`. Large output passes through the overflow helper once ticket 09 exists (a no-op until then).

**Blocked by:** None (can start immediately)

**Status:** done

- [x] Regex and fixed-string search, glob, context lines, case folding and the result cap work against a fixture tree
- [x] `.gitignore` is respected; binary files are skipped
- [x] Works with and without `rg` installed (the fallback gives the same results on the fixtures)
- [x] Paths outside the working directory are refused
- [x] Tool is registered, read-only, with a schema that fits the existing `Tool` interface; repo check passes
