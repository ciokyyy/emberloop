# 13: `check` tool with named repo checks

**What to build:** `.emberloop/check` may stay a plain command (name `default`) or be a JSON map of check names to commands, where a command may contain one `{args}` slot. The existing spawn-time run (`runChecks`, 3-minute timeout, 60-line tail, one run shared by co-launched reviewers) keeps working unchanged against both shapes (it runs `default`, or the first key when no `default` exists, and says which). Add a `check` tool: `name` (default `default`) and `args` (a string substituted into the `{args}` slot only; shell metacharacters in it are refused, and a command without a slot rejects non-empty `args`). Output is the same status line plus 60-line tail as today. Default placement is `execute`. Offered to `review`, `work`, `plan` and `full`, not `scout`.

**Blocked by:** None (can start immediately)

**Status:** done

- [x] A plain `.emberloop/check` file behaves exactly as before for the spawn-time run and the tool
- [x] A named map runs the named check; an unknown name lists the available names
- [x] `{args}` substitution works; metacharacters (`;`, `|`, `&`, `$`, backticks, newlines, redirects) are refused
- [x] Timeout, failure tail and pass output match the existing strings
- [x] Tool is limited to the roles above; repo check passes
