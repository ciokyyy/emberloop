# 08: First route renders before the lists arrive

**What to build:** `/quota` and `/runs` open fast on a cold Daemon. Boot no longer awaits the Session and Run lists before the first route renders; each view loads its own data (spec Part 2, Speed).

**Blocked by:** 07-coalesced-runs-refetch (both change `App.svelte` boot)

**Status:** ready-for-agent

- [ ] Boot renders the first route without awaiting the Session and Run lists
- [ ] Each view loads its own data and shows a loading state until it arrives
- [ ] Vitest covers a boot that renders before the lists arrive
- [ ] Cold boot time after the change is recorded beside the baseline from ticket 04; repo check and `npm test` pass
