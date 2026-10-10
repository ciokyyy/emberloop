import { describe, it, expect, vi } from 'vitest';
import { isNearBottom, anchorDelta, applyAnchor, blockIndexForSeq, STICK_THRESHOLD } from './transcript.js';
import { getSession } from './api.js';

describe('stick-to-bottom', () => {
  it('pins within the threshold, releases above it', () => {
    expect(isNearBottom({ scrollHeight: 1000, scrollTop: 900, clientHeight: 100 })).toBe(true);
    expect(isNearBottom({ scrollHeight: 1000, scrollTop: 0, clientHeight: 100 })).toBe(false);
    expect(
      isNearBottom({ scrollHeight: 1000, scrollTop: 1000 - 100 - STICK_THRESHOLD + 1, clientHeight: 100 }),
    ).toBe(true);
    expect(
      isNearBottom({ scrollHeight: 1000, scrollTop: 1000 - 100 - STICK_THRESHOLD - 1, clientHeight: 100 }),
    ).toBe(false);
  });
});

describe('load-earlier anchoring', () => {
  it('keeps the viewport by adding grown height to scrollTop', () => {
    const el = { scrollHeight: 3000, scrollTop: 50 };
    applyAnchor(el, { scrollHeight: 1000, scrollTop: 50 });
    expect(el.scrollTop).toBe(2050);
    expect(anchorDelta({ scrollHeight: 1000 }, { scrollHeight: 1000 })).toBe(0);
  });
});

describe('blockIndexForSeq', () => {
  const blocks = [
    { key: 1, events: [{ seq: 1 }, { seq: 2 }] },
    { key: 3, events: [{ seq: 3 }] },
    { key: 4, events: [{ seq: 4 }, { seq: 5 }] },
  ];
  it('finds the block holding a seq', () => {
    expect(blockIndexForSeq(blocks, 1)).toBe(0);
    expect(blockIndexForSeq(blocks, 5)).toBe(2);
  });
  it('falls back to the nearest block at or before the seq', () => {
    expect(blockIndexForSeq(blocks, 99)).toBe(2);
    expect(blockIndexForSeq([], 1)).toBe(-1);
    expect(blockIndexForSeq(blocks, null)).toBe(-1);
  });
});

describe('load-earlier paging against a mocked api', () => {
  it('prepends the page and moves firstSeq back', async () => {
    const pages = {
      7: { events: [{ seq: 4 }, { seq: 5 }, { seq: 6 }], hasMore: true, firstSeq: 4 },
      4: { events: [{ seq: 1 }, { seq: 2 }, { seq: 3 }], hasMore: false, firstSeq: 1 },
    };
    vi.stubGlobal(
      'fetch',
      vi.fn(async (u) => ({
        ok: true,
        json: async () => pages[new URL(u, 'http://x').searchParams.get('before')] ?? { events: [] },
      })),
    );
    let events = [{ seq: 7 }, { seq: 8 }];
    let firstSeq = 7;
    let hasMore = true;
    while (hasMore) {
      const body = await getSession('s1', { before: firstSeq, turns: 6 });
      events = [...body.events, ...events];
      firstSeq = body.firstSeq;
      hasMore = body.hasMore;
    }
    expect(events.map((e) => e.seq)).toEqual([1, 2, 3, 4, 5, 6, 7, 8]);
    expect(firstSeq).toBe(1);
  });
});
