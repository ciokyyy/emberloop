# 32: Live probes

**What to build:** One live probe per Backend that compresses a real Session and checks the new handle answers from the Handoff alone, and one probe that calls the Jev endpoint.

**Blocked by:** 11, 12, 21

**Status:** ready-for-agent

- [ ] A probe script each for pig SDK, pig-over-ACP and claude in `scripts/`; each plants a fact, compresses, then asks for the fact
- [ ] A Jev probe behind `EL_LIVE_JEV=1`
- [ ] The scripts doc lists the probes and states they need live agents
- [ ] The probes are not part of the repo check
