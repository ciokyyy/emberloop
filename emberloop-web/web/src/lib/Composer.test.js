import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, within, fireEvent } from '@testing-library/svelte';
import { tick } from 'svelte';
import Composer from './Composer.svelte';
import { writeVoicePrefs, DEFAULT_VOICE } from './voicePrefs.js';

const { mockStart, mockTranscribe } = vi.hoisted(() => ({
  mockStart: vi.fn(),
  mockTranscribe: vi.fn(),
}));
vi.mock('./stt.js', () => ({
  sttSupport: () => ({ ok: true, reason: '' }),
  startRecording: (...args) => mockStart(...args),
  transcribe: (...args) => mockTranscribe(...args),
  STT_MAX_BYTES: 25 * 1024 * 1024,
}));

describe('Composer send mode', () => {
  it('prompts for a plain send when idle', () => {
    const { container } = render(Composer, { props: { status: 'idle', busy: false, onSend: () => {} } });
    expect(within(container).getByLabelText('message').placeholder).toBe('Send a prompt…');
  });
  it('prompts for steer or follow-up while working', () => {
    const { container } = render(Composer, { props: { status: 'working', busy: false, onSend: () => {} } });
    expect(within(container).getByLabelText('message').placeholder).toBe('Steer or follow up…');
    expect(within(container).getByText('Follow-up')).toBeTruthy();
  });
  it('sends as follow-up when follow-up is picked', async () => {
    const onSend = vi.fn();
    const { container } = render(Composer, { props: { status: 'working', busy: false, onSend } });
    await fireEvent.click(within(container).getByText('Follow-up'));
    await tick();
    await fireEvent.input(within(container).getByLabelText('message'), { target: { value: 'later' } });
    await tick();
    await fireEvent.click(within(container).getByText('Send'));
    await tick();
    expect(onSend).toHaveBeenCalledWith('later', 'followup', [], []);
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
    expect(onSend).toHaveBeenCalledWith('wait', 'steer', [], []);
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

describe('Composer voice message', () => {
  let opts;
  let stopOut;
  let cancel;
  beforeEach(() => {
    opts = null;
    cancel = vi.fn();
    stopOut = { blob: new Blob(['audio'], { type: 'audio/webm' }), durationMs: 3000, peaks: [10, 50, 90] };
    mockStart.mockReset().mockImplementation(async (o) => {
      opts = o;
      return { stop: async () => stopOut, cancel };
    });
    mockTranscribe.mockReset().mockResolvedValue('hello world');
    globalThis.URL.createObjectURL = vi.fn(() => 'blob:draft');
    globalThis.URL.revokeObjectURL = vi.fn();
    writeVoicePrefs(DEFAULT_VOICE);
  });

  async function start(q) {
    await fireEvent.click(q.getByLabelText('record voice'));
    await tick();
  }
  async function stop(q) {
    await fireEvent.click(q.getByLabelText('stop recording'));
    await tick();
  }

  it('shows a live recorder with timer and waveform, and sends nothing', async () => {
    const onSend = vi.fn();
    const { container } = render(Composer, { props: { status: 'idle', busy: false, onSend } });
    const q = within(container);
    await start(q);
    expect(q.getByTestId('voice-recorder')).toBeTruthy();
    expect(q.getByTestId('voice-status').textContent).toContain('Listening');
    opts.onLevel(0.5);
    opts.onLevel(0.7);
    await tick();
    expect(q.getByTestId('voice-wave').children.length).toBeGreaterThan(0);
    await stop(q);
    expect(onSend).not.toHaveBeenCalled();
  });

  it('puts the transcript in the draft with a voice bubble, not sent yet', async () => {
    const onSend = vi.fn();
    const { container } = render(Composer, { props: { status: 'idle', busy: false, onSend } });
    const q = within(container);
    await start(q);
    await stop(q);
    await vi.waitFor(() => expect(q.getByLabelText('message').value).toBe('hello world'));
    expect(q.getByTestId('voice-draft')).toBeTruthy();
    expect(q.getByTestId('voice-bubble')).toBeTruthy();
    expect(mockTranscribe).toHaveBeenCalledWith(expect.any(Blob), { format: false });
    expect(onSend).not.toHaveBeenCalled();
  });

  it('blocks send while transcribing', async () => {
    let finish;
    mockTranscribe.mockImplementation(() => new Promise((r) => (finish = r)));
    const { container } = render(Composer, { props: { status: 'idle', busy: false, onSend: () => {} } });
    const q = within(container);
    await fireEvent.input(q.getByLabelText('message'), { target: { value: 'typed' } });
    await start(q);
    await stop(q);
    await vi.waitFor(() => expect(q.getByTestId('voice-status').textContent).toContain('Transcribing'));
    expect(q.getByText('Send').disabled).toBe(true);
    finish('more');
    await vi.waitFor(() => expect(q.getByLabelText('message').value).toBe('typed more'));
    expect(q.getByText('Send').disabled).toBe(false);
  });

  it('lets the user edit the transcript, then sends text plus the recording', async () => {
    const onSend = vi.fn();
    const { container } = render(Composer, { props: { status: 'idle', busy: false, onSend } });
    const q = within(container);
    await start(q);
    await stop(q);
    await vi.waitFor(() => expect(q.getByLabelText('message').value).toBe('hello world'));
    await fireEvent.input(q.getByLabelText('message'), { target: { value: 'hello there' } });
    await tick();
    await fireEvent.click(q.getByText('Send'));
    await tick();
    expect(onSend).toHaveBeenCalledWith('hello there', 'prompt', [], [], {
      blob: stopOut.blob,
      durationMs: 3000,
      peaks: [10, 50, 90],
    });
    expect(q.queryByTestId('voice-draft')).toBeNull();
  });

  it('keeps the recording when transcription fails so the user can type', async () => {
    mockTranscribe.mockRejectedValue(new Error('down'));
    const { container } = render(Composer, { props: { status: 'idle', busy: false, onSend: () => {} } });
    const q = within(container);
    await start(q);
    await stop(q);
    await vi.waitFor(() => expect(q.getByTestId('voice-note').textContent).toMatch(/Transcription failed \(down\)/));
    expect(q.getByTestId('voice-draft')).toBeTruthy();
  });

  it('discards the recording from the draft bubble', async () => {
    const { container } = render(Composer, { props: { status: 'idle', busy: false, onSend: () => {} } });
    const q = within(container);
    await start(q);
    await stop(q);
    await vi.waitFor(() => expect(q.getByTestId('voice-draft')).toBeTruthy());
    await fireEvent.click(q.getByLabelText('discard recording'));
    await tick();
    expect(q.queryByTestId('voice-draft')).toBeNull();
  });

  it('cancels a take without transcribing', async () => {
    const { container } = render(Composer, { props: { status: 'idle', busy: false, onSend: () => {} } });
    const q = within(container);
    await start(q);
    await fireEvent.click(q.getByLabelText('cancel recording'));
    await tick();
    expect(cancel).toHaveBeenCalled();
    expect(mockTranscribe).not.toHaveBeenCalled();
    expect(q.queryByTestId('voice-recorder')).toBeNull();
  });

  it('keeps the audio so far when the browser ends the take early', async () => {
    const { container } = render(Composer, { props: { status: 'idle', busy: false, onSend: () => {} } });
    const q = within(container);
    await start(q);
    opts.onEnded();
    await vi.waitFor(() => expect(q.getByLabelText('message').value).toBe('hello world'));
    expect(q.getByTestId('voice-note').textContent).toMatch(/stopped early/i);
  });

  it('says nothing heard when the transcript is empty', async () => {
    mockTranscribe.mockResolvedValue('');
    const { container } = render(Composer, { props: { status: 'idle', busy: false, onSend: () => {} } });
    const q = within(container);
    await start(q);
    await stop(q);
    await vi.waitFor(() => expect(q.getByTestId('voice-note').textContent).toMatch(/no speech/i));
  });

  it('hides the mic when voice is off in Settings', async () => {
    writeVoicePrefs({ enabled: false, cleanup: false });
    const { container } = render(Composer, { props: { status: 'idle', busy: false, onSend: () => {} } });
    const q = within(container);
    expect(q.queryByLabelText('record voice')).toBeNull();
    writeVoicePrefs({ enabled: true, cleanup: false });
    await tick();
    expect(q.getByLabelText('record voice')).toBeTruthy();
  });

  it('runs the cleanup pass when Settings turns it on', async () => {
    writeVoicePrefs({ enabled: true, cleanup: true });
    const { container } = render(Composer, { props: { status: 'idle', busy: false, onSend: () => {} } });
    const q = within(container);
    await start(q);
    await stop(q);
    await vi.waitFor(() => expect(mockTranscribe).toHaveBeenCalledWith(expect.any(Blob), { format: true }));
  });
});

vi.mock('./api.js', () => ({
  listImages: vi.fn(async () => ({ images: [{ id: 'img1', name: 'a.png', url: '/api/sessions/s1/images/img1' }] })),
  imageSrc: (sessionId, imageId) => `/api/sessions/${sessionId}/images/${imageId}`,
}));

describe('Composer stored image references', () => {
  it('references a gallery image in the tray and sends its id', async () => {
    const onSend = vi.fn();
    const { container } = render(Composer, { props: { status: 'idle', busy: false, sessionId: 's1', onSend } });
    const q = within(container);
    await fireEvent.click(q.getByLabelText('Session image gallery'));
    await tick();
    await fireEvent.click(q.getByTitle('Reference a.png'));
    await tick();
    expect(q.getByTestId('composer-tray')).toBeTruthy();
    await fireEvent.input(q.getByLabelText('message'), { target: { value: 'see this' } });
    await tick();
    await fireEvent.click(q.getByText('Send'));
    await tick();
    expect(onSend).toHaveBeenCalledWith('see this', 'prompt', [], ['img1']);
  });
});

describe('Composer remember button', () => {
  it('sends the draft at once with the /remember prefix', async () => {
    const onSend = vi.fn();
    const { container } = render(Composer, { props: { status: 'idle', busy: false, onSend } });
    const q = within(container);
    await fireEvent.input(q.getByLabelText('message'), { target: { value: 'use tabs' } });
    await tick();
    await fireEvent.click(q.getByLabelText('remember as rule'));
    await tick();
    expect(onSend).toHaveBeenCalledWith('/remember use tabs', 'prompt', [], []);
  });
  it('prefixes an empty draft for editing instead of sending', async () => {
    const onSend = vi.fn();
    const { container } = render(Composer, { props: { status: 'idle', busy: false, onSend } });
    const q = within(container);
    await fireEvent.click(q.getByLabelText('remember as rule'));
    await tick();
    expect(onSend).not.toHaveBeenCalled();
    expect(q.getByLabelText('message').value).toBe('/remember ');
  });
  it('toggles a prefixed draft back to a normal message', async () => {
    const onSend = vi.fn();
    const { container } = render(Composer, { props: { status: 'idle', busy: false, onSend } });
    const q = within(container);
    await fireEvent.input(q.getByLabelText('message'), { target: { value: '/remember use tabs' } });
    await tick();
    await fireEvent.click(q.getByLabelText('remember as rule'));
    await tick();
    expect(onSend).not.toHaveBeenCalled();
    expect(q.getByLabelText('message').value).toBe('use tabs');
  });
});
