import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, within, fireEvent } from '@testing-library/svelte';
import { tick } from 'svelte';
import Transcript from './Transcript.svelte';

const queueState = vi.hoisted(() => ({ items: [] }));
vi.mock('./api.js', async (orig) => ({
  ...(await orig()),
  getQueue: vi.fn(async () => ({ queue: queueState.items.map((item) => ({ ...item })) })),
  editQueueItem: vi.fn(async (_sessionId, id, text) => {
    queueState.items = queueState.items.map((item) => item.id === id ? { ...item, text } : item);
    return { updated: true };
  }),
  removeQueueItem: vi.fn(async (_sessionId, id) => {
    queueState.items = queueState.items.filter((item) => item.id !== id);
    return { removed: true };
  }),
  getCurrentUserId: vi.fn(() => 'u1'),
}));

beforeEach(() => {
  queueState.items = [
    { id: 'native-1', mode: 'steer', text: 'native steer', imageIds: [] },
    { id: 'pending-1', mode: 'followup', text: 'pending follow-up', imageIds: ['img-1'] },
  ];
});

afterEach(() => {
  vi.clearAllMocks();
});

const eventBlock = (key, title, body) => ({
  key,
  card: 'user',
  title,
  body,
  sender: 'user',
  senderId: 'u1',
  imageIds: [],
  events: [{ seq: key }],
});

describe('Transcript message action rows', () => {
  it('marks native queued messages in place, renders daemon-pending rows, and updates edit/cancel live', async () => {
    const { container } = render(Transcript, {
      props: {
        sessionId: 's1',
        blocks: [eventBlock(1, 'Steer', 'native steer'), eventBlock(2, 'You', 'delivered prompt')],
      },
    });
    const q = within(container);

    await vi.waitFor(() => expect(container.querySelectorAll('[data-state="queued"]')).toHaveLength(2));
    expect(container.querySelector('[data-block-key="1"] [data-card="user"]')?.getAttribute('data-state')).toBe('queued');
    expect(container.querySelector('[data-block-key="2"] [data-card="user"]')?.getAttribute('data-state')).toBe('delivered');
    expect(container.textContent).toContain('pending follow-up');
    expect(container.querySelector('[data-card="user"][data-state="queued"] img')?.getAttribute('src')).toContain('/api/sessions/s1/images/img-1');

    const nativeCard = container.querySelector('[data-block-key="1"]');
    await fireEvent.click(within(nativeCard).getByRole('button', { name: /Message actions/ }));
    await fireEvent.click(within(nativeCard).getByRole('button', { name: 'Edit' }));
    const editor = within(nativeCard).getByRole('textbox', { name: 'Edit queued message' });
    await fireEvent.input(editor, { target: { value: 'edited native steer' } });
    await fireEvent.click(within(nativeCard).getByRole('button', { name: 'Save' }));
    await tick();

    expect(nativeCard.textContent).toContain('edited native steer');
    expect(nativeCard.querySelector('[data-card="user"]')?.getAttribute('data-state')).toBe('queued');

    await fireEvent.click(within(nativeCard).getByRole('button', { name: /Message actions/ }));
    await fireEvent.click(within(nativeCard).getByRole('button', { name: 'Cancel' }));
    await vi.waitFor(() => expect(nativeCard.querySelector('[data-card="user"]')?.getAttribute('data-state')).toBe('canceled'));
    expect(within(nativeCard).getByTestId('canceled-message').textContent).toBe('Canceled');
    await fireEvent.click(within(nativeCard).getByRole('button', { name: /Message actions/ }));
    expect(within(nativeCard).queryByRole('button', { name: 'Edit and resend' })).toBeNull();
    expect(queueState.items.map((item) => item.id)).toEqual(['pending-1']);
  });

  it('keeps read-only transcripts free of queue rows when no Session is active', async () => {
    queueState.items = [{ id: 'q1', mode: 'steer', text: 'queued', imageIds: [] }];
    const { container } = render(Transcript, { props: { blocks: [], sessionId: null, emptyHint: 'Empty' } });
    await tick();
    expect(container.textContent).toContain('Empty');
    expect(container.querySelector('[data-state="queued"]')).toBeNull();
  });
});
