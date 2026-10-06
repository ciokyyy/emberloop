# 07: Implement `read` tier 3 with the chosen browser

**What to build:** Replace the tier-3 "not installed" stub with the browser chosen by the research note, so a page that tier 1 and tier 2 could not read is fetched through it. The header names tier 3, the same address safety rules apply, and a missing install still gives the clear error.

**Blocked by:** 04, 06

**Status:** done

- [x] Challenged or JS-only pages are read through the browser
- [x] Safety rules (address blocking, size and time caps) apply to the browser path too
- [x] Missing install returns the clear error; repo check passes
