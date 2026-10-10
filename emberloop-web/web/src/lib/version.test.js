import { describe, it, expect, vi } from 'vitest';
import { fetchVersion, isStale, uiHash, versionText, copyText } from './version.js';

describe('fetchVersion', () => {
  it('resolves the build identity', async () => {
    const body = { version: 'dev', commit: 'abc1234', uiHash: '0123456789ab' };
    await expect(fetchVersion(async () => body)).resolves.toEqual(body);
  });

  it('resolves null on a bad shape, a failure, or a throw', async () => {
    await expect(fetchVersion(async () => [])).resolves.toBeNull();
    await expect(fetchVersion(async () => ({ commit: 'abc1234' }))).resolves.toBeNull();
    await expect(fetchVersion(async () => ({ uiHash: '0123456789ab' }))).resolves.toBeNull();
    await expect(
      fetchVersion(async () => {
        throw new Error('down');
      }),
    ).resolves.toBeNull();
  });
});

describe('isStale', () => {
  it('is stale only when both hashes are known and differ', () => {
    expect(isStale('aaa', 'bbb')).toBe(true);
    expect(isStale('aaa', 'aaa')).toBe(false);
    expect(isStale('dev', 'bbb')).toBe(false);
    expect(isStale('aaa', 'dev')).toBe(false);
    expect(isStale('aaa', '')).toBe(false);
    expect(isStale('', 'bbb')).toBe(false);
  });
});

describe('versionText', () => {
  it('renders the badge line', () => {
    expect(versionText('0123456789ab', 'abc1234')).toBe('ui 0123456789ab · be abc1234');
  });
});

describe('uiHash', () => {
  it('falls back to dev outside a stamped build', () => {
    expect(typeof uiHash()).toBe('string');
  });
});

describe('copyText', () => {
  it('uses the async clipboard when present', async () => {
    const writeText = vi.fn(async () => {});
    vi.stubGlobal('navigator', { clipboard: { writeText } });
    await expect(copyText('ui a · be b')).resolves.toBe(true);
    expect(writeText).toHaveBeenCalledWith('ui a · be b');
    vi.unstubAllGlobals();
  });

  it('resolves false when nothing can copy', async () => {
    vi.stubGlobal('navigator', {});
    await expect(copyText('ui a · be b')).resolves.toBe(false);
    vi.unstubAllGlobals();
  });
});
