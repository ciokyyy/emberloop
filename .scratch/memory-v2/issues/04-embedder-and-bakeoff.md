# 04: Embedder, async worker and default model

**What to build:** An `Embedder` interface, an OpenRouter `/api/v1/embeddings` client (batch up to 32), and a `FakeEmbedder`. A bounded async worker per Daemon computes vectors after the write transaction and stores little-endian float32 plus `embedding_model`. Brute-force cosine search in Go over active rows, cap 50,000 vectors per bank, query budget 800 ms with a 256-entry LRU and FTS5 fallback on timeout. Setting `memory_embedding_model`; empty disables vectors. Re-run `.scratch/memory-v2/bench-embed/` on the shipped labelled set against `qwen/qwen3-embedding-4b`, `voyageai/voyage-4-lite` and `baai/bge-m3`; change the pinned default only with evidence.

**Blocked by:** 01

**Status:** done

- [x] A Memory is usable before its vector exists
- [x] Only redacted text reaches the embedder
- [x] Recall ignores vectors from another model
- [x] Query timeout falls back to FTS5 and does not fail the call
- [x] Bake-off result is written here; the spec is updated only if the default changes
- [x] Owning `AGENTS.md` files and root index updated; repo check passes

**Bake-off confirmation (2026-10-15):** Re-ran `.scratch/memory-v2/bench-embed/` with `emberloop-web/memory/testdata/extract_eval.json` (15 labelled positive message/concept pairs) using OpenRouter. `qwen/qwen3-embedding-4b`: 15/15 top-1, 15/15 top-3, MRR 1.000, mean paraphrase cosine 0.910, contradiction 0.859, unrelated 0.303, paraphrase/contradiction gap 0.050, 2.6s. `voyageai/voyage-4-lite`: 15/15, 15/15, 1.000, 0.903, 0.890, 0.194, 0.013, 1.5s. `baai/bge-m3`: 15/15, 15/15, 1.000, 0.881, 0.856, 0.525, 0.025, 3.1s. Qwen remains the default: retrieval was tied, and it had the strongest paraphrase/contradiction separation; Voyage had lower unrelated similarity and was faster, while BGE had markedly higher unrelated similarity. The checked-in spec default remains unchanged.
