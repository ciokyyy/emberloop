import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { transcribe, startRecording, makePeaks, STT_MAX_BYTES, WAVE_BARS } from './stt.js';

describe('transcribe', () => {
  beforeEach(() => {
    globalThis.fetch = vi.fn(async () => ({
      ok: true,
      json: async () => ({ text: '  hello world  ' }),
    }));
  });
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it('posts the recording with ?format=0 by default and trims', async () => {
    const out = await transcribe(new Blob(['audio'], { type: 'audio/webm;codecs=opus' }));
    expect(out).toBe('hello world');
    expect(fetch).toHaveBeenCalledOnce();
    const [url, opts] = fetch.mock.calls[0];
    // The cleanup pass stays off unless asked: it adds a model round trip.
    expect(url).toBe('/api/stt?format=0');
    expect(opts.method).toBe('POST');
    expect(opts.headers['Content-Type']).toBe('audio/webm;codecs=opus');
    expect(opts.body).toBeInstanceOf(Blob);
  });

  it('runs the cleanup pass with format:true', async () => {
    await transcribe(new Blob(['audio']), { format: true });
    expect(fetch.mock.calls[0][0]).toBe('/api/stt?format=1');
  });

  it('throws the Daemon reason on failure', async () => {
    globalThis.fetch = vi.fn(async () => ({
      ok: false,
      status: 502,
      json: async () => ({ error: 'no credits' }),
    }));
    await expect(transcribe(new Blob(['x']))).rejects.toThrow('no credits');
  });

  it('refuses an over-cap recording without calling the Daemon', async () => {
    const big = { size: STT_MAX_BYTES + 1, type: 'audio/webm' };
    await expect(transcribe(big)).rejects.toThrow('25 MB');
    expect(fetch).not.toHaveBeenCalled();
  });
});

describe('makePeaks', () => {
  it('returns the floor for silence and for no samples', () => {
    expect(makePeaks([])).toEqual(new Array(WAVE_BARS).fill(6));
    expect(makePeaks([0, 0, 0])).toEqual(new Array(WAVE_BARS).fill(6));
  });
  it('scales the loudest bar to 100 and keeps order', () => {
    const levels = [...new Array(48).fill(0.1), ...new Array(48).fill(0.5)];
    const p = makePeaks(levels, 2);
    expect(p).toEqual([20, 100]);
  });
});

class MockRecorder {
  static instances = [];
  static isTypeSupported = () => true;
  constructor(stream, opts) {
    this.stream = stream;
    this.mimeType = opts?.mimeType || 'audio/webm';
    this.state = 'inactive';
    MockRecorder.instances.push(this);
  }
  start() {
    this.state = 'recording';
    queueMicrotask(() => this.ondataavailable?.({ data: new Blob(['audio']) }));
  }
  stop() {
    this.state = 'inactive';
    queueMicrotask(() => {
      this.ondataavailable?.({ data: new Blob(['tail']) });
      this.onstop?.();
    });
  }
}

function mockMic() {
  const tracks = [{ stop: vi.fn(), onended: null }];
  Object.defineProperty(globalThis.navigator, 'mediaDevices', {
    value: { getUserMedia: async () => ({ getTracks: () => tracks }) },
    configurable: true,
  });
  globalThis.MediaRecorder = MockRecorder;
  MockRecorder.instances = [];
  return tracks;
}

function mockWakeLock() {
  const release = vi.fn(async () => {});
  const request = vi.fn(async () => ({ release }));
  Object.defineProperty(globalThis.navigator, 'wakeLock', { value: { request }, configurable: true });
  return { request, release };
}

describe('startRecording', () => {
  let tracks;
  beforeEach(() => {
    tracks = mockMic();
    delete globalThis.window.AudioContext;
  });
  afterEach(() => {
    delete globalThis.MediaRecorder;
    delete globalThis.navigator.wakeLock;
    delete globalThis.window.AudioContext;
    vi.useRealTimers();
  });

  it('records one take and returns blob, duration and peaks', async () => {
    const rec = await startRecording();
    await new Promise((r) => setTimeout(r, 20));
    const out = await rec.stop();
    expect(MockRecorder.instances).toHaveLength(1);
    expect(out.blob.size).toBeGreaterThan(0);
    expect(out.durationMs).toBeGreaterThanOrEqual(0);
    expect(out.peaks).toHaveLength(WAVE_BARS);
    expect(tracks[0].stop).toHaveBeenCalled();
  });

  it('holds the screen awake while recording and releases it on stop', async () => {
    const wl = mockWakeLock();
    const rec = await startRecording();
    await vi.waitFor(() => expect(wl.request).toHaveBeenCalledWith('screen'));
    await rec.stop();
    await vi.waitFor(() => expect(wl.release).toHaveBeenCalled());
  });

  it('asks for the lock again when the tab becomes visible', async () => {
    const wl = mockWakeLock();
    const rec = await startRecording();
    await vi.waitFor(() => expect(wl.request).toHaveBeenCalledTimes(1));
    document.dispatchEvent(new Event('visibilitychange'));
    await vi.waitFor(() => expect(wl.request).toHaveBeenCalledTimes(2));
    rec.cancel();
  });

  it('records on when wake lock is unsupported', async () => {
    const rec = await startRecording();
    const out = await rec.stop();
    expect(out.blob.size).toBeGreaterThan(0);
  });

  it('reports an interrupted take and still yields the audio so far', async () => {
    const onEnded = vi.fn();
    const rec = await startRecording({ onEnded });
    await new Promise((r) => setTimeout(r, 5));
    tracks[0].onended();
    expect(onEnded).toHaveBeenCalledOnce();
    const out = await rec.stop();
    expect(out.blob.size).toBeGreaterThan(0);
  });

  it('cancel drops the take and releases the mic', async () => {
    const rec = await startRecording();
    rec.cancel();
    expect(tracks[0].stop).toHaveBeenCalled();
  });
});
