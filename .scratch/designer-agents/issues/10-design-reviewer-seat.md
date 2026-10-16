# 10: Design Reviewer seat

**What to build:** The required Reviewer set depends on the Ticket's effective type: `code` gets spec and standards as today; `design` gets spec, standards and design. The design Reviewer is a persistent Reviewer on the `reviewer-design` Chain (empty falls back to `reviewer-standards` with a Ticket warning), judges the rendered result at the Ticket tip rather than the diff, and reports through the same typed `submit` verdict, with the tip gate, stale-Pass rule and selective re-review as other Reviewers. The Ticket is not Passed until every required Reviewer has a Pass at the current tip. The design Reviewer's first prompt gets its own brief (its axis is visual quality, hierarchy, spacing, responsiveness, states, copy and fit to the Ticket's intent; it may not edit). The browser skill text is ticket 11; this ticket leaves a hook for it. Re-read `runs_exec.go` (`requiredReviewerTypes`, `reviewerPrompt`, `standardsBrief`, `specBrief`), `runs_reviewers_test.go`, `runs_reviewer_gates_test.go`, `review_parse.go` and `runs_inject.go`.

**Blocked by:** 02-ticket-type-parse, 08-design-chain-roles

**Status:** done

- [x] `requiredReviewerTypes` becomes a function of Ticket type; `code` Tickets get exactly the current two Reviewers (test pins it)
- [x] A `design` Ticket seats a third Reviewer on `reviewer-design`; its verdict, tip gate and stale-Pass rule match the existing Reviewers; Pass needs all three
- [x] Empty `reviewer-design` uses the `reviewer-standards` Chain and records a warning
- [x] The design Reviewer brief and prompt are separate from the standards brief; blocker and major findings require Dispositions as today
- [x] Chain advance, Switch and restart recovery work for the new seat; `ensuring` serialization still holds with three seats
- [x] Run driver and reviewer-gate tests cover the three-seat Pass, a `changes` from the design Reviewer, selective re-review and the fallback warning; `.emberloop/check` passes
