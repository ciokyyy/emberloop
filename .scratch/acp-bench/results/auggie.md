# auggie 0.36.0 — bench result: BLOCKED (longtail worker, 2026-10-15)

Verdict: **blocked: Augment cloud only — needs real Augment OAuth session;
no fakellm redirection.**

- Artifact: `npm @augmentcode/auggie@0.36.0` (39 MB; entry
  `augment.mjs`, `--acp` flag; a guessed `dist/index.js` path crashes —
  use `augment.mjs --acp`).
- Protocol-only: `initialize` ok (1.9 s; caps loadSession, image,
  session list). `session/new` → `Internal error` (unauthenticated).
- Auth (from `--help`, not attempted): `login` (OAuth browser flow),
  `AUGMENT_SESSION_AUTH` / `~/.augment/session.json` session seeding,
  `-m/--model` selector. No custom endpoint flags surfaced.
- Subscriptions: Augment subscription only. Nothing else.
