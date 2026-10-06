# 09: Overflow store and `ctx_search`

**What to build:** Large tool output is stored instead of flooding context. In the database from ticket 03, add an FTS5 table (porter tokenizer) and a `ctx_blobs` table (id, session, source label, created at, content type, bytes). Content is chunked by markdown heading and blank-line blocks, code blocks kept whole, headings weighted higher, chunks carrying line ranges. A shared helper takes a tool result: at or under `overflow.threshold_bytes` (setting, default about 8KB) it passes through unchanged; above it, it stores the content and returns a short head and tail, total size and line count, a handle (`ctx:<id>`) and a list of searchable terms. Wire it into `bash` (replacing the 30KB truncation) and URL `read` (replacing ticket 04's truncate-and-save-a-file). Add the `ctx_search` tool: `queries` (one or more strings), optional `source` handle, `limit` per query (default 3), returning BM25-ranked snippets focused on the match with handle and line range, and a plain "nothing matched" otherwise. `read` accepts a handle with a line range (`ctx:12:50-100`). Rows older than 24 hours are purged on database open and by `prune`. `ctx_search` is offered to every role that can read. The helper takes an optional `intent` so ticket 16 can return matching chunks inline.

**Blocked by:** 03, 04

**Status:** done

- [x] Output under the threshold is byte-identical to before; over it, a handle and summary are returned
- [x] Chunking keeps fenced code blocks whole; line ranges are correct
- [x] BM25 ranks an exact-term chunk first; porter stemming matches `caching` and `cached`
- [x] `ctx_search` takes several queries, scopes to a handle, and says plainly when nothing matches
- [x] `read` of `ctx:<id>:<range>` pages stored output
- [x] `bash` and URL `read` use the store above the threshold; ticket 04's file-save path is removed
- [x] Purge after 24 hours on open and by `prune`; the threshold setting is honoured
- [x] Tests cover the above; repo check passes
