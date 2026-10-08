# 07: Docs and repo wiring for the web pipeline

**What to build:** The module docs describe the Run feature accurately: what the Daemon owns, the new API surface, the contracts (core never imports web, no herdr in web), and how to test it. The root README points to the Runs view. The repo index lists the new spec area. Docs state the final behaviour, not the ticket history.

**Blocked by:** 04, 05, 06

**Status:** ready-for-agent

- [ ] The web module doc covers Runs, discovery, state files, actions and the shared cap
- [ ] The core module doc notes the exports the web world uses
- [ ] The root README and agent index mention the Runs view and this spec area
- [ ] The main check passes in each touched module
