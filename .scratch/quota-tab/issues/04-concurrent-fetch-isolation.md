# 04: Per-provider concurrent fetch isolation

**What to build:** One slow or failing quota source never delays or hides another card. The Daemon fetches each quota provider concurrently instead of in a sequential loop, so a single outage degrades only its own card while every other card stays fresh.

**Blocked by:** 03-cached-quota-read (both change the same quota read path)

**Status:** ready-for-agent

- [ ] A blocked upstream stub for one provider does not delay the other cards (HTTP-seam test with temp state dir)
- [ ] A failing upstream for one provider leaves the other cards intact in `ok` state
- [ ] The shared per-key lock still serializes concurrent refreshers into one upstream request
- [ ] Go tests at the HTTP seam with local stubs; repo check passes
