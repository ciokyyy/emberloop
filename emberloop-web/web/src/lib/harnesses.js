// Harness catalogue for the new-Session dialog. The model list filters by
// harness; the dialog also accepts a custom model string per harness.

export const HARNESSES = [
  { id: 'claude', label: 'Claude', models: ['claude-opus-4-6', 'claude-sonnet-4-6', 'claude-haiku-4-6'] },
  { id: 'pig', label: 'Pig', models: ['auto'] },
  { id: 'fake', label: 'Fake (tests)', models: ['fake-model'] },
];

export const DEFAULT_WORK_DIR = '.';

export function modelsForHarness(harnessId) {
  return HARNESSES.find((h) => h.id === harnessId)?.models ?? [];
}
