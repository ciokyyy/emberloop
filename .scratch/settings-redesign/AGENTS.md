# AGENTS.md — .scratch/settings-redesign

## Purpose

The design reference for the Settings, Quota and Providers redesign in `emberloop-web/web`: `mock/index.html` is a static, single-file Tailwind mock (design brief in its top comment) that the settings, quota and providers tickets implement. Serve it on the tailnet at `/settings-mock/` (`tailscale serve --set-path`), never on a third-party host.

## Ownership

Owns `mock/index.html`. Code lands in `emberloop-web/web/src/lib`, owned by `emberloop-web/web/README.md`.

## Local Contracts

- The mock decides layout, grouping and URLs: a hub at `/settings` and one page per concern (`/settings/agents`, `runs`, `context`, `memory`, `access`, `profile`, `notifications`, `sound-voice`); `/quota` (read-only) and `/providers` (admin writes) are global pages under the Quota rail icon.
- Shell: Rail, second sidebar, main. Page navigation lives only in the second sidebar (Settings list; Quota list with Live status); on phones the same list fills the drawer. No third column.
- Colors come only from `emberloop-web/web/src/app.css` tokens; Button and Badge are pills, primary is white, ring blue is focus only.
- Agents merges role rows and Run Chains into one ordered chain per role; every entry carries harness, model and effort.
- Project overrides are out of scope.

## Work Guidance

- Open `mock/index.html` in a browser; it loads Tailwind and lucide from CDNs and needs no build.

## Verification

No check.
