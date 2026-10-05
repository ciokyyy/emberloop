---
name: worker
description: "Implement one ticket in its own git worktree and report done or blocked."
tools: [read, edit, write, bash, find, background]
checkout: worktree
blocking: false
statuses: [done, blocked]
spawns: [reviewer-standards, reviewer-spec]
requires_review: [reviewer-standards, reviewer-spec]
---
{{prompt}}
