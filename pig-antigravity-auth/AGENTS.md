# AGENTS.md — pig-antigravity-auth

## Purpose

A pig extension registering the google-antigravity model provider: Google
account OAuth against Antigravity's Cloud Code Assist endpoints, serving the
subscription's Gemini, Claude, and GPT-OSS models to pig. Files: `oauth.go`
and `session.go` (account flow and request identity), `stream.go` and
`convert.go` (response shaping), `models.go`, `extension.go`.

## Ownership

Owns this directory as a separate Go module, imported by `emberloop-web` for
the Daemon's browser-hosted Antigravity login. Provider scope only; harness
model selection lives in `emberloop-web/el/models.go`.

## Local Contracts

- The Daemon's exported `LoginWithCallbacks` hosts browser paste login; invalid
  callback input reports an error and re-prompts against the same PKCE challenge.
  The pig SDK extension login keeps its one-shot prompt and error behavior.
- Unofficial client, outside Google's terms of service; account suspension
  is a known risk, stated in the package doc.

## Work Guidance

- No live calls from tests; stream shaping is covered by fixtures, not the
  network.

## Verification

The repo check compiles this module through `emberloop-web` but does not run its
module tests. Run `cd pig-antigravity-auth && go test ./...` for those tests.

## Child DOX Index

No child docs.
