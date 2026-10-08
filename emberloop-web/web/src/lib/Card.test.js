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
