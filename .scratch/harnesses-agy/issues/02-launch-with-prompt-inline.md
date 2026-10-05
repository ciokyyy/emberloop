# 02: Agents receive their prompt directly

**What to build:** Every agent wick launches (workers, scouts, reviewers, any harness) gets its full brief or prompt as its first message, instead of "Read <file> and follow it exactly". wick writes the complete launch (binary, args with the full prompt, env) to a launch file in its state dir and types one shell-agnostic line into the pane: `wick-pig exec <launch-file>`, which reads the file and execs the harness. Brief and prompt files are still written for the archive, but agents are not told to read them.

**Blocked by:** None (can start immediately)

**Status:** done

- [x] `wick-pig exec <file>` execs the binary with exact argv and env, preserving newlines and quotes
- [x] Typed pane line is the single `wick-pig exec` line, working in fish, bash and zsh
- [x] Workers, scouts and reviewers on pig and claude start with the full prompt as their first user message
- [x] Launch files cleaned up with the session's other state files
- [x] Tests: `BuildCommand`/launch tests assert the typed line and launch file contents; an exec test with a fake binary asserts argv and env
