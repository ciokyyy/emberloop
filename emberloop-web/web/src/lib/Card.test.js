import { describe, it, expect, vi } from 'vitest';
import { render, within } from '@testing-library/svelte';
import Card from './Card.svelte';
import { compressSession } from './api.js';
import { people } from './people.svelte.js';

vi.mock('./api.js', async (orig) => ({
  ...(await orig()),
  listImages: vi.fn(async () => ({ images: [{ id: 'aud_1', mediaType: 'audio/webm', peaks: [20, 80], durationMs: 5000 }] })),
  compressSession: vi.fn(async () => ({ ok: true })),
}));

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
    const { container } = render(Card, { props: { block: block('user', 'Steer', 'go left') } });
    expect(container.querySelector('[data-card="user"]')?.textContent).toContain('Steer');
  });
  it('user card labels a follow-up visibly', () => {
    const { container } = render(Card, { props: { block: block('user', 'Follow-up', 'after that') } });
    expect(container.querySelector('[data-card="user"]')?.textContent).toContain('Follow-up');
  });
  it('an echoed prompt (swap) skips the entrance animation', () => {
    const { container } = render(Card, { props: { block: { ...block('user', 'You', 'hello'), noAnim: true } } });
    const card = container.querySelector('[data-card="user"]');
    expect(card?.getAttribute('data-swap')).toBe('true');
    expect(card?.className).not.toMatch(/animate-rise/);
  });
});
describe('Card divider', () => {
  it('shows the title and fields', () => {
    const { container } = render(Card, { props: { block: block('divider', 'Context compressed', '#2 · deterministic') } });
    const text = container.querySelector('[data-card="divider"]')?.textContent ?? '';
    expect(text).toContain('Context compressed');
    expect(text).toContain('#2 · deterministic');
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

describe('Card subagent and image branches', () => {
  it('renders a group header with member rows that open the child', async () => {
    const { fireEvent } = await import('@testing-library/svelte');
    let opened = null;
    const { container } = render(Card, {
      props: {
        block: { key: 1, card: 'group', title: '2 subagents', groupId: 'g', members: [{ id: 'child-1', name: 'Alpha', harness: 'pig', scout: false, status: 'idle' }], finished: false, events: [] },
        sessionId: 'parent-1',
        sessions: [{ id: 'child-1', name: 'Alpha', harness: 'pig', status: 'idle' }],
        onOpenSession: (id) => (opened = id),
      },
    });
    const group = container.querySelector('[data-card="group"]');
    expect(group).not.toBeNull();
    expect(group.textContent).toContain('Alpha');
    expect(group.textContent).toContain('worker');
    await fireEvent.click(container.querySelector('[data-child="child-1"]'));
    expect(opened).toBe('child-1');
  });
  it('renders image chips on user messages with attachments', () => {
    const { container } = render(Card, {
      props: { block: { ...block('user', 'You', 'see this'), imageIds: ['img1'] }, sessionId: 's1' },
    });
    const img = container.querySelector('[data-card="user"] img');
    expect(img?.getAttribute('src')).toContain('/api/sessions/s1/images/img1');
  });
  it('renders a voice message as a bubble with the text as its transcript', async () => {
    const { container } = render(Card, {
      props: { block: { ...block('user', 'You', 'what the user said'), imageIds: ['aud_1', 'img1'] }, sessionId: 's1' },
    });
    const card = container.querySelector('[data-card="user"]');
    expect(card.querySelector('[data-testid="voice-bubble"]')).not.toBeNull();
    expect(card.querySelector('audio').getAttribute('src')).toContain('/api/sessions/s1/images/aud_1');
    // The text hides behind the Transcript button; the picture stays a picture.
    expect(card.textContent).not.toContain('what the user said');
    expect(card.querySelector('[data-testid="voice-bubble"]').getAttribute('title')).toBe('what the user said');
    expect(card.querySelectorAll('img')).toHaveLength(1);
    await vi.waitFor(() => expect(card.querySelector('[aria-label="seek voice message"]').children).toHaveLength(2));
  });
  it('shows the child name with the id only as a tooltip', () => {
    const { container } = render(Card, {
      props: { block: { key: 2, card: 'subagent', title: 'Alpha', body: 'x', childId: 'child-abcdef', events: [] } },
    });
    const card = container.querySelector('[data-card="subagent"]');
    expect(card.textContent).toContain('Alpha');
    expect(card.getAttribute('title')).toBe('child-abcdef');
  });
});

describe('Card compress blocked', () => {
  it('shows the banner title and the error, and Retry reruns the compress', async () => {
    const { fireEvent } = await import('@testing-library/svelte');
    const { container } = render(Card, {
      props: { block: { key: 3, card: 'compressBlocked', title: 'compress needs pig credits', body: '402 Payment Required', collapsed: false, events: [] }, sessionId: 's1' },
    });
    const card = container.querySelector('[data-card="compressBlocked"]');
    expect(card.textContent).toContain('compress needs pig credits');
    expect(card.textContent).toContain('402 Payment Required');
    await fireEvent.click(within(container).getByRole('button', { name: 'Retry' }));
    expect(compressSession).toHaveBeenCalledWith('s1');
  });
});

describe('user prompt sender badge', () => {
  const prompt = (over) => ({ key: 1, card: 'user', title: 'You', body: 'hello', collapsed: false, imageIds: [], ...over });

  it('shows the sender name on a user prompt', () => {
    people.list = [{ id: 'u-mira', username: 'mira', online: true }];
    const { container } = render(Card, { props: { block: prompt({ sender: 'user', senderId: 'u-mira' }) } });
    expect(within(container).getByTestId('sender-badge').textContent).toContain('mira');
    people.list = [];
  });

  it('falls back to a short id when the User is not in the list', () => {
    people.list = [];
    const { container } = render(Card, { props: { block: prompt({ sender: 'user', senderId: 'u-ghost-account-id' }) } });
    expect(within(container).getByTestId('sender-badge').textContent).toContain('u-ghost-');
  });

  it('shows no badge on a system prompt or a prompt without a sender', () => {
    const system = render(Card, { props: { block: prompt({ sender: 'system', senderId: '' }) } });
    expect(within(system.container).queryByTestId('sender-badge')).toBeNull();
    system.unmount();
    const legacy = render(Card, { props: { block: prompt({}) } });
    expect(within(legacy.container).queryByTestId('sender-badge')).toBeNull();
  });
});
