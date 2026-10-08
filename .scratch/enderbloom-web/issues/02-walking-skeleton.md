# 02: Walking skeleton with the fake Backend

**What to build:** The `emberloop-web` module end to end with only the fake Backend. A user opens the Daemon's address, sees an empty Session list, creates a Session (harness, model, working directory), sends a prompt, and watches scripted Events stream into a plain transcript. The Event schema, the Event log, SSE with replay from a sequence number, the loopback and Origin guard, and the embedded static Svelte app (unstyled) all exist. Reloading the page shows the same Session.

**Blocked by:** 01

**Status:** done

- [x] Starting the Daemon serves the embedded UI on loopback only
- [x] A client creates a Session, sends a prompt, and reads Events over SSE in order with sequence numbers
- [x] Reconnecting with a last-seen sequence number delivers only the missed Events
- [x] Reloading the page replays the whole log and shows the same transcript
- [x] A request with a foreign `Origin` or `Host` is rejected
- [x] Restarting the Daemon keeps the Session readable as history
- [x] Seam tests at the HTTP and SSE API with the fake Backend pass; the module is wired into the repo check
