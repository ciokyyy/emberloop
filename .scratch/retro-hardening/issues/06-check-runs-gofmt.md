# 06: The repo check fails on unformatted Go

**What to build:** The repo's check command also runs a formatting check, and the one file that has been unformatted since an early ticket is formatted in the same change, so the check is green when this lands. Formatting becomes a pass or fail result, never a judgement call for a worker.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] A deliberately unformatted file makes the check command fail (test or recorded run)
- [ ] The current tree passes the check, vet and tests
- [ ] Spec stories 5 and 6

Spec: `.scratch/retro-hardening/spec.md`. Read it for the decisions this ticket implements, and its Hard limits section, which binds you.
