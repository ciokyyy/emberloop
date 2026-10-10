# 01: Shared ticket parser package

**What to build:** Extract the ticket parsing from `emberloop/driver.go` (`readDriverTickets`, `driverBlockedValue`, `parseBlockerRefs`, `resolveDriverRefs`) into one package that the Driver and the Daemon both import. Behaviour and error strings stay the same. Add `ValidateSpec` for one spec or ticket file and a cross-file check for duplicate numbers, unknown blockers and cycles. The Daemon Runs preview (`emberloop-web/runs.go`, `discoverSpecs`) uses the package.

**Blocked by:** None

**Status:** todo

- [ ] The Driver builds its ticket graph through the shared package with no change in its tests
- [ ] The Daemon preview uses the same package
- [ ] `ValidateSpec` returns problems with file, line and text for a missing number, a missing or unparsable `Blocked by` line and a missing title
- [ ] Cross-file checks report duplicate numbers, unknown blockers and cycles
- [ ] Table tests cover each problem kind
