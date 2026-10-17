import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, within, fireEvent } from '@testing-library/svelte';
import { tick } from 'svelte';
import Card from './Card.svelte';
import { compressSession, promoteSessionRule, decideProfileQueue, editQueueItem, removeQueueItem } from './api.js';
import { people } from './people.svelte.js';

afterEach(() => {
  vi.clearAllMocks();
  vi.unstubAllGlobals();
  delete navigator.clipboard;
  delete navigator.share;
  vi.useRealTimers();
});

vi.mock('./api.js', async (orig) => ({
  ...(await orig()),
  listImages: vi.fn(async () => ({ images: [{ id: 'aud_1', mediaType: 'audio/webm', peaks: [20, 80], durationMs: 5000 }] })),
  compressSession: vi.fn(async () => ({ ok: true })),
  promoteSessionRule: vi.fn(async () => ({ id: 7, status: 'pending' })),
  decideProfileQueue: vi.fn(async () => ({ version: 1, approvedBy: 'user' })),
  getCurrentUserId: vi.fn(() => 'u1'),
  editQueueItem: vi.fn(async () => ({ updated: true })),
  removeQueueItem: vi.fn(async () => ({ removed: true })),
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
describe('Profile patch card actions', () => {
  it.each([['Approve', 'approved'], ['Reject', 'rejected']])('sends %s through the profile queue API', async (label, status) => {
    const { container } = render(Card, { props: { block: { ...block('profilePatch'), profilePatch: { id: 19, ops: [], state: 'pending' } } } });
    await fireEvent.click(within(container).getByRole('button', { name: label }));
    expect(decideProfileQueue).toHaveBeenCalledWith(19, status);
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

describe('Card promote action', () => {
  const injected = (rules) => ({ key: 2, card: 'divider', title: 'Memory injected', body: '2 rules', collapsed: false, rules, events: [] });

  it('offers Promote on each injected rule in the chat', () => {
    const { container } = render(Card, {
      props: { block: injected([{ id: 'rule-aaa111', version: 1 }, { id: 'rule-bbb222', version: 2 }]), sessionId: 's1' },
    });
    expect(container.querySelector('[data-testid="promote-rules"]')).not.toBeNull();
    expect(within(container).getByRole('button', { name: /Promote rule rule-aaa/ })).toBeTruthy();
    expect(within(container).getByRole('button', { name: /Promote rule rule-bbb/ })).toBeTruthy();
  });

  it('proposes through the Session route without leaving the chat', async () => {
    const { container } = render(Card, {
      props: { block: injected([{ id: 'rule-aaa111', version: 1 }]), sessionId: 's1' },
    });
    await fireEvent.click(within(container).getByRole('button', { name: /Promote rule/ }));
    await tick();
    expect(promoteSessionRule).toHaveBeenCalledWith('s1', 'rule-aaa111');
    expect(within(container).getByRole('button', { name: /Promotion proposed/ })).toBeTruthy();
  });

  it('hides the promote row without a Session or without rules', () => {
    const noSession = render(Card, { props: { block: injected([{ id: 'r1', version: 1 }]) } });
    expect(within(noSession.container).queryByTestId('promote-rules')).toBeNull();
    noSession.unmount();
    const noRules = render(Card, {
      props: { block: { key: 3, card: 'divider', title: 'Memory injected', body: '', collapsed: false, events: [] }, sessionId: 's1' },
    });
    expect(within(noRules.container).queryByTestId('promote-rules')).toBeNull();
    noRules.unmount();
  });
});

describe('Card thinking live state', () => {
  it('disables the height animation only while the thinking block is live', () => {
    const live = render(Card, { props: { block: { ...block('thinking', 'Thinking', 'hmm'), live: true } } });
    expect(live.container.querySelector('[data-card="thinking"]')?.getAttribute('data-live')).toBe('true');
    expect(live.container.querySelector('[data-card="thinking"] pre')?.textContent).toBe('hmm');
    live.unmount();
    const settled = render(Card, { props: { block: block('thinking', 'Thinking', 'hmm') } });
    expect(settled.container.querySelector('[data-card="thinking"]')?.getAttribute('data-live')).toBeNull();
    expect(settled.container.querySelector('[data-card="thinking"]')?.className).toMatch(/animate-rise/);
  });
});
describe('per-message actions', () => {
  const userMessage = (over = {}) => ({ ...block('user', 'You', '**hello**\n[docs](https://example.com)'), sender: 'user', senderId: 'u1', imageIds: [], ...over });
  const openActions = async (container) => fireEvent.click(within(container).getByRole('button', { name: /Message actions/ }));

  it('copies plain text and Markdown from User and assistant messages', async () => {
    const writeText = vi.fn(async () => {});
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } });
    const user = render(Card, { props: { block: userMessage() } });
    await openActions(user.container);
    await fireEvent.click(within(user.container).getByRole('button', { name: 'Copy text' }));
    expect(writeText).toHaveBeenLastCalledWith('hello\ndocs');
    user.unmount();

    const assistant = render(Card, { props: { block: block('assistant', 'Agent', '**agent**') } });
    await openActions(assistant.container);
    await fireEvent.click(within(assistant.container).getByRole('button', { name: 'Copy Markdown' }));
    expect(writeText).toHaveBeenLastCalledWith('**agent**');
  });

  it('shares a report through the phone share sheet and falls back to clipboard on desktop', async () => {
    const share = vi.fn(async () => {});
    const writeText = vi.fn(async () => {});
    Object.defineProperty(navigator, 'share', { configurable: true, value: share });
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } });
    vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: true })));
    const report = render(Card, { props: { block: block('report', 'Subagent report', '**finished**') } });
    await openActions(report.container);
    await fireEvent.click(within(report.container).getByRole('button', { name: 'Share' }));
    expect(share).toHaveBeenCalledWith({ text: 'finished' });

    vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: false })));
    report.unmount();
    const assistant = render(Card, { props: { block: block('assistant', 'Agent', 'answer') } });
    await openActions(assistant.container);
    await fireEvent.click(within(assistant.container).getByRole('button', { name: 'Share' }));
    expect(writeText).toHaveBeenLastCalledWith('answer');
  });
  it('provides Copy and Share on User, assistant and report messages', async () => {
    const writeText = vi.fn(async () => {});
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } });
    vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: false })));
    const messages = [
      ['user', userMessage({ body: '**user**' })],
      ['assistant', block('assistant', 'Agent', '**assistant**')],
      ['report', block('report', 'Subagent report', '**report**')],
    ];
    for (const [kind, message] of messages) {
      const rendered = render(Card, { props: { block: message, sessionId: 's1' } });
      await openActions(rendered.container);
      expect(within(rendered.container).getByRole('button', { name: 'Copy text' })).toBeTruthy();
      expect(within(rendered.container).getByRole('button', { name: 'Copy Markdown' })).toBeTruthy();
      expect(within(rendered.container).getByRole('button', { name: 'Share' })).toBeTruthy();
      await fireEvent.click(within(rendered.container).getByRole('button', { name: 'Copy text' }));
      expect(writeText).toHaveBeenLastCalledWith(kind);
      await openActions(rendered.container);
      await fireEvent.click(within(rendered.container).getByRole('button', { name: 'Share' }));
      expect(writeText).toHaveBeenLastCalledWith(kind);
      rendered.unmount();
    }
  });


  it('shows clipboard failures to the User', async () => {
    const { container } = render(Card, { props: { block: userMessage() } });
    await openActions(container);
    await fireEvent.click(within(container).getByRole('button', { name: 'Copy text' }));
    await vi.waitFor(() => expect(within(container).getByRole('alert').textContent).toContain('Clipboard'));
  });
  it('shows Web Share failures and never offers edit for another User', async () => {
    Object.defineProperty(navigator, 'share', { configurable: true, value: vi.fn(async () => { throw new Error('share blocked'); }) });
    vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: true })));
    const report = render(Card, { props: { block: block('report', 'Subagent report', 'result') } });
    await openActions(report.container);
    await fireEvent.click(within(report.container).getByRole('button', { name: 'Share' }));
    await vi.waitFor(() => expect(within(report.container).getByRole('alert').textContent).toContain('share blocked'));
    report.unmount();

    const other = render(Card, { props: { block: userMessage({ senderId: 'u2' }), sessionId: 's1' } });
    await openActions(other.container);
    expect(within(other.container).queryByRole('button', { name: 'Edit and resend' })).toBeNull();
  });


  it('loads only the signed-in User message into the composer as a new prompt', async () => {
    const { COMPOSER_EDIT_EVENT } = await import('./composer.js');
    const onEdit = vi.fn();
    window.addEventListener(COMPOSER_EDIT_EVENT, onEdit);
    const { container } = render(Card, { props: { block: userMessage({ body: 'Fix this', imageIds: ['image-1'] }), sessionId: 's1' } });
    await openActions(container);
    await fireEvent.click(within(container).getByRole('button', { name: 'Edit and resend' }));
    expect(onEdit).toHaveBeenCalledWith(expect.objectContaining({ detail: { sessionId: 's1', text: 'Fix this', imageIds: ['image-1'] } }));
    window.removeEventListener(COMPOSER_EDIT_EVENT, onEdit);
    expect(container.querySelector('[data-card="user"]')?.getAttribute('data-state')).toBe('delivered');
  });

  it('marks queued cards distinctly and edits or cancels one queue item through the API', async () => {
    const onQueueChange = vi.fn();
    const queuedItem = { id: 'q1', mode: 'followup', text: 'old text', imageIds: ['image-1'] };
    const { container } = render(Card, {
      props: { block: { ...userMessage({ title: 'Follow-up', body: 'old text' }) }, sessionId: 's1', queuedItem, onQueueChange },
    });
    expect(container.querySelector('[data-card="user"]')?.getAttribute('data-state')).toBe('queued');
    await openActions(container);
    expect(within(container).queryByRole('button', { name: 'Edit and resend' })).toBeNull();
    await fireEvent.click(within(container).getByRole('button', { name: 'Edit' }));
    const editor = within(container).getByRole('textbox', { name: 'Edit queued message' });
    await fireEvent.input(editor, { target: { value: 'new text' } });
    await fireEvent.click(within(container).getByRole('button', { name: 'Save' }));
    expect(editQueueItem).toHaveBeenCalledWith('s1', 'q1', 'new text');
    expect(onQueueChange).toHaveBeenCalledWith({ type: 'edit', id: 'q1', text: 'new text' });

    await openActions(container);
    await fireEvent.click(within(container).getByRole('button', { name: 'Cancel' }));
    expect(removeQueueItem).toHaveBeenCalledWith('s1', 'q1');
    expect(onQueueChange).toHaveBeenLastCalledWith({ type: 'remove', id: 'q1' });
  });

  it('opens the same action group from a keyboard button and a phone long-press', async () => {
    const { container } = render(Card, { props: { block: userMessage() } });
    const trigger = within(container).getByRole('button', { name: /Message actions/ });
    expect(trigger.getAttribute('aria-haspopup')).toBe('true');
    trigger.focus();
    await fireEvent.click(trigger);
    expect(trigger.getAttribute('aria-expanded')).toBe('true');
    await fireEvent.click(trigger);

    vi.useFakeTimers();
    const press = new Event('pointerdown', { bubbles: true });
    Object.defineProperties(press, { pointerType: { value: 'touch' }, clientY: { value: 20 } });
    container.querySelector('[data-card="user"]').dispatchEvent(press);
    await vi.advanceTimersByTimeAsync(500);
    await tick();
    expect(within(container).getByTestId('message-action-menu')).toBeTruthy();
  });
});
