import { afterEach, describe, expect, it, vi } from 'vitest';
import { createRunsStore } from './runsStore.js';

afterEach(() => vi.useRealTimers());

describe('runsStore', () => {
  it('shares its initial fetch and coalesces a Run-event burst', async () => {
    vi.useFakeTimers();
    let publish;
    const stopChannel = vi.fn();
    const fetch = vi.fn()
      .mockResolvedValueOnce({ runs: [{ id: 'run-initial' }] })
      .mockResolvedValueOnce({ runs: [{ id: 'run-updated' }] });
    const store = createRunsStore({
      fetch,
      subscribe: (onChange) => {
        publish = onChange;
        return stopChannel;
      },
    }, { coalesceMs: 50 });
    const firstValues = [];
    const secondValues = [];
    const stopFirst = store.subscribe((runs) => firstValues.push(runs));
    const stopSecond = store.subscribe((runs) => secondValues.push(runs));

    await store.load();
    expect(fetch).toHaveBeenCalledOnce();
    expect(firstValues.at(-1).map((run) => run.id)).toEqual(['run-initial']);
    expect(secondValues.at(-1).map((run) => run.id)).toEqual(['run-initial']);

    // Initial connection hello is already represented by the first fetch.
    publish('hello');
    await vi.advanceTimersByTimeAsync(50);
    expect(fetch).toHaveBeenCalledOnce();

    publish('runs');
    await vi.advanceTimersByTimeAsync(30);
    publish('runs');
    await vi.advanceTimersByTimeAsync(19);
    publish('runs');
    expect(fetch).toHaveBeenCalledOnce();
    await vi.advanceTimersByTimeAsync(1);
    expect(fetch).toHaveBeenCalledTimes(2);
    expect(firstValues.at(-1).map((run) => run.id)).toEqual(['run-updated']);
    expect(secondValues.at(-1).map((run) => run.id)).toEqual(['run-updated']);

    await vi.advanceTimersByTimeAsync(50);
    expect(fetch).toHaveBeenCalledTimes(2);
    stopFirst();
    stopSecond();
    expect(stopChannel).toHaveBeenCalledOnce();
  });

  it('queues one coalesced refresh when Run events arrive during a list request', async () => {
    vi.useFakeTimers();
    let publish;
    let resolveInitial;
    const fetch = vi.fn()
      .mockImplementationOnce(() => new Promise((resolve) => { resolveInitial = resolve; }))
      .mockResolvedValueOnce({ runs: [{ id: 'run-latest' }] });
    const store = createRunsStore({
      fetch,
      subscribe: (onChange) => {
        publish = onChange;
        return () => {};
      },
    }, { coalesceMs: 50 });
    const values = [];
    const unsubscribe = store.subscribe((runs) => values.push(runs));
    await Promise.resolve();
    expect(fetch).toHaveBeenCalledOnce();

    publish('runs');
    publish('runs');
    await vi.advanceTimersByTimeAsync(50);
    expect(fetch).toHaveBeenCalledOnce();

    resolveInitial({ runs: [{ id: 'run-stale' }] });
    await vi.advanceTimersByTimeAsync(0);
    expect(fetch).toHaveBeenCalledTimes(2);
    expect(values.at(-1).map((run) => run.id)).toEqual(['run-latest']);
    unsubscribe();
  });
});
