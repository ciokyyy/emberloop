// Harness catalogue for the new-Session dialog. The model list filters by
// harness; the dialog also accepts a custom model string per harness.

export const HARNESSES = [
  { id: 'claude', label: 'Claude', models: ['opus', 'sonnet', 'haiku'] },
  { id: 'pig', label: 'Pig', models: ['auto'] },
  { id: 'fake', label: 'Fake (tests)', models: ['fake-model'] },
];

export const DEFAULT_WORK_DIR = '~';

export function modelsForHarness(harnessId) {
  return HARNESSES.find((h) => h.id === harnessId)?.models ?? [];
}

// Display names. Claude takes aliases that always resolve to the newest
// model in that family, so the label says so.
const MODEL_LABELS = {
  claude: {
    opus: 'Claude Opus (latest)',
    sonnet: 'Claude Sonnet (latest)',
    haiku: 'Claude Haiku (latest)',
  },
};

export function modelLabel(harnessId, model) {
  return MODEL_LABELS[harnessId]?.[model] ?? model;
}
