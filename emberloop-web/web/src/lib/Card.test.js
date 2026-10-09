import { describe, it, expect } from 'vitest';
import { render, within } from '@testing-library/svelte';
import Card from './Card.svelte';

const block = (card, title = 'T', body = 'body text') => ({ key: 1, card, title, body, collapsed: card === 'tool', events: [] });

describe('Card renders every Event kind', () => {
  for (const card of ['user', 'assistant', 'thinking', 'tool', 'diff', 'permission', 'status', 'subagent', 'error', 'unknown']) {
    it(`renders ${card}`, () => {
      const { container } = render(Card, { props: { block: block(card) } });
      expect(container.querySelector(`[data-card="${card}"]`)).not.toBeNull();
    });
  }
  it('tool output is collapsed by default and expandable', () => {
    const { container } = render(Card, { props: { block: block('tool', 'read', 'out') } });
    const details = container.querySelector('details');
    expect(details?.hasAttribute('open')).toBe(false);
    expect(within(container).getByText('read')).toBeTruthy();
  });
  it('error card is announced as an alert', () => {
    const { container } = render(Card, { props: { block: block('error', 'Error', 'boom') } });
    expect(within(container).getByRole('alert')).toBeTruthy();
  });
  it('diff lines render inside one card', () => {
    const { container } = render(Card, { props: { block: block('diff', 'Diff', '-old\n+new') } });
    expect(container.querySelector('[data-card="diff"]')?.textContent).toContain('+new');
  });
  it('user card labels a steer visibly', () => {
    const { container } = render(Card, { props: { block: { ...block('user', 'Steer', 'go left') } } });
    expect(container.querySelector('[data-card="user"]')?.textContent).toContain('Steer');
  });
  it('user card labels a follow-up visibly', () => {
    const { container } = render(Card, { props: { block: { ...block('user', 'Follow-up', 'after that') } } });
    expect(container.querySelector('[data-card="user"]')?.textContent).toContain('Follow-up');
  });
});

describe('Card compaction', () => {
  const comp = (compaction, body = '') => ({ key: 1, card: 'compaction', title: 'x', body, collapsed: true, compaction, events: [] });
  it('running shows a progress label', () => {
    const { container } = render(Card, { props: { block: comp({ state: 'running' }) } });
    expect(container.querySelector('[data-card="compaction"]')?.textContent).toContain('Compressing context');
  });
  it('done shows the token drop and the saved percent', () => {
    const { container } = render(Card, { props: { block: comp({ state: 'done', reason: 'manual', tokensBefore: 180000, tokensAfter: 24000 }, 'kept') } });
    const text = container.querySelector('[data-card="compaction"]')?.textContent ?? '';
    expect(text).toContain('180k');
    expect(text).toContain('24.0k');
    expect(text).toContain('−87%');
    expect(text).toContain('kept');
  });
  it('an automatic compression is tagged auto', () => {
    const { container } = render(Card, { props: { block: comp({ state: 'done', reason: 'threshold', tokensBefore: 10, tokensAfter: 5 }) } });
    expect(container.querySelector('[data-card="compaction"]')?.textContent).toContain('auto');
  });
  it('failed shows the error', () => {
    const { container } = render(Card, { props: { block: comp({ state: 'failed', error: 'model unavailable' }) } });
    expect(container.querySelector('[data-card="compaction"]')?.textContent).toContain('model unavailable');
  });
});
