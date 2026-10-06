# 12: Read-only `git` tool

**What to build:** A native `git` tool taking `args` (list of strings), run as `git -C <worktree> --no-pager ...`. Allowed subcommands: `diff`, `log`, `show`, `blame`, `status`, `ls-files`, `grep`, `rev-parse`, `merge-base`, `cat-file`, plus `branch` and `tag` only with a list flag. Anything else is refused with the allowed list. Refused flags: `-c`, `--output`, `--ext-diff`, `--textconv`, `--exec`, `--upload-pack`. `--no-ext-diff --no-textconv` are always added; the git config environment is cleared (`GIT_CONFIG_GLOBAL` and `GIT_CONFIG_SYSTEM` pointed at the null device, `GIT_EXTERNAL_DIFF` unset, no pager). The tool is `ReadOnly`, uses the bash-style output sanitising and timeout, and its default placement is `execute`.

**Blocked by:** None (can start immediately)

**Status:** done

- [x] Each allowed subcommand works against a temporary repository
- [x] Every refused subcommand and flag is refused with a clear message, including flag forms like `--output=file` and `-c x=y`
- [x] A hostile repository config (an external diff driver, textconv) is never executed
- [x] The repository is byte-identical (tree, index, refs) after a run of every allowed command
- [x] Repo check passes
