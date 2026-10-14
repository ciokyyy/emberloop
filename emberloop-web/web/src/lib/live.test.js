import { describe, it, expect, vi, afterEach } from 'vitest';
import { watch, stopLive } from './live.js';

afterEach(() => {
  stopLive();
  vi.unstubAllGlobals();
});

describe('live channel', () => {
  it('closes the stream and drops every watcher on stopLive', () => {
    const close = vi.fn();
    vi.stubGlobal('EventSource', class { close() { close(); } });
    watch('sessions', () => {});
    expect(close).not.toHaveBeenCalled();
    stopLive();
    expect(close).toHaveBeenCalledTimes(1);
  });

  it('opens a fresh stream for the next watch after stopLive', () => {
    const opened = vi.fn();
    vi.stubGlobal('EventSource', class { constructor() { opened(); } close() {} });
    watch('sessions', () => {});
    stopLive();
    watch('sessions', () => {});
    expect(opened).toHaveBeenCalledTimes(2);
  });

  it('sends watched topics and forwards quota provider payloads', () => {
    let stream;
    vi.stubGlobal('EventSource', class {
      constructor(url) { this.url = url; stream = this; }
      close() {}
    });
    const update = vi.fn();
    const stop = watch('quota', update);
    expect(stream.url).toContain('topic=quota');
    const message = { topic: 'quota', providers: [{ key: 'claude' }] };
    stream.onmessage({ data: JSON.stringify(message) });
    expect(update).toHaveBeenCalledWith('quota', message);
    stop();
  });
});
