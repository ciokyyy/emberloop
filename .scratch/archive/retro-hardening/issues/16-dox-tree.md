# 16: The repo has its DOX tree

**What to build:** The repo adopts DOX (agent0ai/dox): the root AGENTS.md is the framework contract plus a short project preamble and the child index, and the tree is built by scanning the repo. Child docs sit at each durable boundary: the main Go package (carrying the file map: which file owns what), its tools, its MCP server, its built-in agent types and its test data, the herdr report extension, the Antigravity provider extension, the live scripts, and the spec and ticket area. Each follows the framework's section order; Verification names the repo's existing check or says nothing. The root stays short because always-loaded text costs every turn. A repo test checks that every AGENTS.md is listed in its parent's child index and every listed child exists.

**Blocked by:** 15 Know which context files each harness loads

**Status:** done

- [x] Root AGENTS.md is the DOX contract, a preamble and a complete child index
- [x] Every durable boundary named in the ticket has a child doc in the framework's shape
- [x] The index test passes, and fails when an entry or a child is removed
- [x] No rule is duplicated across docs; Verification sections name only existing checks
- [x] Spec stories 16, 17, 22, 23

Spec: `.scratch/archive/retro-hardening/spec.md`. Read it for the decisions this ticket implements, and its Hard limits section, which binds you.
