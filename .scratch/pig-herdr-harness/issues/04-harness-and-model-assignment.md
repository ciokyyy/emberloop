# How is a harness and a model assigned to each implementor?

Type: grilling
Status: open
Blocked by: 01

## Question

`implement-spec` fans out "background worktree+branch implementers + merger subagents" but never says which agent or which model any of them are. The destination needs at least `pig` and `claude`, and the human wants to be able to say what each role gets — "implementor should be X".

Decide:

1. **Shape.** Where does the assignment live? A config file in the repo next to the tracker, a block in the Piglet, a flag on `/implement`, or something else. It has to survive a VPS restart and be readable by a worker that starts cold.
2. **Granularity.** Per run, per role (implementer / merger / reviewer / fixer), per ticket, or per ticket class? `implement-spec` has distinct roles — implementer, merger, reviewer, single fixer — and they plausibly want different models. A reviewer on the same model as the implementer reviews nothing.
3. **Selection vs execution.** Do we name a harness ("claude") and a model pattern ("claude-opus-4-5"), or a harness plus a *role* ("strong", "cheap", "fast") that maps to a model per repo? PiG already has scoped model cycling and fuzzy pattern matching; herdr's `agent start --kind` takes an executable and passes trailing args through.
4. **Failure shape.** What happens when the named harness is not installed on the VPS, or the model is not authenticated? Fail the run loudly, or degrade to a default? [Recommendation: fail loudly — a silent substitution means the run silently is not the run you specified.]
5. **What PiG already gives us.** `pig` supports `--model`, scoped models, and Piglets that pin model preferences and tool selections. Determine whether a Piglet already *is* the answer before inventing a config format.

This is the one ingredient the route genuinely lacks. Everything around it — task graph, brief, gate, review — already exists.