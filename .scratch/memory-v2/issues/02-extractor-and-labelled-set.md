# 02: Extractor, prompt x1 and labelled set

**What to build:** An `Extractor` interface with an OpenRouter chat client (`reasoning.effort`, temperature 0, `max_tokens` 4000), a `ScriptedExtractor` for tests, prompt version `x1`, JSON parsing, and the Go validation: drop unknown kind, empty or over-300 `text`, `quote` not a substring of the cited user Event, or text changed by the redactor. `stated=false` stores veracity `inferred`. Importance on write follows the spec. Fallback chain: configured model, then `ClassifyHeuristic` at importance 0.4 inferred, then nothing; one retry after 2 s on 429 or timeout. Add `memory/testdata/extract_eval.json` (15 positive, 20 negative, URLs and secrets stripped) and a gated `TestLiveExtract`.

**Blocked by:** 01

**Status:** done

- [x] Ungrounded, over-long, unknown-kind and redactor-changed items are dropped without a second model call
- [x] Importance is 0.7 plain, 0.85 with durable language, 0.5 inferred
- [x] An empty model setting skips the LLM and uses the heuristic fallback
- [x] A 429 retries once, then falls through the chain
- [x] `TestLiveExtract` with the shipped prompt catches at least 10 of 15 with at most 1 of 20 false alarms (record the result in the ticket before merge)
- [x] Owning `AGENTS.md` files and root index updated; repo check passes
