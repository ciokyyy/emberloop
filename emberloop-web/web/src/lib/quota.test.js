import { describe, it, expect } from 'vitest';
import {
  highestPercent,
  quotaHint,
  quotaPlan,
  quotaSource,
  quotaState,
  quotaUpdatedLine,
  resetCountdown,
  staleLabel,
  thresholdClass,
  wordLabel,
} from './quota.js';

const NOW = Date.UTC(2030, 0, 1);
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

  it('treats a missing or invalid percent as calm', () => {
    expect(thresholdClass(undefined)).toBe('calm');
    expect(thresholdClass(null)).toBe('calm');
    expect(thresholdClass('not a percent')).toBe('calm');
  });
});


describe('wordLabel', () => {
  it('formats provider window labels in words and leaves custom labels intact', () => {
    expect(wordLabel('5h')).toBe('5-hour');
    expect(wordLabel('five_hour')).toBe('5-hour');
    expect(wordLabel('wk')).toBe('Weekly');
    expect(wordLabel('seven_day')).toBe('Weekly');
    expect(wordLabel('mo')).toBe('Monthly');
    expect(wordLabel('Credits')).toBe('Credits');
  });
});

describe('resetCountdown', () => {
  it('formats hours, days and minutes from epoch seconds', () => {
    expect(resetCountdown(secs(3600), NOW)).toBe('1h00m');
    expect(resetCountdown(secs(6000), NOW)).toBe('1h40m');
    expect(resetCountdown(secs(1500), NOW)).toBe('25m');
    expect(resetCountdown(secs(6 * 86400), NOW)).toBe('6d0h');
    expect(resetCountdown(secs(90000), NOW)).toBe('1d1h');
  });

  it('accepts ISO reset times and reads now or blank when appropriate', () => {
    expect(resetCountdown(new Date(secs(3600) * 1000).toISOString(), NOW)).toBe('1h00m');
    expect(resetCountdown(secs(-5), NOW)).toBe('now');
    expect(resetCountdown(0, NOW)).toBe('');
    expect(resetCountdown(undefined, NOW)).toBe('');
  });
});

describe('staleLabel', () => {
  it('renders update ages in useful units', () => {
    expect(staleLabel(NOW / 1000 - 8, NOW)).toBe('updated 8s ago');
    expect(staleLabel(NOW / 1000 - 300, NOW)).toBe('updated 5m ago');
    expect(staleLabel(NOW / 1000 - 7200, NOW)).toBe('updated 2h ago');
    expect(staleLabel(NOW / 1000 - 3 * 86400, NOW)).toBe('updated 3d ago');
    expect(staleLabel(NOW / 1000, NOW)).toBe('updated just now');
  });

  it('reads never updated when there is no timestamp', () => {
    expect(staleLabel(0, NOW)).toBe('never updated');
    expect(staleLabel(undefined, NOW)).toBe('never updated');
  });
});

describe('provider presentation', () => {
  it('provides safe contract fallbacks without deriving values', () => {
    const provider = { key: 'agy-gemini', state: 'not_configured' };
    expect(quotaPlan({ key: 'claude' })).toBe('Pro');
    expect(quotaSource({ key: 'claude' })).toBe('Host login');
    expect(quotaState(provider)).toBe('not_connected');
    expect(quotaHint(provider)).toContain('~/.el/agy-oauth.json');
    expect(quotaHint(provider)).toContain('Claude + GPT-OSS pools');
    expect(quotaPlan({ key: 'agy-gemini' })).toBe('Gemini + 3P');
    expect(quotaUpdatedLine({ key: 'opencode-go', state: 'stale', updated_at: NOW / 1000 - 720 }, NOW)).toBe('updated 12m ago · retrying');
    expect(quotaUpdatedLine({ key: 'claude', state: 'expired', updated_at: NOW / 1000 - 3600 }, NOW)).toBe('last read 1h ago');
    expect(quotaPlan({ key: 'openrouter', plan: 'Credits' })).toBe('Credits');
  });

  it('keeps cached error data in the stale presentation', () => {
    expect(quotaState({ state: 'error', windows: [{ label: '5h', pct: 22 }] })).toBe('stale');
    expect(quotaState({ state: 'error', windows: [] })).toBe('error');
  });
});

describe('highestPercent', () => {
  it('uses the maximum integer pct across provider windows', () => {
    expect(highestPercent({ windows: [{ pct: 36 }, { pct: 94 }] })).toBe(94);
    expect(highestPercent({ windows: [{ pct: 0 }, { pct: 100 }] })).toBe(100);
    expect(highestPercent({ windows: [] })).toBeNull();
  });
});
