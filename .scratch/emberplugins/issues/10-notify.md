# 10: Notify

**What to build:** An approved Emberplugin with the `notify` permission sends the user a notification, for example "scraper found 12 new leads". It reaches Web Push subscriptions and shows as an in-app banner like Needs-you. Notifications are rate-limited, tagged with the plugin name, and tapping one opens the plugin page.

**Blocked by:** 09 (Approval screen and trust ADR)

**Status:** ready-for-agent

- [ ] A notify route gated by the `notify` permission
- [ ] Web Push delivery through the existing push service, with the plugin name in the title
- [ ] In-app banner in the UI for plugin notifications
- [ ] Rate limit per plugin, with refusal and a logged drop when exceeded
- [ ] Tapping or clicking opens the plugin page
- [ ] Tests cover permission refusal, rate limit, push delivery with the existing test stubs, and banner rendering
