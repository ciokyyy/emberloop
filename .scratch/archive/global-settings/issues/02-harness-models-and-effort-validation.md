# 02: Harness model lists with effort levels, cached, and validation

**What to build:** Each harness lists its models and the effort levels each accepts through a new `Models() → [{id, label, efforts[]}]` method. pig parses `pig --list-models` (thinking models get off, minimal, low, medium, high, xhigh, max; others none). claude sends the `initialize` control request to `claude -p --input-format stream-json --output-format stream-json` with no prompt and reads `models[]` (`value`, `displayName`, `supportedEffortLevels`). Lists are cached per harness in the state dir. `el settings models [--refresh] [--harness h]` prints them. Saving a slot rejects a model or effort its harness doesn't offer (refreshing the cache once before rejecting); switching harness keeps model and effort when still valid, otherwise clears them with a notice.

**Blocked by:** 01

**Status:** done

- [x] `Models()` on the harness interface; pig and claude implemented through the `Exec` seam
- [x] claude listing spends no tokens (no user message sent) and the process exits
- [x] Per-harness cache with timestamp; `--refresh` rebuilds
- [x] Save rejects an unknown model or unsupported effort; free-text models allowed when the cache can't be built (warn, don't block)
- [x] Harness switch keeps or clears model and effort as described
- [x] Tests: captured `--list-models` output and a captured `initialize` response fed through a fake `Exec`
