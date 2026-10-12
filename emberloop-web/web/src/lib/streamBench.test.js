import { describe, it, expect } from 'vitest';
import { groupTranscript, createTranscriptCache, groupTranscriptCached, latestUsageCached, createUsageCache } from './events.js';
import { renderMarkdown, createIncrementalMarkdown } from './markdown.js';
import { nextShown, MAX_BACKLOG } from './pace.js';
import { fadeNewText } from './streamFade.js';
import { memoSegs } from './toolstrip.js';
import { highlightOutput } from './highlight.js';
import { appendText } from './appendText.js';

// Synthetic fast-model stream: one prompt then N chunk deltas.
export function synthStream(n, { chunkChars = 24 } = {}) {
  const events = [{ seq: 1, kind: 'prompt', text: 'go', mode: 'prompt' }];
  for (let i = 0; i < n; i++) {
    const kind = i % 10 === 9 ? 'thinking_chunk' : 'assistant_chunk';
    events.push({ seq: i + 2, kind, text: `delta ${i} ` + 'x'.repeat(chunkChars) });
  }
  return events;
}

function median(a) {
  const s = [...a].sort((x, y) => x - y);
  return s[Math.floor(s.length / 2)];
}

describe('stream bench', () => {
  for (const n of [50, 200, 1000]) {
    it(`groupTranscript single flush at ${n} deltas`, () => {
      const events = synthStream(n);
      const samples = [];
      for (let k = 0; k < 7; k++) {
        const t0 = performance.now();
        groupTranscript(events, {}, { working: true });
        samples.push(performance.now() - t0);
      }
      console.log(`BENCH groupTranscript n=${n} p50=${median(samples).toFixed(2)}ms`);
    });
  }

  it('old behavior: regroup every prefix (flush per delta)', () => {
    const n = 200;
    const events = synthStream(n);
    let parses = 0;
    const origParse = JSON.parse;
    JSON.parse = (...a) => {
      parses++;
      return origParse(...a);
    };
    try {
      const t0 = performance.now();
      for (let i = 1; i <= n; i++) groupTranscript(events.slice(0, i + 1), {}, { working: true });
      const dt = performance.now() - t0;
      console.log(
        `BENCH old-prefixes n=${n} total=${dt.toFixed(1)}ms p50flush=${(dt / n).toFixed(3)}ms parses=${parses}`,
      );
    } finally {
      JSON.parse = origParse;
    }
  });

  it('old behavior: full markdown re-parse per delta', { timeout: 60000 }, async () => {
    const { marked } = await import('marked');
    const { default: DOMPurify } = await import('dompurify');
    marked.setOptions({ gfm: true, breaks: true });
    let text = '';
    const t0 = performance.now();
    for (let i = 0; i < 200; i++) {
      text += `delta ${i} with **bold** and \`code\` plus more words to grow the message\n\n`;
      DOMPurify.sanitize(marked.parse(text, { async: false }));
    }
    console.log(
      `BENCH markdown-old 200 deltas total=${(performance.now() - t0).toFixed(1)}ms final=${(text.length / 1024).toFixed(1)}KB`,
    );
  });
  it('new behavior: coalesced cached flushes over 1000 deltas', () => {
    const n = 1000;
    const events = synthStream(n);
    const cache = createTranscriptCache();
    const sessionsById = {};
    const flushes = [];
    // 80ms coalescing ≈ 20 deltas per flush at fast-model rates.
    for (let i = 0; i < events.length; i += 20) {
      const slice = events.slice(0, i + 20);
      const t0 = performance.now();
      groupTranscriptCached(slice, sessionsById, { working: true }, cache);
      flushes.push(performance.now() - t0);
    }
    flushes.sort((a, b) => a - b);
    const p50 = flushes[Math.floor(flushes.length / 2)];
    const total = flushes.reduce((a, b) => a + b, 0);
    console.log(`BENCH new-coalesced n=${n} flushes=${flushes.length} p50flush=${p50.toFixed(3)}ms total=${total.toFixed(1)}ms`);
    expect(p50).toBeLessThan(2);
  });

  it('cached grouping matches full grouping (settled prefix stable)', () => {
    const events = synthStream(300);
    events.push({ seq: 1001, kind: 'tool_call_started', text: JSON.stringify({ id: 't1', name: 'bash', input: 'vitest', status: 'running' }) });
    for (let i = 0; i < 50; i++) {
      events.push({ seq: 1002 + i, kind: 'tool_call_updated', text: JSON.stringify({ id: 't1', name: 'bash', output: `line${i}\n`, status: 'running' }) });
    }
    const cache = createTranscriptCache();
    const sessionsById = {};
    let cached;
    for (let i = 0; i < events.length; i += 17) {
      cached = groupTranscriptCached(events.slice(0, i + 17), sessionsById, { working: true }, cache);
    }
    const full = groupTranscript(events, sessionsById, { working: true });
    const norm = (blocks) =>
      blocks.map((b) => {
        const { live, _t, ...rest } = b;
        return { ...rest, events: (rest.events ?? []).map((e) => e.seq) };
      });
    expect(norm(cached)).toEqual(norm(full));
    // Settled prefix keeps object identity across a later burst.
    const before = cached;
    const more = [...events, { seq: 2000, kind: 'tool_call_updated', text: JSON.stringify({ id: 't1', name: 'bash', output: 'more\n', status: 'running' }) }];
    const after = groupTranscriptCached(more, sessionsById, { working: true }, cache);
    expect(after.slice(0, -1).every((b, i) => b === before[i])).toBe(true);
  });

  it('parse count: incremental does O(deltas), regroup-every-flush does O(n^2)', { timeout: 60000 }, () => {
    const events = [{ seq: 1, kind: 'prompt', text: 'go', mode: 'prompt' }];
    for (let i = 0; i < 200; i++) {
      events.push({ seq: i + 2, kind: 'tool_call_updated', text: JSON.stringify({ id: 't1', name: 'bash', output: `out ${i} ` + 'y'.repeat(20), status: 'running' }) });
    }
    const countParses = (fn) => {
      let parses = 0;
      const orig = JSON.parse;
      JSON.parse = (...a) => {
        parses++;
        return orig(...a);
      };
      try {
        fn();
      } finally {
        JSON.parse = orig;
      }
      return parses;
    };
    const oldParses = countParses(() => {
      for (let i = 1; i <= events.length; i++) groupTranscript(events.slice(0, i), {}, { working: true });
    });
    // NOTE: sessionsById must keep its identity across flushes (as App's
    // derived does); a fresh {} per flush would invalidate the cache.
    const sessionsById = {};
    const newParses = countParses(() => {
      const cache = createTranscriptCache();
      for (let i = 1; i <= events.length; i++) groupTranscriptCached(events.slice(0, i), sessionsById, { working: true }, cache);
    });
    console.log(`BENCH parses n=200 old=${oldParses} new=${newParses}`);
    expect(newParses).toBeLessThan(oldParses / 10);
  });

  it('usage cache scans only the tail', () => {
    const events = synthStream(200);
    events.push({ seq: 500, kind: 'usage', text: JSON.stringify({ used: 10, size: 100 }) });
    const cache = createUsageCache();
    expect(latestUsageCached(events, cache)).toEqual({ used: 10, size: 100 });
    const more = [...events, ...synthStream(10).map((e, i) => ({ ...e, seq: 501 + i }))];
    expect(latestUsageCached(more, cache)).toEqual({ used: 10, size: 100 });
  });

  it('new behavior: incremental markdown per delta + exact settle', () => {
    let text = '';
    const parts = [];
    for (let i = 0; i < 200; i++) {
      text += `delta ${i} with **bold** and \`code\` plus more words to grow the message\n\n`;
      parts.push(text);
    }
    // A fenced block mid-stream exercises the fence-parity head split.
    parts.push(text + '```js\nconst a = 1;\n');
    parts.push(text + '```js\nconst a = 1;\n```\n\ntail *em* text');
    const incr = createIncrementalMarkdown();
    const t0 = performance.now();
    let html = '';
    for (const p of parts) html = incr.render(p);
    const dt = performance.now() - t0;
    console.log(`BENCH markdown-new ${parts.length} deltas total=${dt.toFixed(1)}ms head=${JSON.stringify(incr.stats())}`);
    const settled = incr.renderFull(parts[parts.length - 1]);
    expect(settled).toBe(renderMarkdown(parts[parts.length - 1]));
    expect(html).toContain('tail');
  });
  it('pace bounds the backlog so a fast model can catch up', () => {
    expect(nextShown(0, 100000)).toBe(100000 - MAX_BACKLOG);
    let shown = 100000 - MAX_BACKLOG;
    for (let i = 0; i < 200 && shown < 100000; i++) shown = nextShown(shown, 100000);
    expect(shown).toBe(100000);
  });

  it('fade skips wrapping past the backlog so fast streams do no DOM churn', () => {
    let t = 0;
    const f = fadeNewText(() => t);
    const root = () => {
      const el = document.createElement('div');
      el.innerHTML = '<p>hello</p>';
      return el;
    };
    f.apply(root());
    t = 100;
    const el = root();
    el.querySelector('p').textContent = 'hello world';
    f.apply(el, { backlog: 5000 });
    expect(el.querySelector('.stream-in')).toBeNull();
    t = 200;
    const el2 = root();
    el2.querySelector('p').textContent = 'hello world!!';
    f.apply(el2, { backlog: 10 });
    expect(el2.querySelector('.stream-in')).not.toBeNull();
  });

  it('highlight memo returns the same segments for repeated lines', () => {
    const a = memoSegs('out', 'ok 123 file.js', highlightOutput);
    const b = memoSegs('out', 'ok 123 file.js', highlightOutput);
    expect(a).toBe(b);
    expect(a.map((s) => s.text).join('')).toBe('ok 123 file.js');
  });

  it('appendText appends the tail without touching the head node', () => {
    const pre = document.createElement('pre');
    const action = appendText(pre, 'hello');
    const head = pre.firstChild;
    action.update('hello world');
    expect(pre.textContent).toBe('hello world');
    expect(pre.firstChild).toBe(head);
    action.update('replaced');
    expect(pre.textContent).toBe('replaced');
  });
});
