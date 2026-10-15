# 04: Embedder, async worker and default model

**What to build:** An `Embedder` interface, an OpenRouter `/api/v1/embeddings` client (batch up to 32), and a `FakeEmbedder`. A bounded async worker per Daemon computes vectors after the write transaction and stores little-endian float32 plus `embedding_model`. Brute-force cosine search in Go over active rows, cap 50,000 vectors per bank, query budget 800 ms with a 256-entry LRU and FTS5 fallback on timeout. Setting `memory_embedding_model`; empty disables vectors. Run a bake-off of `baai/bge-m3`, `openai/text-embedding-3-small` and `qwen/qwen3-embedding-4b` on the labelled set and pin the default.

**Blocked by:** 01

**Status:** todo

- [ ] A Memory is usable before its vector exists
- [ ] Only redacted text reaches the embedder
- [ ] Recall ignores vectors from another model
- [ ] Query timeout falls back to FTS5 and does not fail the call
- [ ] Bake-off result and the pinned default are written in this ticket and in the spec
- [ ] Owning `AGENTS.md` files and root index updated; repo check passes
