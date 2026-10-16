# 13: Warnings for unset design Chains and blind models

**What to build:** Users see why a design Ticket will not use a designer-class model. The Run preview and Run detail show a warning on a `design` Ticket when `worker-design` or `reviewer-design` is empty and the fallback applies ("no designer model set; using the worker model"). Settings > Agents shows a warning on a design Chain entry whose model takes no image input, where model metadata says it, and no warning where the metadata is silent. Warnings are advisory: no setting or Run is blocked. Re-read `/api/models` metadata (`model_efforts`, model catalog sync), `SettingsPage.svelte` and the Run preview from tickets 07, 09 and 10.

**Blocked by:** 07-type-badge-and-flip-ui, 09-designer-worker-routing, 10-design-reviewer-seat

**Status:** done

- [x] The Run snapshot's per-Ticket warnings from tickets 09 and 10 render in the preview and detail views, phone and desktop
- [x] `/api/models` exposes image-input capability where the catalog knows it; Settings flags design Chain entries whose model lacks it
- [x] Saving such a Chain still succeeds; the warning copy says it is advisory
- [x] Tests cover the Run warning rendering, the capability flag in the API and the Settings flag; `npm test`, `npm run build` and `.emberloop/check` pass
