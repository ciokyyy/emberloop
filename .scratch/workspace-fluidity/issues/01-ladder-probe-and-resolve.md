# 01: Ladder provider routes one request by schedule

**Type:** code

**What to build:** A pig provider `ladder` with the seeded model `ladder/deepseek-flash`. Each request resolves to DeepSeek `deepseek-flash` outside the `deepseek-peak` schedule and to Relace `deepseek-ai/DeepSeek-V4.1-Flash` inside it, then delegates the stream. First answer the probe question from the spec: does the pig SDK let a provider stream delegate to another provider's model? If not, build the Daemon loopback endpoint the spec names. Rungs and schedules are data; the seeded values live in one place. Prove it end to end against `fakellm`: a fake clock crosses the peak boundary and the next request changes provider.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] The probe answer and the chosen mechanism are written in the ticket result
- [ ] A request at 02:00 UTC on a weekday reaches the Relace fake; at 05:00 UTC it reaches the DeepSeek fake; at 02:00 UTC on a Saturday it reaches DeepSeek
- [ ] A Session and a Run Worker both run on `ladder/deepseek-flash` through the Daemon with the fake LLM
- [ ] No live provider call in tests
- [ ] `.emberloop/check` passes

Spec: `../spec.md`.

## Result

- **Probe:** Yes. PiG v0.4.1 exposes `Context.ModelRegistry().StreamSimple`, which delegates a provider stream to a selected model from the host model registry.
- **Chosen mechanism:** Use PiG's `coding.ModelRuntime.RegisterVirtualModel` route for `ladder/deepseek-flash`; the SDK selects a physical model per request and streams it through its native runtime. No Daemon loopback endpoint is needed.
- **Verification:** A fake clock drives weekday peak, weekday off-peak and Saturday requests through a Pig Session, plus a Run Worker, against local `fakellm` servers.
