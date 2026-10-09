# 04: `read` works on URLs (native fetch, then Jina)

**What to build:** `read` accepts `http(s)://` and `www.` links and returns the page like a file. Tier 1 is a plain fetch (browser User-Agent, redirects, one bounded retry on 429) with in-process HTML to Markdown, JSON pretty-printing, plain text and Markdown passed through, plus `:raw` and line-range selectors. Tier 2 is the Jina reader using `JINA_API_KEY` from the environment (anonymous limits when absent, noted in the header); PDFs and office documents go straight to it. A tier-3 interface exists and returns a clear "not installed" error. Escalation happens on 403, 429, 503, a bot-challenge page, under 100 non-whitespace characters, or a timeout. Output has a header naming URL, content type and the tier that answered, wrapped as untrusted content, and is truncated at 300 lines with the rest saved to a file. Loopback, private and link-local addresses are refused on every redirect hop; the body is capped near 10 MB and the request at 20 s; Jina is skipped for URLs failing the address check or carrying credentials or token-like query parameters.

**Blocked by:** None (can start immediately). Truncated output goes to a file here; ticket 09 later replaces that with the overflow store.

**Status:** done

- [x] HTML, JSON, text and Markdown pages read correctly; selectors and `:raw` work
- [x] Each escalation trigger moves to the next tier, and the header names the tier
- [x] PDF and office documents are read through tier 2
- [x] Private, loopback and link-local targets are refused, including via redirect
- [x] URLs with credentials or token-like parameters are never sent to Jina, and the header says so
- [x] Oversize and slow responses are cut off cleanly; truncation saves the overflow to a readable file
- [x] Tier 3 returns "not installed"
- [x] Local test servers and a fake Jina endpoint cover it; repo check passes
