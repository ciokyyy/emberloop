// Composer mode: which delivery the composer promises. Idle Sessions take a
// plain prompt; working Sessions take a Steer (next tool boundary) or a
// Follow-up (after the turn). No interrupt exists by design.

export const MODES = [
  { id: 'prompt', label: 'Prompt' },
  { id: 'steer', label: 'Steer' },
  { id: 'followup', label: 'Follow-up' },
];

export function resolveComposerMode({ status, selected }) {
  if (status === 'working') return selected === 'followup' ? 'followup' : 'steer';
  return 'prompt';
}

export function availableModes(status) {
  if (status === 'working') return MODES.filter((m) => m.id !== 'prompt');
  return MODES.filter((m) => m.id === 'prompt');
}
