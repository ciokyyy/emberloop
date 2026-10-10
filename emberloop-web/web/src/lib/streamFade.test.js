import { describe, it, expect } from 'vitest';
import { fadeNewText, FADE_MS } from './streamFade.js';

function root(text) {
  const el = document.createElement('div');
  el.innerHTML = `<p>${text}</p>`;
  return el;
}

describe('fadeNewText', () => {
  it('never animates the first render', () => {
    let t = 0;
    const f = fadeNewText(() => t);
    const el = root('hello');
    f.apply(el);
    expect(el.querySelector('.stream-in')).toBeNull();
  });

  it('wraps only newly arrived text, with its age as a negative delay', () => {
    let t = 0;
    const f = fadeNewText(() => t);
    f.apply(root('hello'));
    t = 100;
    const el = root('hello world');
    f.apply(el);
    const spans = el.querySelectorAll('.stream-in');
    expect(spans.length).toBe(1);
    expect(spans[0].textContent).toBe(' world');
    expect(spans[0].style.animationDelay).toBe('0ms');
    t = 200;
    const again = root('hello world!');
    f.apply(again);
    const texts = [...again.querySelectorAll('.stream-in')].map((s) => [s.textContent, s.style.animationDelay]);
    expect(texts).toEqual([[' world', '-100ms'], ['!', '0ms']]);
  });

  it('stops wrapping text older than the fade window', () => {
    let t = 0;
    const f = fadeNewText(() => t);
    f.apply(root('a'));
    t = 10;
    f.apply(root('ab'));
    t = 10 + FADE_MS + 5;
    const el = root('ab');
    f.apply(el);
    expect(el.querySelector('.stream-in')).toBeNull();
  });
});
