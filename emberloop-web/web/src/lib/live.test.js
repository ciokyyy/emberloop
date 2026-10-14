import { describe, it, expect, vi, afterEach } from 'vitest';
import { watch, stopLive } from './live.js';

afterEach(() => {
  stopLive();
  vi.unstubAllGlobals();
});

describe('live channel', () => {
  it('closes both streams and drops every watcher on stopLive', () => {
    const close = vi.fn();
    vi.stubGlobal('EventSource', class { close() { close(); } });
    watch('sessions', () => {});
    expect(close).not.toHaveBeenCalled();
    stopLive();
    expect(close).toHaveBeenCalledOnce();
  });

  it('opens a fresh stream for the next watch after stopLive', () => {
    const opened = vi.fn();
    vi.stubGlobal('EventSource', class { constructor() { opened(); } close() {} });
    watch('sessions', () => {});
    stopLive();
    watch('sessions', () => {});
    expect(opened).toHaveBeenCalledTimes(2);
  });

  it('keeps the shared EventSource when ordinary topic watchers change', () => {
    const streams = [];
    vi.stubGlobal('EventSource', class {
      constructor(url) { this.url = url; this.close = vi.fn(); streams.push(this); }
    });
    const stopSessions = watch('sessions', () => {});
    const stopSession = watch('session:one', () => {});
    expect(streams).toHaveLength(1);
    expect(streams[0].url).toMatch(/\/api\/live$/);
    stopSession();
    expect(streams).toHaveLength(1);
    expect(streams[0].close).not.toHaveBeenCalled();
    stopSessions();
  });

  it('opens a quota-only stream and forwards provider payloads', () => {
    const streams = [];
    vi.stubGlobal('EventSource', class {
      constructor(url) { this.url = url; this.close = vi.fn(); streams.push(this); }
    });
    const update = vi.fn();
    const stop = watch('quota', update);
    expect(streams).toHaveLength(2);
    expect(streams[0].url).toMatch(/\/api\/live$/);
    expect(streams[1].url).toContain('topic=quota');
    const message = { topic: 'quota', providers: [{ key: 'claude' }] };
    streams[1].onmessage({ data: JSON.stringify(message) });
    expect(update).toHaveBeenCalledWith('quota', message);
    stop();
    expect(streams[1].close).toHaveBeenCalledOnce();
  });
});
