# 15: Know which context files each harness loads

**What to build:** A recorded probe answers which context files the pig harness (built-in tools and skills off) and the claude harness (setting sources off) actually load into a worker's context, and whether either already reads the root AGENTS.md. A one-line CLAUDE.md that imports the root AGENTS.md is added so claude reads the same source. If a harness does not load it, the worker brief carries one line telling the worker to read the root AGENTS.md first. The finding is written into this ticket.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] The ticket records, for each harness, which of AGENTS.md and CLAUDE.md load, with evidence
- [ ] A CLAUDE.md importing the root AGENTS.md exists once the root file does
- [ ] The worker brief carries the read-the-root-doc line only where a harness needs it (brief test)
- [ ] Spec story 21

Spec: `.scratch/retro-hardening/spec.md`. Read it for the decisions this ticket implements, and its Hard limits section, which binds you.
 ## Probe finding (recorded 2026-10-05, static probe — no live pig/claude started, per Hard limits)
 
 Method: inspected the commands wick builds for each harness
 (`wick-pig/harness.go`: `pigHarness.Command`, `claudeHarness.Command`) and
 the pig harness's own docs (`pig docs`: `config.md` "Context files",
 `commands.md` flags). No agent was launched; the finding rests on which
 flags wick passes and what the docs say those flags leave loaded.
 
 ### pig harness (passes `--no-builtin-tools --no-skills`, no `--no-context-files`)
 
 - Loads **both** `AGENTS.md` and `CLAUDE.md`. Pig docs (`config.md`):
   context files are read from the agent directory, the working directory
   and every parent up to the filesystem root; each directory contributes
   the first file that exists in the order `AGENTS.override.md`, `AGENTS.md`,
   `AGENTS.MD`, `CLAUDE.md`, `CLAUDE.MD`. The opt-out is `--no-context-files`
   (`-nc`), which wick never passes (`harness_test.go`:
   `TestPigWorkerGetsNoSkillsAndTheReporter` pins the pig command line).
 - `--no-builtin-tools` and `--no-skills` touch tools and skill discovery
   only (`commands.md`); they do not affect context files.
 - The pig worker's cwd is its worktree root, so the **root `AGENTS.md`
   already loads** (per-directory first match: with both files present at
   the root, `AGENTS.md` wins — the same source `CLAUDE.md` imports).
   Pig also skips the main checkout's same-named file inside a worktree that
   has its own, so the worker reads its worktree's copy, not a stale one.
 - Consequence: pig workers need **no** brief line.
 
 ### claude harness (passes `--setting-sources ""`, sets `CLAUDE_CODE_DISABLE_CLAUDE_MDS=1`)
 
 - Per the spec decision, claude reads `CLAUDE.md`, not `AGENTS.md`.
 - The stripped worker command sets `CLAUDE_CODE_DISABLE_CLAUDE_MDS=1`
   (plus `CLAUDE_CODE_DISABLE_AUTO_MEMORY=1`, `--setting-sources ""`;
   `harness.go`, pinned by `TestClaudeRunsStripped`). By name the harness
   disables `CLAUDE.md` loading for the worker session, so a stripped
   claude worker loads **neither** `AGENTS.md` **nor** `CLAUDE.md` — this
   cannot be relied upon either way, which is the same conclusion.
 - Consequence: claude workers **must** be told. The worker brief now opens
   with one line — `First, read the root AGENTS.md and follow it before
   anything else.` — for `claude` only (`writeBrief(..., harness)`,
   `TestBriefCarriesRootDocLineOnlyWhereNeeded`). Pig briefs carry no
   such line.
 
 ### `CLAUDE.md`
 
 - Added repo-root `CLAUDE.md` with the single line `@AGENTS.md`, so any
   claude session that does load `CLAUDE.md` (non-stripped use) reads the
   same source as pig. Stripped claude workers are covered by the brief
   line regardless.
 - Note: the root `AGENTS.md` itself lands in ticket 16 (blocked by this
   one); until then the `@AGENTS.md` import and the brief line resolve to
   nothing. Both are forward-compatible and need no change when 16 merges.
