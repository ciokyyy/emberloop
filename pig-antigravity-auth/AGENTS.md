# AGENTS.md — pig-antigravity-auth

## Purpose

A pig extension registering the google-antigravity model provider: Google
account OAuth against Antigravity's Cloud Code Assist endpoints, serving the
subscription's Gemini, Claude, and GPT-OSS models to pig. Files: `oauth.go`
and `session.go` (account flow and request identity), `stream.go` and
`convert.go` (response shaping), `models.go`, `extension.go`.

## Ownership

Owns this directory as a separate Go module. Provider scope only; harness
model selection lives in `wick-pig/models.go`.

## Local Contracts

- Unofficial client, outside Google's terms of service; account suspension
  is a known risk, stated in the package doc.

## Work Guidance

- No live calls from tests; stream shaping is covered by fixtures, not the
  network.

## Verification

 No check; the repo check covers `wick-pig/` and `wick-tui/`.

## Child DOX Index

No child docs.
