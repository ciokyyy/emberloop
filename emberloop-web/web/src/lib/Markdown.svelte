<!-- Agent output as sanitized Markdown. -->
<script>
  import { untrack } from 'svelte';
  import { fadeNewText } from './streamFade.js';
  import { nextShown, safeCut } from './pace.js';
  import { renderMarkdown, createIncrementalMarkdown } from './markdown.js';

  let { text = '', class: className = '', fade = false } = $props();
  let root = $state(null);
  let fader = null;
  const incr = createIncrementalMarkdown();

  // Tables sit in a clipped, rounded wrapper: a bare table cannot round its
  // corners with collapsed borders, and the wrapper scrolls wide ones.
  // Paced reveal: a mount shows the whole text (history never animates); a
  // later growth reveals toward the new text frame by frame. Any change that
  // is not growth of what is already shown applies at once.
  const full = $derived(String(text ?? ''));
  let shown = $state(untrack(() => String(text ?? '').length));
  let frame = 0;
  $effect(() => {
    const target = full;
    const reduced = typeof matchMedia !== 'undefined' && matchMedia('(prefers-reduced-motion: reduce)').matches;
    untrack(() => {
      if (!fade || reduced || typeof requestAnimationFrame === 'undefined') {
        shown = target.length;
        return;
      }
      if (shown > target.length) shown = target.length;
      if (frame) return;
      const step = () => {
        frame = 0;
        const t = full;
        shown = nextShown(shown, t.length);
        if (shown < t.length) frame = requestAnimationFrame(step);
      };
      if (shown < target.length) frame = requestAnimationFrame(step);
    });
  });
  $effect(() => () => {
    if (frame) cancelAnimationFrame(frame);
  });
  const visible = $derived(safeCut(full, shown));

  // Renders throttle to ~5Hz while streaming: a fast model pushes dozens of
  // deltas per window, and every render replaces the whole DOM. The settled
  // head parses once (see markdown.js); only the tail re-parses per tick.
  // The settled message always gets one exact full render. Mount renders the
  // mount text at once, so history and tests never see a blank frame.
  const RENDER_MS = 200;
  let html = $state(untrack(() => renderMarkdown(String(text ?? ''))));
  let clock = $state(0);
  let lastRender = 0;
  $effect(() => {
    const v = visible;
    const f = full;
    clock;
    const now = typeof performance !== 'undefined' ? performance.now() : 0;
    untrack(() => {
      if (v.length >= f.length) {
        const next = incr.renderFull(f);
        if (next !== html) html = next;
        lastRender = now;
        return;
      }
      if (now - lastRender >= RENDER_MS) {
        const next = incr.render(v);
        if (next !== html) html = next;
        lastRender = now;
        return;
      }
      const t = setTimeout(() => {
        clock += 1;
      }, RENDER_MS - (now - lastRender));
      return () => clearTimeout(t);
    });
  });

  // Growing text fades in: only text that arrives after the first render is
  // wrapped, so history and settled messages never animate. Past the skip
  // backlog the text appears at once instead of churning splitText spans.
  $effect(() => {
    html;
    const f = full;
    const v = visible;
    if (!fade || !root) return;
    fader ??= fadeNewText();
    fader.apply(root, { backlog: f.length - v.length });
  });
</script>

<div bind:this={root} class="md {className}">{@html html}</div>
