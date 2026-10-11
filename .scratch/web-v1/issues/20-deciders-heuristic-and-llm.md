# 20: Decider interface, heuristic and LLM implementations

**What to build:** `Decider` Go interface answering typed questions (`durable`, `kind`, `scope_hint`, `contradicts`, `relevant`) with probabilities. Implementations: `heuristic` (default, from ticket 19) and `llm` (one-off pig call with no tools, JSON schema output, `memoryModel` setting, same calling code as the compress summarizer). The `ensemble` mode: heuristic proposes, the second Decider confirms against `confirmLow` and `confirmHigh`. Every output is stored with model and version. Settings: `deciderMode`, `memoryModel`, thresholds. A Decider never writes; Go validates and applies.

**Blocked by:** 08, 19

**Status:** done

- [x] Invalid or out-of-range Decider output is rejected and treated as `unavailable`, never as a yes
- [x] Ensemble drops below `confirmLow`, stores an observation between the thresholds, and makes a candidate queue-eligible at or above `confirmHigh`
- [x] Decider outputs are stored in `decider_json` with model and version
- [x] A fake Decider with scripted probabilities drives API tests
- [x] Redaction runs before every `llm` call
- [x] Settings panel exposes mode, model and thresholds (admin-only for global ones)
- [x] Owning `AGENTS.md` files and root index updated; repo check passes
