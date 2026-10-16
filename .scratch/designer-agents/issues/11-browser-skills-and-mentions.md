# 11: Browser skills for Designers and Reviewers

**What to build:** Designers and design Reviewers get the browser skill injected; every other agent gets one line. Two new `run_skills` defaults, `designer` and `reviewer-design`, carry the working rules the research and the probe (05) established: snapshot first with depth and element limits; screenshots only to judge visuals, at 390 px and 1280 px, JPEG at a capped scale, never full-page by default; open the page through the Ticket's dev server (`.emberloop/preview` URL or the AGENTS.md dev command); read screenshots back with `read`; check console and an `a11y` run before `submit`; localhost only; what to do when the browser CLI is missing. Every other Worker and Reviewer brief and every web Session's role prompt gains one line saying the `agent-browser` CLI exists for local UI checks and naming `agent-browser skills get core`. Use the probe's findings to word the screenshot-on-Backend guidance. Re-read `runs_inject.go` (`defaultRunSkills`, the skill-alias map), `prompts.go` (`rolePromptFor`) and the `run_skills` settings.

**Blocked by:** 05-browser-live-probe, 09-designer-worker-routing, 10-design-reviewer-seat

**Status:** ready-for-agent

- [ ] `designer` and `reviewer-design` skills appear in the Designer's and design Reviewer's first prompts, trimmed to the role like other skills, and are overridable through `run_skills` settings
- [ ] Worker, spec Reviewer and standards Reviewer prompts and every web Session role prompt gain exactly one mention line; other prompt text is unchanged
- [ ] The skills name the CLI by its real command shapes verified in the probe, and state the localhost-only rule and the missing-CLI fallback
- [ ] Prompt tests pin the injected text for each role and the absence of the browser section on `code` Tickets beyond the mention line
- [ ] `emberloop-web/AGENTS.md` records the injection contract; `.emberloop/check` passes
