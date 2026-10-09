import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { transcribe, startLiveRecording, STT_MAX_BYTES } from './stt.js';

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

  it('posts one segment with ?format=1 by default and trims', async () => {
    const out = await transcribe(new Blob(['audio'], { type: 'audio/webm;codecs=opus' }));
    expect(out).toBe('hello world');
    expect(fetch).toHaveBeenCalledOnce();
    const [url, opts] = fetch.mock.calls[0];
    expect(url).toBe('/api/stt?format=1');
    expect(opts.method).toBe('POST');
    expect(opts.headers['Content-Type']).toBe('audio/webm;codecs=opus');
    expect(opts.body).toBeInstanceOf(Blob);
  });

  it('skips the cleanup pass with format:false', async () => {
    await transcribe(new Blob(['audio']), { format: false });
    expect(fetch.mock.calls[0][0]).toBe('/api/stt?format=0');
  });

  it('throws the Daemon reason on failure', async () => {
    globalThis.fetch = vi.fn(async () => ({
      ok: false,
      status: 502,
      json: async () => ({ error: 'no credits' }),
    }));
    await expect(transcribe(new Blob(['x']))).rejects.toThrow('no credits');
  });

  it('refuses an over-cap segment without calling the Daemon', async () => {
    const big = { size: STT_MAX_BYTES + 1, type: 'audio/webm' };
    await expect(transcribe(big)).rejects.toThrow('25 MB');
    expect(fetch).not.toHaveBeenCalled();
  });
});

// MediaRecorder mock: each instance is its own encoder, so blobs from
// different instances prove independently decodable segments.
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
    queueMicrotask(() => this.ondataavailable?.({ data: new Blob([`audio-${MockRecorder.instances.length}`]) }));
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
  const tracks = [{ stop: vi.fn() }];
  Object.defineProperty(globalThis.navigator, 'mediaDevices', {
    value: { getUserMedia: async () => ({ getTracks: () => tracks }) },
    configurable: true,
  });
  globalThis.MediaRecorder = MockRecorder;
  MockRecorder.instances = [];
  return tracks;
}

describe('startLiveRecording', () => {
  let tracks;
  beforeEach(() => {
    tracks = mockMic();
    vi.useRealTimers();
  });
  afterEach(() => {
    delete globalThis.MediaRecorder;
    vi.useRealTimers();
  });

  const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

  it('emits timed segments in order, each from its own recorder', async () => {
    const seen = [];
    const rec = await startLiveRecording({ onSegment: (blob, seq) => seen.push([blob, seq]), segmentMs: 60 });
    await sleep(250);
    await rec.stop();
    expect(seen.length).toBeGreaterThanOrEqual(2);
    expect(seen.map(([, seq]) => seq)).toEqual(seen.map((_, i) => i));
    for (const [blob] of seen) {
      expect(blob).toBeInstanceOf(Blob);
      expect(blob.size).toBeGreaterThan(0);
      expect(blob.type).toContain('audio/');
    }
    // One fresh MediaRecorder per segment: never slices of one recording.
    expect(MockRecorder.instances.length).toBeGreaterThanOrEqual(seen.length);
    expect(tracks[0].stop).toHaveBeenCalledOnce();
  });

  it('flushes one tail segment on immediate stop', async () => {
    const seen = [];
    const rec = await startLiveRecording({ onSegment: (blob, seq) => seen.push(seq), segmentMs: 60000 });
    await rec.stop();
    expect(seen).toEqual([0]);
  });

  it('cancel drops the tail and releases the mic', async () => {
    const seen = [];
    const rec = await startLiveRecording({ onSegment: (blob, seq) => seen.push(seq), segmentMs: 60000 });
    await sleep(20);
    rec.cancel();
    await sleep(50);
    expect(seen).toEqual([]);
    expect(tracks[0].stop).toHaveBeenCalledOnce();
  });

  it('requires onSegment', async () => {
    await expect(startLiveRecording()).rejects.toThrow('onSegment');
  });
});
