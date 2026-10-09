# 07: agy token usage parser

**What to build:** agy sessions produce normalized usage records per request, in the same shape the run-records spec defines for pig and claude: `{agent, at, harness, provider, model, input, output, cacheRead, cacheWrite, reasoning, reportedCost?}`. Source is whatever the probe found carries usage (transcript or stream-json); agy's estimated session cost is stored as reported cost if available.

**Blocked by:** 03

**Status:** wontfix (superseded: agy is a pig provider, see spec.md)

- [ ] Parser turns probe fixtures into expected records
- [ ] Thinking tokens map to `reasoning`, cache reads to `cacheRead`
- [ ] Plugs into the run-records usage-parser interface (if that isn't merged yet, define the record type compatibly and note it here)
