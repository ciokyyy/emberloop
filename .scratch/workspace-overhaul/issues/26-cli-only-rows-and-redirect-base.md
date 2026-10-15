# 26: CLI-only rows and the redirect base URL

**What to build:** No provider row is a dead end. Other OAuth providers show their exact CLI command (for example `pig login <provider>`) with a copy button; cloud-auth providers (Bedrock, Vertex, Cloudflare) show "configure via CLI". A redirect base URL setting (settings file key, default empty) is used for browser OAuth only for providers where a probe shows the redirect is accepted; it is never the default (spec Parts 2 to 6, Providers).

**Blocked by:** 22-providers-page-and-api-keys

**Status:** done

- [x] OAuth rows without a flow show the CLI command with a copy button
- [x] Cloud-auth rows read "configure via CLI"
- [x] The redirect base URL setting exists, defaults to empty, and applies only to providers whose probe accepts it
- [x] Go tests cover the probe and the default; vitest covers the rows
- [x] Repo check and `npm test` pass
