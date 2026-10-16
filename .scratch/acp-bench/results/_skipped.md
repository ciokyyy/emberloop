# Skipped candidates (longtail worker, 2026-10-15)

Fast-skipped per brief (closed/proprietary cloud login not redirectable to
fakellm, or no prebuilt artifact installable without cargo/docker/uv/Guix;
this box has only node/npm, python3.11/pip, go, git).

- pi-rust-rpi (bigfish1913/pi-rust): source-only, no cargo → skip.
  (12 MiB RSS claim unverified.)
- claude-code-acp-rs: `cargo install` only → skip.
- banjo (joelreymont/banjo): source-only Zig, no toolchain verified → skip.
- muse-acp (BrokkAi/muse-acp): source-only Rust → skip.
- aar (fischerf/aar): source-only Rust → skip.
- kaagum: Guix package → skip.
- sigit / localharness / construct / zeph: crates.io source, no cargo → skip.
- agentpool (phil65/agentpool): **tried** — needs Python ≥3.13, box has
  3.11 (`pip install agentpool==2.9.18` → "No matching distribution";
  only 0.0.1 stub installable). Re-try on a 3.13 box: LiteLLM-based, the
  most fakellm-friendly Python candidate on paper.
- cuga (cuga 0.4.1): **tried** — `pip install cuga` pulls torch/CUDA +
  opencv/playwright (~5.6 GB in venv, install timed out at 600 s); too
  heavy for this class of backend. Venv deleted.
- hermes-agent: source checkout + venv sync, star-count anomaly unresolved,
  too heavy for remaining budget → skip (defer).
- openhands: installer-script distribution only (no npm, no GH binaries
  found; tags exist up to v1.9.0 but the runtime wants docker) → skip.
- kiro-cli / cursor-agent / copilot-cli / junie / factory-droid /
  blackbox-cli / poolside: proprietary cloud logins, not redirectable → skip.
- crush-acp note: `bub-acp-server` not on npm (404) → bub not attempted.
