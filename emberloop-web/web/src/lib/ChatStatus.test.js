import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/svelte';
import ChatStatus from './ChatStatus.svelte';

const glyph = (props) => {
  const { container } = render(ChatStatus, { props });
  return container.querySelector('[role="status"]');
};

describe('ChatStatus', () => {
  it('is one glyph with a word for the accessible name and no visible text', () => {
    const el = glyph({ connected: true, status: 'idle' });
    expect(el.getAttribute('aria-label')).toBe('Live');
    expect(el.getAttribute('data-testid')).toBe('live');
    expect(el.textContent.trim()).toBe('');
  });
  it('shows working while the Session works', () => {
    const el = glyph({ connected: true, status: 'working' });
    expect(el.getAttribute('data-state')).toBe('working');
    expect(el.getAttribute('aria-label')).toBe('Working');
  });
  it('shows reconnecting ahead of working when the stream is down', () => {
    const el = glyph({ connected: false, status: 'working' });
    expect(el.getAttribute('data-state')).toBe('reconnecting');
    expect(el.getAttribute('data-testid')).toBe('reconnect');
    expect(el.getAttribute('aria-label')).toBe('Reconnecting…');
  });
});
