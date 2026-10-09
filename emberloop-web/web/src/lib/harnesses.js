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

// Split a "provider/id" model into its parts. Bare ids (claude aliases,
// "auto", custom strings) have no provider.
export function splitModel(model) {
  const text = String(model);
  const i = text.indexOf('/');
  if (i < 0) return { provider: '', name: text };
  return { provider: text.slice(0, i), name: text.slice(i + 1) };
}

// Picker items for a harness: [{ value, label, detail }]. Pig rows carry the
// provider as detail (name on top, provider underneath) and sort by provider
// then name, so one provider's models read as a group; other harnesses keep
// their short lists in order. The current selection stays first when the
// Daemon no longer lists it, so a stored choice never vanishes.
export function modelItems(harness, models, current = '') {
  const items = models.map((m) => {
    if (harness === 'pig') {
      const { provider, name } = splitModel(m);
      return { value: m, label: provider ? name : m, detail: provider };
    }
    return { value: m, label: modelLabel(harness, m) };
  });
  if (items.some((i) => i.detail)) {
    items.sort((a, b) => (a.detail ?? '').localeCompare(b.detail ?? '') || a.label.localeCompare(b.label));
  }
  if (current && !items.some((i) => i.value === current)) {
    if (harness === 'pig') {
      const { provider, name } = splitModel(current);
      items.unshift({ value: current, label: provider ? name : current, detail: provider });
    } else {
      items.unshift({ value: current, label: modelLabel(harness, current) });
    }
  }
  return items;
}
