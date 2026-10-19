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

// A pig picker row. Aggregator ids carry a vendor prefix ("relace/deepseek-ai/
// DeepSeek-V4.1-Flash"): the label keeps the model name alone so it fits, and
// the vendor joins the provider on the muted detail line.
function pigItem(model) {
  const { provider, name } = splitModel(model);
  if (!provider) return { value: model, label: model, detail: '' };
  const cut = name.lastIndexOf('/');
  if (cut < 0) return { value: model, label: name, detail: provider };
  return { value: model, label: name.slice(cut + 1), detail: `${provider} · ${name.slice(0, cut)}` };
}

// Picker items for a harness: [{ value, label, detail }]. Pig rows carry the
// provider as detail (name on top, provider underneath) and sort by provider
// then name, so one provider's models read as a group; other harnesses keep
// their short lists in order. The current selection stays first when the
// Daemon no longer lists it, so a stored choice never vanishes.
export function modelItems(harness, models, current = '') {
  const items = models.map((m) => {
    if (harness === 'pig') return pigItem(m);
    return { value: m, label: modelLabel(harness, m) };
  });
  if (items.some((i) => i.detail)) {
    items.sort((a, b) => (a.detail ?? '').localeCompare(b.detail ?? '') || a.label.localeCompare(b.label));
  }
  if (current && !items.some((i) => i.value === current)) {
    if (harness === 'pig') {
      items.unshift(pigItem(current));
    } else {
      items.unshift({ value: current, label: modelLabel(harness, current) });
    }
  }
  return items;
}

// Card identity per harness: a letter mark plus an accent that tints the
// Subagent card edge and badge for the card's whole life. Claude is
// terracotta orange; pig is the pig-logo teal-green (done states stay
// Tailwind green, distinct from teal).
export const HARNESS_STYLE = {
  claude: { label: 'Claude', mark: 'C', accent: '#d97757', tint: '#e8a082' },
  pig: { label: 'Pig', mark: 'P', accent: '#4fb08f', tint: '#6fcaa9' },
  fake: { label: 'Fake', mark: 'F', accent: '#71717a', tint: '#a1a1aa' },
};

export function harnessStyle(harnessId) {
  const s = HARNESS_STYLE[harnessId];
  if (s) return s;
  const id = String(harnessId ?? '');
  return { label: id || 'agent', mark: (id[0] ?? '?').toUpperCase(), accent: '#71717a', tint: '#a1a1aa' };
}
