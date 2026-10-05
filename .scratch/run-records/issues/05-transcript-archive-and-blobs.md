# 05: Transcript archive and prompt/tool-definition blobs

**What to build:** Each agent's harness transcript is copied into wick's archive under its run, kept current while the agent runs and finalised when it settles, so it outlives harness cleanup. At session start, the system prompt, the tool definitions served for its role, and the harness flags are stored once by content hash in a blob store; the session's archive references the hashes.

**Blocked by:** 01

**Status:** done

- [x] Archive copy tracks a growing transcript and resumes from its offset after restart
- [x] Same prompt or tool set across sessions → one blob, many references
- [x] Archive layout documented in this ticket for the stats indexer
- [x] Tests: fake transcript appended in steps; blob dedup

## Archive layout (for the stats indexer, ticket 07 prune, TUI)

All paths are siblings of the worker store, under wick's state directory
(the directory holding `workers.json`). Code: `wick-pig/archive.go`.

```text
blobs/<hex>                            one file per distinct content, named for
                                       its SHA-256; the system prompt, the tool
                                       definitions served for the session's role,
                                       and the harness flags each land here once
                                       no matter how many sessions share them.
archive/<run>/<agent>/session.json     {run, agent, harness, role, started,
                                       system_prompt: "sha256:<hex>",
                                       tools: "sha256:<hex>",
                                       flags: "sha256:<hex>",
                                       transcript: "transcript.jsonl",
                                       transcript_source: "<harness path>"}
archive/<run>/<agent>/transcript.jsonl verbatim copy of the harness transcript,
                                       appended by byte offset; survives harness
                                       cleanup.
archive/<run>/<agent>/transcript.offset
                                       "<source offset> <copy size>": bytes of
                                       the source already copied, and the copy's
                                       own size. A restart resumes from the
                                       offset and cuts a torn tail back instead
                                       of re-copying it. Parse with two fields;
                                       a file that does not parse is corrupt.
archive/<run>/<agent>/usage.jsonl      NOT HERE: the per-agent usage log lives
                                       outside the archive (spec: usage log and
                                       archive are siblings). It is
                                       `<state>/usage/<agent-id>.jsonl` per
                                       ticket 03 (`UsageLogPath`, sharing the
                                       file with ticket 04's find lines),
                                       which the stats indexer reads instead.
- `<run>` is the spec slug (`specSlug` of `WICK_SPEC`), or `"adhoc"` for a
  session outside any run. `<agent>` is `WICK_AGENT_ID`, sanitised for the
  filesystem (`/` and `\` become `-`, as with launch files). Every session
  wick launches inside a run inherits the spawner's spec (workers,
  reviewers, scouts, read-only agents), so each agent's transcript and
  session record land under the run they belong to.
- The blob hash is `sha256:<hex>`; the file is the hex part. Two sessions
  sharing a prompt/tool set/flags write one blob, two `session.json`
  references (same content stored twice leaves one file: temp file plus
  atomic rename; racers write identical bytes, so either outcome is final).
- Tool definitions are stored as the MCP `tools/list` shape
  (`name, description, inputSchema, annotations.readOnlyHint`), so the blob
  matches what the harness saw. Flags store only the stable launch choice
  (`{harness, model, effort}`), never the prompt, so same-way launches share
  one blob.
- The copier runs in the session's own wick process (`wick-mcp` attached to
  the harness): `SnapshotFromEnv` once at startup, `CopyFromEnv` every
  `archivePoll` (5 s) via `StartArchiveLoop`, plus a final `CopyFromEnv`
  when serving ends (the settled-session final copy). Launches carry the
  identity the session needs to find its own transcript
  (`TranscriptSource`: session id, then agent id as label fallback, across
  the named harness or all harnesses): `WICK_HARNESS` always, and
  `WICK_SESSION_ID`, `WICK_SESSION_DIR`, `WICK_MODEL`, `WICK_EFFORT` when
  the session sets them.
- A missing source (nothing written yet, or harness cleanup) copies nothing
  and keeps the archive; a source shorter than the offset was replaced, so
  the copy restarts from its beginning. A corrupt offset file is an error.
- Nothing is deleted automatically; prune (ticket 07) removes runs and the
  blobs nothing references. Readers (TUI, stats) poll plain files.
