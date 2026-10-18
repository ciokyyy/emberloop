import { afterEach, describe, expect, it, vi } from 'vitest';
import { backOpen, closeBacks, holdBack, popBack, resetBackStack, selfPopPending, takeSelfPop } from './overlayBack.js';

afterEach(() => {
  resetBackStack();
});

describe('overlay back stack', () => {
  it('takes one entry while a sheet is open and drops it when the sheet closes', () => {
    const back = vi.spyOn(history, 'back').mockImplementation(() => {});
    const close = vi.fn();
    const release = holdBack(close);
    expect(backOpen()).toBe(true);
    expect(history.state?.elSheet).toBe(1);

    release();
    expect(backOpen()).toBe(false);
    expect(back).toHaveBeenCalledTimes(1);
    expect(selfPopPending()).toBe(true);
    expect(takeSelfPop()).toBe(true);
    expect(selfPopPending()).toBe(false);
  });

  it('closes the top sheet on Back and keeps the entry for the sheet underneath', () => {
    const back = vi.spyOn(history, 'back').mockImplementation(() => {});
    const first = vi.fn();
    const second = vi.fn();
    const releaseFirst = holdBack(first);
    const releaseSecond = holdBack(second);
    // One entry serves the whole stack; a second sheet reuses it.
    expect(history.state?.elSheet).toBe(1);

    expect(popBack()).toBe(true);
    expect(second).toHaveBeenCalledTimes(1);
    expect(first).not.toHaveBeenCalled();
    expect(backOpen()).toBe(true);
    // Back consumed the entry, so the sheet underneath takes a fresh one.
    expect(history.state?.elSheet).toBe(1);

    // The closed sheet's cleanup leaves the surviving entry alone.
    releaseSecond();
    expect(back).not.toHaveBeenCalled();

    expect(popBack()).toBe(true);
    expect(first).toHaveBeenCalledTimes(1);
    expect(backOpen()).toBe(false);
    releaseFirst();
  });

  it('closes every sheet for a navigation without starting a Back traversal', () => {
    const back = vi.spyOn(history, 'back').mockImplementation(() => {});
    const close = vi.fn();
    holdBack(close);
    expect(closeBacks()).toBe(1);
    expect(close).toHaveBeenCalledTimes(1);
    expect(back).not.toHaveBeenCalled();
    expect(backOpen()).toBe(false);
  });

  it('ignores a popstate for a sheet that already closed itself', () => {
    const back = vi.spyOn(history, 'back').mockImplementation(() => {});
    const close = vi.fn();
    const release = holdBack(close);
    release();
    expect(takeSelfPop()).toBe(true);
    const other = vi.fn();
    holdBack(other);
    window.dispatchEvent(new PopStateEvent('popstate'));
    expect(other).toHaveBeenCalledTimes(1);
    expect(back).toHaveBeenCalledTimes(1);
  });

  it('does nothing when no sheet is open', () => {
    expect(popBack()).toBe(false);
    expect(backOpen()).toBe(false);
  });
});
