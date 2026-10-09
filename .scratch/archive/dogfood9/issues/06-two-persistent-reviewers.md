# 06: code_review runs two persistent reviewers

**What to build:** A worker's `code_review` runs a **code** reviewer (repo standards and correctness) and a **spec** reviewer (does the diff cover the ticket and stay inside the spec; uncovered work is a finding), in parallel, and returns one reply with a section per reviewer stating its verdict and the commit it applies to. Reviewers stay alive for the whole ticket: their panes stay open and idle, and el records per worker each reviewer's pane, session, last reviewed commit and last verdict with its commit.

- Arguments: optional `reviewer` (`code` | `spec`) and optional `note` (the worker's reply to findings).
- First call: both reviewers, full diff (base..working tree, as today).
- Later calls: the reviewers named, or by default every reviewer whose last verdict is `changes`. The follow-up is typed into the live pane the way tell_worker delivers, carrying the diff since that reviewer's last review, the note, and the full diff path; the reply is read from the transcript.
- Every reply must end with `VERDICT: pass` or `VERDICT: changes`; missing, el nudges once, then records `changes`.
- The spec reviewer gets pointers to the ticket and its spec; the code reviewer to the repo's standards docs. Reviewers keep the review role (read, find).
- No round cap.

**Blocked by:** 01 (reviewers read their packets by absolute path), 05 (the worker record carries the spec)

**Status:** ready-for-agent

- [ ] First call starts two reviewers and returns both sections with verdicts and commits.
- [ ] Follow-up routing: default to open findings; `reviewer=` overrides; the note reaches the reviewer.
- [ ] A follow-up goes to the same pane/session, with the incremental diff.
- [ ] VERDICT parsing, and nudge-then-`changes` on a missing line.
- [ ] The spec reviewer's packet names the ticket and the spec.
- [ ] All covered over the fake exec.

## Hard limits (this ticket is built by the tool it changes)

- Spec: `.scratch/archive/dogfood9/spec.md`. Read it for the decisions this ticket implements.
- Edit only files under `el/` (and this ticket's named files). Never touch `~/.local/bin`, `~/.pig`, `~/.claude`, `~/.claude.json`, `~/.agents`, `~/.el`, or `scripts/`.
- Never run `go install`, never copy binaries anywhere. Build with `go build ./...` and test with `go test -skip TestLive ./...` from `el/`.
- Never run a `TestLive*` test. Never start el, pig, claude or bunx inside the work. Tests use a fake exec and temp dirs, never the network or the real home.
- Servers, watchers and tails go to the `background` tool, never bash.
- Keep `go vet ./...` and `go test -skip TestLive ./...` green.
