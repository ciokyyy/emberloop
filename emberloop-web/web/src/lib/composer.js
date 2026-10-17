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

export const COMPOSER_EDIT_EVENT = 'el:composer:edit';

// Load a delivered message into the current Session's composer without changing history.
export function dispatchComposerEdit(detail) {
  if (typeof window === 'undefined' || typeof CustomEvent === 'undefined') return;
  window.dispatchEvent(new CustomEvent(COMPOSER_EDIT_EVENT, { detail }));
}

// Plain-text copy keeps the readable content while dropping common Markdown syntax.
export function markdownToPlainText(markdown = '') {
  return String(markdown ?? '')
    .replace(/\r\n?/g, '\n')
    .replace(/```[^\n]*\n?([\s\S]*?)```/g, '$1')
    .replace(/`([^`]+)`/g, '$1')
    .replace(/!\[([^\]]*)\]\([^)]*\)/g, '$1')
    .replace(/\[([^\]]+)\]\([^)]*\)/g, '$1')
    .replace(/^[ \t]{0,3}(?:#{1,6}\s+|>\s?|[-+*]\s+|\d+[.)]\s+)/gm, '')
    .replace(/(\*\*|__|~~)(.*?)\1/g, '$2')
    .replace(/(^|[^\w])([*_])([^*_\n]+)\2(?=$|[^\w])/gm, '$1$3')
    .replace(/<[^>]+>/g, '');
}

// Use the phone share sheet on narrow layouts; elsewhere Share copies link-free text.
export async function shareMessage(text) {
  const phone = typeof window !== 'undefined' && typeof matchMedia === 'function' && matchMedia('(max-width: 767px)').matches;
  if (phone && typeof navigator !== 'undefined' && typeof navigator.share === 'function') {
    await navigator.share({ text: String(text ?? '') });
    return 'shared';
  }
  await copyText(text);
  return 'copied';
}

// Clipboard API is preferred; the selection fallback also works on plain HTTP.
export async function copyText(text) {
  const value = String(text ?? '');
  let clipboardError;
  if (typeof navigator !== 'undefined' && navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(value);
      return;
    } catch (error) {
      clipboardError = error;
    }
  }
  if (typeof document !== 'undefined' && document.body && typeof document.execCommand === 'function') {
    const field = document.createElement('textarea');
    field.value = value;
    field.setAttribute('readonly', '');
    field.style.position = 'fixed';
    field.style.opacity = '0';
    document.body.append(field);
    field.select();
    let copied = false;
    try {
      copied = document.execCommand('copy');
    } finally {
      field.remove();
    }
    if (copied) return;
  }
  throw clipboardError ?? new Error('Clipboard is unavailable.');
}
