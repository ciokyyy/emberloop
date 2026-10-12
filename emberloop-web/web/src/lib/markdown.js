// Incremental Markdown rendering. A full marked.parse + DOMPurify.sanitize
// over the whole growing message per flush is O(L^2) total (and each render
// replaces the DOM, re-triggering the fade walk). The settled head — complete
// block runs behind a blank-line boundary with balanced code fences — parses
// once and is cached sanitized; only the growing tail re-parses per tick.
// renderFull parses everything exactly (settle + correctness backstop).
import { marked } from 'marked';
import DOMPurify from 'dompurify';

marked.setOptions({ gfm: true, breaks: true });

export function renderMarkdown(text) {
  return DOMPurify.sanitize(marked.parse(String(text ?? ''), { async: false }))
    .replaceAll('<table', '<div class="md-table"><table')
    .replaceAll('</table>', '</table></div>');
}

const FENCE = /^\s*```/;
const BLANK = /^\s*$/;

export function createIncrementalMarkdown() {
  let headLen = 0;
  let headText = '';
  let headHtml = '';
  let parity = 0; // ``` fences inside the head, mod 2

  function render(text) {
    text = String(text ?? '');
    if (text.slice(0, headLen) !== headText) {
      headLen = 0;
      headText = '';
      headHtml = '';
      parity = 0;
    }
    // Walk the unparsed tail line by line for the latest blank-line boundary
    // past the head with balanced fences; that boundary is a block edge, so
    // the head segment parses independently of the tail.
    let best = -1;
    let bestParity = parity;
    let p = parity;
    let off = headLen;
    const tail = text.slice(headLen);
    const lines = tail.split('\n');
    for (let li = 0; li < lines.length; li++) {
      const line = lines[li];
      const end = off + line.length;
      if (BLANK.test(line) && p === 0 && li < lines.length - 1 && end + 1 > headLen) {
        best = end + 1;
        bestParity = p;
      }
      if (FENCE.test(line)) p ^= 1;
      off = end + 1;
    }
    if (best > headLen) {
      headHtml += renderMarkdown(text.slice(headLen, best));
      headText = text.slice(0, best);
      headLen = best;
      parity = bestParity;
    }
    if (headLen === 0) return renderMarkdown(text);
    if (headLen >= text.length) return headHtml;
    return headHtml + renderMarkdown(text.slice(headLen));
  }

  function renderFull(text) {
    text = String(text ?? '');
    headLen = 0;
    headText = '';
    headHtml = '';
    parity = 0;
    return renderMarkdown(text);
  }

  function stats() {
    return { headLen, headHtml: headHtml.length };
  }

  return { render, renderFull, stats };
}
