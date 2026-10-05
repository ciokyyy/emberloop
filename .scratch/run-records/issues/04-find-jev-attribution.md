# 04: find / jev calls recorded against the calling agent

**What to build:** Each decision-model call made by `find` is logged in the calling agent's usage log with the decision model, a find-call id, and any usage or cost the endpoint returns. First, probe the decision endpoint: does it return usage or cost when asked (e.g. OpenRouter's usage option)? Request it if supported; otherwise log call counts only.

**Blocked by:** 01

**Status:** ready-for-agent

- [ ] Probe result recorded in this ticket
- [ ] Decision client requests usage when supported and returns it with the answers
- [ ] Log lines carry `WICK_AGENT_ID`, kind `find`, model, call count, usage or cost if any
- [ ] Tests with a fake decision provider, with and without usage
