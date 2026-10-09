import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, within, fireEvent } from '@testing-library/svelte';
import { tick } from 'svelte';
import Composer from './Composer.svelte';

const { mockStartLive, mockTranscribe } = vi.hoisted(() => ({
  mockStartLive: vi.fn(),
  mockTranscribe: vi.fn(),
}));
vi.mock('./stt.js', () => ({
  sttSupport: () => ({ ok: true, reason: '' }),
  startLiveRecording: (...args) => mockStartLive(...args),
  transcribe: (...args) => mockTranscribe(...args),
  STT_MAX_BYTES: 25 * 1024 * 1024,
}));

describe('Composer mode badge', () => {
  it('sends as prompt when idle', () => {
    const { container } = render(Composer, { props: { status: 'idle', busy: false, onSend: () => {} } });
    expect(within(container).getByTestId('composer-mode').textContent).toContain('Prompt');
  });
  it('offers steer and follow-up while working, defaulting to steer', () => {
    const { container } = render(Composer, { props: { status: 'working', busy: false, onSend: () => {} } });
    const badge = within(container).getByTestId('composer-mode');
    expect(badge.textContent).toContain('Steer');
    expect(within(container).getByText('Follow-up')).toBeTruthy();
  });
  it('switches the promised mode when follow-up is picked', async () => {
    const { container } = render(Composer, { props: { status: 'working', busy: false, onSend: () => {} } });
    await fireEvent.click(within(container).getByText('Follow-up'));
    await tick();
    expect(within(container).getByTestId('composer-mode').textContent).toContain('Follow-up');
  });
  it('keeps the text and shows an error when send fails', async () => {
    const { container } = render(Composer, { props: { status: 'idle', busy: false, onSend: () => { throw new Error('down'); } } });
    const box = within(container).getByLabelText('message');
    await fireEvent.input(box, { target: { value: 'hello' } });
    await tick();
    await fireEvent.click(within(container).getByText('Send'));
    await tick();
    expect(box.value).toBe('hello');
    expect(within(container).getByRole('alert').textContent).toMatch(/failed/i);
  });
  it('sends with the working mode on submit', async () => {
    const onSend = vi.fn();
    const { container } = render(Composer, { props: { status: 'working', busy: false, onSend } });
    await fireEvent.input(within(container).getByLabelText('message'), { target: { value: 'wait' } });
    await tick();
    await fireEvent.click(within(container).getByText('Send'));
    await tick();
    expect(onSend).toHaveBeenCalledWith('wait', 'steer', []);
  });
  it('clears the box at once and restores it when the send fails', async () => {
    let reject;
    const onSend = () => new Promise((_, r) => (reject = r));
    const { container } = render(Composer, { props: { status: 'idle', busy: false, onSend } });
    const box = within(container).getByLabelText('message');
    await fireEvent.input(box, { target: { value: 'hello' } });
    await tick();
    await fireEvent.click(within(container).getByText('Send'));
    await tick();
    expect(box.value).toBe('');
    reject(new Error('down'));
    await new Promise((r) => setTimeout(r, 0));
    await tick();
    expect(box.value).toBe('hello');
  });
});

describe('Composer working indicator', () => {
  it('shows spinner and label only while working', () => {
    const w = render(Composer, { props: { status: 'working', busy: false, onSend: () => {} } });
    const el = within(w.container).getByTestId('working-indicator');
    expect(el.textContent).toContain('Working');
    expect(el.querySelector('svg').getAttribute('class')).toContain('motion-reduce:animate-none');
  });
  it('shows nothing when idle', () => {
    const { container } = render(Composer, { props: { status: 'idle', busy: false, onSend: () => {} } });
    expect(within(container).queryByTestId('working-indicator')).toBeNull();
  });
});

describe('Composer live voice input', () => {
  let captured;
  beforeEach(() => {
    captured = null;
    mockStartLive.mockReset().mockImplementation(async ({ onSegment }) => {
      captured = onSegment;
      return { stop: async () => {}, cancel: () => {} };
    });
    mockTranscribe.mockReset().mockResolvedValue('hello world');
  });

  const blob = (s = 'x') => new Blob([s], { type: 'audio/webm' });
  async function start(q) {
    await fireEvent.click(q.getByLabelText('record voice'));
    await tick();
  }
  async function stop(q) {
    await fireEvent.click(q.getByLabelText('stop recording'));
    await tick();
  }

  it('streams each segment into the draft live without sending', async () => {
    const onSend = vi.fn();
    const { container } = render(Composer, { props: { status: 'idle', busy: false, onSend } });
    const q = within(container);
    await start(q);
    captured(blob(), 0);
    await vi.waitFor(() => expect(q.getByLabelText('message').value).toBe('hello world'));
    expect(q.getByTestId('voice-live').textContent).toContain('hello world');
    await stop(q);
    expect(onSend).not.toHaveBeenCalled();
    expect(mockTranscribe).toHaveBeenCalledWith(expect.any(Blob), { format: true });
  });

  it('holds a fast later segment until the earlier one lands', async () => {
    let resolveFirst;
    mockTranscribe.mockImplementation((b) =>
      b.size === 1 ? new Promise((r) => (resolveFirst = r)) : Promise.resolve('two'),
    );
    const { container } = render(Composer, { props: { status: 'idle', busy: false, onSend: () => {} } });
    const q = within(container);
    await start(q);
    captured(new Blob(['a']), 0);
    captured(new Blob(['bb']), 1);
    await new Promise((r) => setTimeout(r, 20));
    expect(q.getByLabelText('message').value).toBe('');
    resolveFirst('one');
    await vi.waitFor(() => expect(q.getByLabelText('message').value).toBe('one two'));
    await stop(q);
  });

  it('keeps text typed mid-recording', async () => {
    const { container } = render(Composer, { props: { status: 'idle', busy: false, onSend: () => {} } });
    const q = within(container);
    await start(q);
    await fireEvent.input(q.getByLabelText('message'), { target: { value: 'typed' } });
    await tick();
    captured(blob(), 0);
    await vi.waitFor(() => expect(q.getByLabelText('message').value).toBe('typed hello world'));
    await stop(q);
  });

  it('marks a failed part and keeps the rest', async () => {
    mockTranscribe.mockRejectedValueOnce(new Error('down')).mockResolvedValueOnce('kept');
    const { container } = render(Composer, { props: { status: 'idle', busy: false, onSend: () => {} } });
    const q = within(container);
    await start(q);
    captured(blob(), 0);
    captured(blob(), 1);
    await vi.waitFor(() => expect(q.getByLabelText('message').value).toBe('kept'));
    expect(q.getByTestId('voice-note').textContent).toMatch(/part 1 failed.*rest is kept/);
    await stop(q);
  });

  it('says nothing heard when every part is empty', async () => {
    mockTranscribe.mockResolvedValue('');
    const { container } = render(Composer, { props: { status: 'idle', busy: false, onSend: () => {} } });
    const q = within(container);
    await start(q);
    captured(blob(), 0);
    await stop(q);
    await vi.waitFor(() => expect(q.getByTestId('voice-note').textContent).toMatch(/no speech/i));
  });

  it('toggles the per-segment cleanup pass', async () => {
    const { container } = render(Composer, { props: { status: 'idle', busy: false, onSend: () => {} } });
    const q = within(container);
    await start(q);
    captured(blob(), 0);
    await vi.waitFor(() => expect(mockTranscribe).toHaveBeenCalledWith(expect.any(Blob), { format: true }));
    await fireEvent.click(q.getByText(/cleanup on/i));
    await tick();
    expect(q.getByText(/cleanup off/i)).toBeTruthy();
    captured(blob(), 1);
    await vi.waitFor(() => expect(mockTranscribe).toHaveBeenCalledWith(expect.any(Blob), { format: false }));
    await stop(q);
  });
});
