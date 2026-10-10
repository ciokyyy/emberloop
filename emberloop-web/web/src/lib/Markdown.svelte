<!-- Agent output as sanitized Markdown. -->
<script>
  import { marked } from 'marked';
  import DOMPurify from 'dompurify';

  import { untrack } from 'svelte';
  import { fadeNewText } from './streamFade.js';
  import { nextShown, safeCut } from './pace.js';

  let { text = '', class: className = '', fade = false } = $props();
  let root = $state(null);
  let fader = null;

  marked.setOptions({ gfm: true, breaks: true });
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

  const html = $derived(
    DOMPurify.sanitize(marked.parse(visible, { async: false }))
      .replaceAll('<table', '<div class="md-table"><table')
      .replaceAll('</table>', '</table></div>'),
  );

  // Growing text fades in: only text that arrives after the first render is
  // wrapped, so history and settled messages never animate.
  $effect(() => {
    html;
    if (!fade || !root) return;
    fader ??= fadeNewText();
    fader.apply(root);
  });
</script>

<div bind:this={root} class="md {className}">{@html html}</div>
