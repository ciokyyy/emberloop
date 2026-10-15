# AGENTS.md — .scratch/settings-redesign

## Purpose

The design reference for the Settings, Quota and Providers UI in `emberloop-web/web`: `mock/index.html` is a static Tailwind mock, with its design brief in the top comment. The mock is illustrative; shipped routes and behavior are documented in `emberloop-web/AGENTS.md` and `emberloop-web/web/README.md`. Serve it on the tailnet at `/settings-mock/` (`tailscale serve --set-path`), never on a third-party host.

## Ownership

Owns `mock/index.html`. Code lands in `emberloop-web/web/src/lib`, owned by `emberloop-web/web/README.md`.

## Local Contracts

- The mock illustrates the hub at `/settings` and one page per concern (`/settings/agents`, `runs`, `context`, `memory`, `access`, `profile`, `notifications`, `sound-voice`); in the shipped app, `/quota` is read-only and `/providers` is the global provider-connect page under the Quota rail icon.
- Shell: Rail, second sidebar, main. Page navigation lives only in the second sidebar (Settings list; Quota list with Live status); on phones the same list fills the drawer. No third column. The shipped app's quota API, live refresh and status-list contracts are owned by `emberloop-web/AGENTS.md` and `emberloop-web/web/README.md`.
- Colors come only from `emberloop-web/web/src/app.css` tokens; Button and Badge are pills, primary is white, ring blue is focus only.
- Agents merges role models and Run Chains into one ordered chain per role, grouped as Session agents and Run agents; every entry carries harness, model and effort. The static mock's effort menu is illustrative; the live page offers only the selected model's supported effort levels.
- Project overrides are out of scope.

## Work Guidance

- Open `mock/index.html` in a browser; it loads Tailwind and lucide from CDNs and needs no build.

## Verification

No check.
