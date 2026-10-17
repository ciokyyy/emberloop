import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, within } from '@testing-library/svelte';
import Transcript from './Transcript.svelte';
import { tick } from 'svelte';

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
  cleanup();
  vi.unstubAllGlobals();
  vi.clearAllMocks();
});

function fakeFrames() {
  let id = 0;
  let now = 0;
  const frames = new Map();
  const request = (callback) => {
    const frame = ++id;
    frames.set(frame, callback);
    return frame;
  };
  const cancel = (frame) => frames.delete(frame);
  vi.stubGlobal('requestAnimationFrame', request);
  vi.stubGlobal('cancelAnimationFrame', cancel);
  window.requestAnimationFrame = request;
  window.cancelAnimationFrame = cancel;
  return {
    flush() {
      for (let pass = 0; frames.size && pass < 50; pass++) {
        const pending = [...frames.values()];
        frames.clear();
        now += 16;
        for (const callback of pending) callback(now);
      }
    },
  };
}
function observeResizes() {
  const observers = [];
  class TestResizeObserver {
    constructor(callback) {
      this.callback = callback;
      observers.push(this);
    }
    observe() {}
    unobserve() {}
    disconnect() {}
  }
  vi.stubGlobal('ResizeObserver', TestResizeObserver);
  window.ResizeObserver = TestResizeObserver;
  return observers;
}


function message(body = 'hello') {
  return { key: 1, card: 'assistant', title: 'Assistant', body, events: [] };

}
function toolStrip(output = 'line one') {
  return {
    key: 1,
    card: 'toolstrip',
    title: 'bash',
    status: 'running',
    settled: false,
    elapsedMs: 100,
    runs: [{ key: 1, name: 'bash', input: 'echo', output, status: 'running' }],
    events: [],
  };
}

function mountTranscript({ blocks = [message()], hasMore = false, onLoadEarlier = null } = {}) {
  const rendered = render(Transcript, { props: { blocks, hasMore, onLoadEarlier, bottomPad: 0 } });
  const scroll = rendered.container.querySelector('[data-testid="transcript-scroll"]');
  const metrics = { scrollHeight: 2000, clientHeight: 600 };
  Object.defineProperties(scroll, {
    scrollHeight: { configurable: true, get: () => metrics.scrollHeight },
    clientHeight: { configurable: true, get: () => metrics.clientHeight },
  });
  scroll.scrollTop = metrics.scrollHeight - metrics.clientHeight;
  fireEvent.scroll(scroll);
  return { ...rendered, scroll, metrics };
}

describe('Transcript scroll follow', () => {
  it('leaves the reader at their wheel position when the last block grows during a pending follow frame', async () => {
    const frames = fakeFrames();
    const { container, rerender, scroll, metrics } = mountTranscript();
    frames.flush();
    const before = scroll.scrollTop;

    metrics.scrollHeight += 200;
    await rerender({ blocks: [message('A streamed chunk that makes the last block taller.')] });
    await fireEvent.wheel(scroll, { deltaY: -90 });
    scroll.scrollTop = before - 90;
    await fireEvent.scroll(scroll);
    frames.flush();

    expect(scroll.scrollTop).toBe(before - 90);
    expect(within(container).getByRole('button', { name: /Latest/ })).toBeTruthy();
  });


  it('does not follow tool-strip height changes after the reader unpins', () => {
    const frames = fakeFrames();
    const observers = observeResizes();
    const { scroll, metrics } = mountTranscript({ blocks: [toolStrip()] });
    frames.flush();
    const resize = observers.at(-1);

    scroll.scrollTop -= 400;
    fireEvent.scroll(scroll);
    const readerTop = scroll.scrollTop;
    metrics.scrollHeight += 200;
    resize.callback();
    frames.flush();

    expect(scroll.scrollTop).toBe(readerTop);
  });

  it('manually anchors a prepended history page and restores browser anchoring', async () => {
    const frames = fakeFrames();
    let scroll;
    let metrics;
    const onLoadEarlier = vi.fn(async () => {
      expect(scroll.style.overflowAnchor).toBe('none');
      metrics.scrollHeight += 500;
    });
    const { container, scroll: scroller, metrics: size } = mountTranscript({ hasMore: true, onLoadEarlier });
    scroll = scroller;
    metrics = size;
    frames.flush();
    scroll.scrollTop = 500;
    await fireEvent.scroll(scroll);
    const beforeTop = scroll.scrollTop;

    const immediateFrame = (callback) => {
      callback(16);
      return 1;
    };
    vi.stubGlobal('requestAnimationFrame', immediateFrame);
    window.requestAnimationFrame = immediateFrame;
    await fireEvent.click(within(container).getByRole('button', { name: 'Load earlier' }));
    await vi.waitFor(() => expect(scroll.scrollTop).toBe(beforeTop + 500));

    expect(onLoadEarlier).toHaveBeenCalledOnce();
    expect(scroll.style.overflowAnchor).toBe('');
  });

  it.each([
    ['wheel', (scroll) => fireEvent.wheel(scroll, { deltaY: -10 })],
    [
      'touch',
      async (scroll) => {
        await fireEvent.touchStart(scroll, { touches: [{ identifier: 1, clientY: 100 }] });
        await fireEvent.touchMove(scroll, { touches: [{ identifier: 1, clientY: 110 }] });
      },
    ],
    ['keyboard', (scroll) => fireEvent.keyDown(scroll, { key: 'PageUp' })],
    ['shift+space', (scroll) => fireEvent.keyDown(scroll, { key: ' ', code: 'Space', shiftKey: true })],
  ])('releases the pin on upward %s input, even within the threshold', async (name, input) => {
    const frames = fakeFrames();
    const { container, scroll } = mountTranscript();
    frames.flush();

    scroll.scrollTop -= 40;
    await fireEvent.scroll(scroll);
    await input(scroll);

    expect(within(container).getByRole('button', { name: /Latest/ })).toBeTruthy();
  });

  it('returns to the bottom with one Latest activation', async () => {
    const frames = fakeFrames();
    const { container, scroll, metrics } = mountTranscript();
    frames.flush();

    scroll.scrollTop -= 400;
    await fireEvent.scroll(scroll);
    const latest = within(container).getByRole('button', { name: /Latest/ });
    await fireEvent.click(latest);
    frames.flush();

    expect(scroll.scrollTop).toBe(metrics.scrollHeight - metrics.clientHeight);
    expect(within(container).queryByRole('button', { name: /Latest/ })).toBeNull();
  });
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
