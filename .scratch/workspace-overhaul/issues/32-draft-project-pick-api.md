# 32: Draft project pick in the Daemon

**What to build:** The first prompt sent from a draft with no Project is checked against the User's Projects before the Session starts. The Daemon asks jev (OpenRouter `systemone`, model `typesafe/jev-1.13`, client `emberloop-web/el/tools/systemone/systemone.go`, key from `systemone.APIKey()`) one `noul` question per Project (name plus gist) and one "none" question, in one call. A suggestion is made only if the top probability is at least 0.5 and leads the second by at least 0.2. `memory.ValidateQuestions` allows only the miner questions, so the pick needs its own question type or a direct wire call. Without an OpenRouter key no suggestion is made (spec Parts 2 to 6, Draft project pick).

**Blocked by:** 29-project-gist

**Status:** done

- [x] The draft's first prompt returns a suggested Project or none, before the Session starts
- [x] Wording per Project is name plus gist; paths are not sent
- [x] The gate applies: accept, below top, below margin and "none" all behave as specified
- [x] No OpenRouter key means no suggestion and today's draft behavior
- [x] Go tests with a stubbed `systemone` endpoint cover the gate, the wording and no key; the gated live probe `jev_projectpick_live_test.go` stays out of the check
- [x] Repo check passes
