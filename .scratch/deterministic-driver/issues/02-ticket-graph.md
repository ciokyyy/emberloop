# 02: The ticket graph is read and checked before any worker starts

**What to build:** `emberloop implement <spec> --driver --check` reads every ticket under the spec, parses each `Blocked by` line (ticket ids first, free text after is ignored), and prints the order in which tickets would start. A line that does not parse, an id that names no ticket, or a dependency cycle fails with the file and line before any worker would start.

**Blocked by:** None (can start immediately)

**Status:** done

- [x] Ids at the start of a `Blocked by` line are read and trailing free text is ignored; `None (can start immediately)` means no blockers
- [x] An unparsable line fails naming the file and line
- [x] An unknown id and a dependency cycle each fail naming the tickets involved
- [x] The printed order is by ticket number among tickets whose blockers are done
- [x] The parser is exercised through the driver seam, not as a standalone helper test
- [x] Spec stories 4, 5, 6, 7, 8

Spec: `.scratch/deterministic-driver/spec.md`. Read it for the decisions this ticket implements and its Testing Decisions, which bind you: test through the one driver seam (the spec and an injected world with a fake clock), assert on calls and run state, never on internals.
