# 17: Profile with versioned patches

**What to build:** Per-User Profile stored as lines with stable IDs in `profile_versions`. Changes are patch operations (add, edit, retire) that a human approves; every version is kept; a retired line is hidden, not deleted. 2 KB cap on the rendered Profile. `memory://profile` read, `profile` tool for patch proposals, settings editor, `profile_updated` Event. Each line carries a tag (`workflow`, `style`, `other`) that drives the Reviewer filter.

**Blocked by:** 06, 15

**Status:** ready-for-agent

- [ ] Profile renders from the latest version; an over-2 KB result is refused with a condense message
- [ ] An agent proposes patches only on explicit User request; patches wait in the queue
- [ ] Approve, edit and reject work from the settings view; each approval appends a version and a `profile_updated` Event
- [ ] Reviewers receive only lines tagged for them; lines that assert expertise or lower the review bar cannot carry that tag
- [ ] Profile is injected into every Session of its owner including Subagents and scouts
- [ ] Tests cover cap, versions, retire, filter and Events
- [ ] Owning `AGENTS.md` files and root index updated; repo check passes
