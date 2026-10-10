import { describe, it, expect } from 'vitest';
import { highlightCode, highlightJson, highlightDiff, diffLineKind, highlightShell, highlightOutput, highlightGrepLine, highlightFileLine, highlightAnsi, looksJson } from './highlight.js';

const text = (segs) => segs.map((s) => s.text).join('');
const cls = (segs, t) => segs.find((s) => s.text === t)?.cls;

describe('highlight', () => {
  it('keeps the line text intact', () => {
    for (const fn of [highlightCode, highlightJson, highlightDiff, highlightShell, highlightOutput, highlightGrepLine, highlightFileLine, highlightAnsi]) {
      for (const l of ['const x = "a b" // hi', '  {"k": [1, true, null]}', '+added line', 'git status --short && ls ~/x', 'a.go:12:func main()', '  12→ return 1', '']) {
        expect(text(fn(l))).toBe(l);
      }
    }
  });
  it('colors code tokens', () => {
    const s = highlightCode('return foo("x", 42)');
    expect(cls(s, 'return')).toBe('tk-kw');
    expect(cls(s, 'foo')).toBe('tk-fn');
    expect(cls(s, '"x"')).toBe('tk-str');
    expect(cls(s, '42')).toBe('tk-num');
  });
  it('colors json keys apart from strings', () => {
    const s = highlightJson('  "path": "/a/b",');
    expect(cls(s, '"path"')).toBe('tk-key');
    expect(cls(s, '"/a/b"')).toBe('tk-str');
  });
  it('classifies diff lines', () => {
    expect(diffLineKind('+x')).toBe('add');
    expect(diffLineKind('-x')).toBe('del');
    expect(diffLineKind('+++ b/x')).toBe('meta');
    expect(diffLineKind('@@ -1 +1 @@')).toBe('hunk');
    expect(highlightDiff('+  return 1')[0].cls).toBe('tk-add-sign');
  });
  it('colors shell commands', () => {
    const s = highlightShell('git commit -m "x" && ls');
    expect(cls(s, 'git')).toBe('tk-fn');
    expect(cls(s, '-m')).toBe('tk-flag');
    expect(cls(s, '"x"')).toBe('tk-str');
    expect(cls(s, '&&')).toBe('tk-op');
  });
  it('colors output status and ansi', () => {
    expect(highlightOutput('FAIL example.com/x')[0].cls).toBe('tk-err');
    expect(highlightOutput('ok  example.com/x 0.1s')[0].cls).toBe('tk-ok');
    expect(highlightAnsi('\x1b[31mred\x1b[0m plain')[0]).toEqual({ text: 'red', cls: 'tk-err' });
  });
  it('colors grep and file lines', () => {
    const g = highlightGrepLine('src/a.js:12:const x = 1');
    expect(g[0]).toEqual({ text: 'src/a.js', cls: 'tk-path' });
    expect(highlightFileLine('  12→ const a = 1')[0].cls).toBe('tk-dim');
  });
  it('detects json blocks', () => {
    expect(looksJson(['{', '"a": 1', '}'])).toBe(true);
    expect(looksJson(['hello'])).toBe(false);
  });
});
