import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';

const css = readFileSync(join(process.cwd(), 'src/app.css'), 'utf8');
const design = readFileSync(join(process.cwd(), 'DESIGN.md'), 'utf8');

const tokensOf = (re) => [...css.matchAll(re)].map((m) => m[1]);

describe('design tokens in app.css', () => {
  it('defines surface levels 0 to 3, text levels 1 to 4 and the five status colours', () => {
    for (const name of ['surface-0', 'surface-1', 'surface-2', 'surface-3', 'fg-1', 'fg-2', 'fg-3', 'fg-4', 'status-working', 'status-needs-you', 'status-done', 'status-failed', 'status-paused']) {
      expect(css, name).toMatch(new RegExp(`--${name}:\\s*#`));
      expect(css, `${name} utility`).toContain(`--color-${name}: var(--${name})`);
    }
    expect(tokensOf(/(--owner-\d):/g)).toHaveLength(6);
  });
  it('defines the type scale, the 4 px spacing base and three elevations', () => {
    for (const name of ['headline', 'title', 'body', 'caption', 'numeric']) expect(css, name).toContain(`--text-${name}:`);
    expect(css).toContain('--spacing: 0.25rem');
    for (const n of [1, 2, 3]) expect(css).toContain(`--shadow-elev-${n}:`);
  });
  it('has one easing and a 100/180/320 ms motion scale', () => {
    expect(css).toMatch(/--dur-instant:\s*100ms/);
    expect(css).toMatch(/--dur-standard:\s*180ms/);
    expect(css).toMatch(/--dur-emphasis:\s*320ms/);
    expect(tokensOf(/(--ease-[a-z]+):/g)).toEqual(['--ease-smooth']);
  });
  it('turns non-essential motion off under reduced motion and keeps state feedback', () => {
    const block = css.slice(css.indexOf('@media (prefers-reduced-motion: reduce) {\n  *,'));
    expect(block).toContain('animation-duration: 0.01ms !important');
    expect(block).toContain('transition-duration: 0.01ms !important');
    expect(block).toContain('scroll-behavior: auto !important');
    expect(block).toContain('[data-motion="essential"]');
  });
});

describe('DESIGN.md', () => {
  it('documents every token family, primitive and rule', () => {
    for (const word of ['surface-0', 'fg-1', 'status-needs-you', '--owner-1', 'text-headline', 'text-numeric', 'shadow-elev-1', '--dur-instant', '--ease-smooth', 'icons.js']) {
      expect(design, word).toContain(word);
    }
    for (const primitive of ['Section', 'Row', 'StatCard', 'Chip', 'IconButton', 'EmptyState', 'Skeleton', 'ErrorState', 'InfoHint']) {
      expect(design, primitive).toContain(primitive);
    }
    for (const rule of ['Icon rule', 'Disclosure rule', 'Responsive rule']) expect(design, rule).toContain(rule);
  });
});
