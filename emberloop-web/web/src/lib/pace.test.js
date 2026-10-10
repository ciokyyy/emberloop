import { describe, it, expect } from 'vitest';
import { nextShown, safeCut, MIN_STEP } from './pace.js';

describe('pace', () => {
  it('moves at least MIN_STEP and never past the target', () => {
    expect(nextShown(0, 2)).toBe(2);
    expect(nextShown(0, 100)).toBe(Math.max(MIN_STEP, 6));
    expect(nextShown(10, 10)).toBe(10);
  });
  it('drains a big burst in bounded frames', () => {
    let n = 0;
    let frames = 0;
    while (n < 5000 && frames < 1000) {
      n = nextShown(n, 5000);
      frames++;
    }
    expect(n).toBe(5000);
    expect(frames).toBeLessThan(150);
  });
  it('does not cut a surrogate pair', () => {
    expect(safeCut('a😀b', 2)).toBe('a');
    expect(safeCut('abc', 5)).toBe('abc');
  });
});
