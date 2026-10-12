<!-- Agent output as sanitized Markdown. Fenced code blocks get a small copy
  button (event delegation on the root, so streaming re-renders keep working;
  buttons are re-attached after each render). -->
<script>
  import { marked } from 'marked';
  import DOMPurify from 'dompurify';
  import { buttonVariants } from './ui/button.svelte';
  import { cn } from './ui/cn.js';

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
      .replaceAll('</table>', '</table></div>')
      // Anchor for the copy button: a relative box for its absolute corner.
      .replaceAll('<pre>', '<pre class="relative md-pre">'),
  );

  // Copy-button icons, same shapes as the lucide Copy and Check components
  // (inlined so they can live inside rendered HTML without a mount).
  const svgOpen = '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">';
  const COPY_SVG = `${svgOpen}<rect width="14" height="14" x="8" y="8" rx="2" ry="2"/><path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2"/></svg>`;
  const CHECK_SVG = `${svgOpen}<path d="M20 6 9 17l-5-5"/></svg>`;

  function makeCopyButton() {
    const btn = document.createElement('button');
    btn.type = 'button';
    // shadcn ghost icon button; 44px hit area on touch, compact on desktop.
    btn.className = cn(buttonVariants({ variant: 'ghost', size: 'icon-sm' }), 'md-copy absolute top-1.5 right-1.5 h-11 w-11 bg-background/70 opacity-70 backdrop-blur hover:opacity-100 sm:h-8 sm:w-8');
    btn.setAttribute('aria-label', 'Copy code');
    btn.innerHTML = COPY_SVG;
    return btn;
  }

  // Plain-http fallback (the app is often served over http on tailnet, where
  // the async clipboard is unavailable): hidden textarea + execCommand.
  function fallbackCopy(text) {
    const ta = document.createElement('textarea');
    ta.value = text;
    ta.style.position = 'fixed';
    ta.style.opacity = '0';
    document.body.appendChild(ta);
    ta.select();
    document.execCommand('copy');
    ta.remove();
  }

  let copyTimer = 0;
  async function copyCode(text, btn) {
    try {
      if (navigator.clipboard?.writeText) await navigator.clipboard.writeText(text);
      else fallbackCopy(text);
    } catch {
      try {
        fallbackCopy(text);
      } catch {
        return;
      }
    }
    btn.innerHTML = `${CHECK_SVG}<span class="text-xs">Copied</span>`;
    btn.style.width = 'auto';
    btn.setAttribute('aria-label', 'Copied');
    clearTimeout(copyTimer);
    copyTimer = setTimeout(() => {
      if (!btn.isConnected) return;
      btn.innerHTML = COPY_SVG;
      btn.style.width = '';
      btn.setAttribute('aria-label', 'Copy code');
    }, 1600);
  }

  // One delegated listener on the root: innerHTML swaps on every stream frame
  // never disturb it.
  function onRootClick(e) {
    const btn = e.target?.closest?.('.md-copy');
    if (!btn || !root?.contains(btn)) return;
    const pre = btn.closest('pre');
    const code = pre?.querySelector('code')?.textContent ?? pre?.textContent ?? '';
    void copyCode(code, btn);
  }

  // Growing text fades in: only text that arrives after the first render is
  // wrapped, so history and settled messages never animate.
  $effect(() => {
    html;
    if (!fade || !root) return;
    fader ??= fadeNewText();
    fader.apply(root);
  });

  // Attach one copy button per fenced block after each render (streaming
  // replaces innerHTML, so re-attach; existing buttons are left alone).
  $effect(() => {
    html;
    if (!root) return;
    for (const pre of root.querySelectorAll('pre')) {
      if (pre.querySelector(':scope > .md-copy')) continue;
      pre.appendChild(makeCopyButton());
    }
  });
</script>

<div bind:this={root} class="md {className}" onclick={onRootClick}>{@html html}</div>
