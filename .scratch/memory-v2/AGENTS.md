# AGENTS.md — .scratch/memory-v2

## Purpose

The spec area for Memory v2 in `emberloop-web/memory/` and the Daemon: automatic LLM extraction at turn end, a Memories tier with embeddings and hybrid recall, narrowed human gate, nightly fold/consolidate/promote stages and a scheduler fix. `spec.md` holds the decisions every ticket implements. Tickets 01 to 11 live in `issues/` (dependencies in each file; the spec Ticket map lists them).

## Ownership

Owns `spec.md` in this directory. Code changes land in `emberloop-web/`, owned by its docs. The spec overrides `.scratch/web-v1/spec.md` for Mining, Deciders, Nightly job, Retrieval and injection and Design rule 4; every other web v1 section stands.

## Local Contracts

- The spec decides conflicts between a ticket and itself.
- Rules, the Profile data model, grounding, redaction, purge and snapshots stay as web v1 defines them.
- Evidence behind the extractor choice: the 2026-10-15 benchmark (35 labelled messages, 70+ OpenRouter models). Re-run it, do not trust the numbers, after any prompt or model change.

## Work Guidance

- Read this spec, then `.scratch/web-v1/spec.md` for the sections it keeps, then `emberloop-web/memory/AGENTS.md`.

## Verification

No check.
