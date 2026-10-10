// Shared spec-discovery state for the Runs area: the sidebar owns the repo
// directory form and the spec list, while RunsView owns the preview table,
// the Start button and the live Run. Both read this module, so a pick in the
// rail-2 sidebar repaints the main area with no prop drilling.
import { listSpecs, previewSpec } from './api.js';

export const discovery = $state({
  repo: '',
  specs: [],
  selected: '',
  previewTitle: '',
  tickets: [],
  message: '',
  error: '',
  // Bump on every explicit user pick so RunsView can drop a live Run whose
  // spec no longer matches. Cold-link seeding (ensureDiscoveryContext) never
  // bumps it, so the seeded Run stays.
  pickNonce: 0,
  // Cold-link seeding runs once per Run and never twice concurrently.
  seeding: false,
  specSeedFor: '',
});

export function resetDiscovery() {
  discovery.repo = '';
  discovery.specs = [];
  discovery.selected = '';
  discovery.previewTitle = '';
  discovery.tickets = [];
  discovery.message = '';
  discovery.error = '';
  discovery.pickNonce = 0;
  discovery.seeding = false;
  discovery.specSeedFor = '';
}

export function initDiscoveryRepo(workDir) {
  if (!discovery.repo && workDir) discovery.repo = workDir;
}

export async function loadDiscovery() {
  discovery.error = '';
  discovery.message = '';
  discovery.specs = [];
  discovery.selected = '';
  discovery.tickets = [];
  discovery.previewTitle = '';
  discovery.specSeedFor = '';
  if (!discovery.repo) {
    discovery.message = 'Enter a repo directory to find specs.';
    return;
  }
  try {
    const body = await listSpecs(discovery.repo);
    discovery.specs = body.specs ?? [];
    if (!discovery.specs.length) discovery.message = 'No specs found under this directory.';
  } catch {
    discovery.error = 'Could not list specs.';
  }
}

// Preview one spec's tickets without touching the live Run: explicit picks
// bump pickNonce (RunsView drops a mismatched Run), the deep-link seed below
// must keep it.
export async function previewDiscovery(spec, { picked = false } = {}) {
  discovery.selected = spec;
  if (picked) discovery.pickNonce++;
  discovery.tickets = [];
  discovery.previewTitle = '';
  discovery.error = '';
  try {
    const body = await previewSpec(spec);
    // A reselect during the fetch wins: ignore the stale answer.
    if (discovery.selected !== spec) return;
    discovery.previewTitle = body.title ?? '';
    discovery.tickets = body.tickets ?? [];
  } catch {
    if (discovery.selected !== spec) return;
    discovery.error = 'Could not load the ticket preview.';
  }
}

export async function selectDiscovery(spec) {
  await previewDiscovery(spec, { picked: true });
}

// Cold deep link fix: opening /runs/<runId> directly used to show the live
// table with no spec-picker context. ensureDiscoveryContext seeds the picker
// from the Run's own repo and spec once, leaving the live Run untouched. It
// never runs twice concurrently, never re-seeds a Run it already seeded,
// and never clobbers a repo the user typed.
export async function ensureDiscoveryContext(run) {
  if (discovery.seeding || !run?.repo) return;
  if (run.id && discovery.specSeedFor === run.id) return;
  if (discovery.specs.length || discovery.selected) return;
  const target = run.repo;
  // The user typed a repo of their own: keep it.
  if (discovery.repo && discovery.repo !== target) return;
  discovery.repo = target;
  discovery.seeding = true;
  try {
    const body = await listSpecs(target);
    if (!run || run.repo !== target) return;
    discovery.specs = body.specs ?? [];
    if (run.id) discovery.specSeedFor = run.id;
    const spec = run.spec;
    if (spec && discovery.specs.some((s) => s.spec === spec)) await previewDiscovery(spec);
  } catch {
    // The live table keeps working without picker context.
  } finally {
    discovery.seeding = false;
  }
}
