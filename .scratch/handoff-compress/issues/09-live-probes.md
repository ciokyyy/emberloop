# 09: Live probes

**What to build:** One live probe per Backend that compresses a real Session and checks the new handle answers from the Handoff alone.

**Blocked by:** 05, 06

**Status:** ready-for-agent

- [ ] A probe script each for pig SDK, pig-over-ACP and claude, in `scripts/`
- [ ] Each probe plants a fact, compresses, then asks for the fact
- [ ] The scripts doc lists the probes and states they need live agents
- [ ] The probes are not part of the repo check
