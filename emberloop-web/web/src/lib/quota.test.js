import { describe, it, expect } from 'vitest';
import { thresholdClass, resetCountdown, staleLabel } from './quota.js';

// Fixed clock so every relative string is deterministic.
const NOW = Date.UTC(2030, 0, 1); // ms
const secs = (s) => NOW / 1000 + s;

describe('thresholdClass', () => {
  it('mirrors the header strip cuts at 60 and 80', () => {
    expect(thresholdClass(0)).toBe('calm');
    expect(thresholdClass(59)).toBe('calm');
    expect(thresholdClass(60)).toBe('warm');
    expect(thresholdClass(79)).toBe('warm');
    expect(thresholdClass(80)).toBe('hot');
    expect(thresholdClass(100)).toBe('hot');
  });

  it('treats a missing percent as calm', () => {
    expect(thresholdClass(undefined)).toBe('calm');
    expect(thresholdClass(null)).toBe('calm');
  });
});

describe('resetCountdown', () => {
  it('mirrors usageResetIn formats', () => {
    expect(resetCountdown(secs(3600), NOW)).toBe('1h00m');
    expect(resetCountdown(secs(6000), NOW)).toBe('1h40m');
    expect(resetCountdown(secs(1500), NOW)).toBe('25m');
    expect(resetCountdown(secs(6 * 86400), NOW)).toBe('6d0h');
    expect(resetCountdown(secs(90000), NOW)).toBe('1d1h');
  });

  it('reads now once the reset passed and blank without one', () => {
    expect(resetCountdown(secs(-5), NOW)).toBe('now');
    expect(resetCountdown(0, NOW)).toBe('');
    expect(resetCountdown(undefined, NOW)).toBe('');
  });
});

describe('staleLabel', () => {
  it('renders the age in the coarsest useful unit', () => {
    expect(staleLabel(NOW / 1000 - 10, NOW)).toBe('updated just now');
    expect(staleLabel(NOW / 1000 - 300, NOW)).toBe('updated 5m ago');
    expect(staleLabel(NOW / 1000 - 7200, NOW)).toBe('updated 2h ago');
    expect(staleLabel(NOW / 1000 - 3 * 86400, NOW)).toBe('updated 3d ago');
  });

  it('reads never updated with no file', () => {
    expect(staleLabel(0, NOW)).toBe('never updated');
    expect(staleLabel(undefined, NOW)).toBe('never updated');
  });
});
