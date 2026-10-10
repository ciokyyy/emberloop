<!-- Virtualized transcript: TanStack-virtual rows with dynamic measurement,
     a "Load earlier" sentinel that auto-loads on scroll up (IntersectionObserver,
     scroll-anchored so the viewport never jumps), stick-to-bottom while streaming
     unless the user scrolled up, and a jump-to-latest button. -->
<script>
  import { tick, untrack } from 'svelte';
  import { createVirtualizer } from '@tanstack/svelte-virtual';
  import Button from './ui/button.svelte';
  import Card from './Card.svelte';
  import { isNearBottom, applyAnchor, blockIndexForSeq } from './transcript.js';

  const ESTIMATE = 320;
  const OVERSCAN = 8;

  let {
    blocks = [],
    hasMore = false,
    loadingEarlier = false,
    onLoadEarlier = null,
    atSeq = 0,
    bottomPad = 192,
    emptyHint = '',
    sessionId = null,
    sessions = [],
    onOpenSession = null,
  } = $props();

  let scrollEl = $state(null);
  let sentinel = $state(null);
  let contentEl = $state(null);
  let stuck = $state(true);

  const virtualizer = createVirtualizer({
    count: blocks.length,
    getScrollElement: () => scrollEl,
    estimateSize: () => ESTIMATE,
    overscan: OVERSCAN,
    getItemKey: (i) => blocks[i]?.key ?? i,
  });

  // Rebind count, keys and scroll element after mount and on every list change.
  $effect(() => {
    const count = blocks.length;
    const el = scrollEl;
    untrack(() => {
      try {
        $virtualizer.setOptions({
          count,
          estimateSize: () => ESTIMATE,
          overscan: OVERSCAN,
          getScrollElement: () => el,
          getItemKey: (i) => blocks[i]?.key ?? i,
        });
      } catch {
        // Unmounted scroll element (or jsdom): the next run rebinds.
      }
    });
  });

  const virtualRows = $derived.by(() => {
    try {
      const rows = $virtualizer.getVirtualItems();
      if (rows.length > 0) return { virtual: true, rows: rows.map((r) => ({ index: r.index, start: r.start, key: r.key })) };
    } catch {
      // Fall through to the plain list below.
    }
    return { virtual: false, rows: [] };
  });

  const totalSize = $derived.by(() => {
    try {
      const total = $virtualizer.getTotalSize();
      if (total > 0) return total;
    } catch {
      // Fall through to zero below.
    }
    return 0;
  });

  const useVirtual = $derived(virtualRows.virtual && totalSize > 0);

  function measure(node) {
    try {
      $virtualizer.measureElement(node);
    } catch {
      // Zero-size element (jsdom): the plain list covers it.
    }
    return { destroy() {} };
  }

  // Glide to the bottom instead of snapping. Time-based exponential easing,
  // not a per-frame fraction: the speed is the same on 60, 120 and 144 Hz
  // displays. The position is kept as a float, because scrollTop rounds to
  // whole pixels and small per-frame steps would stall, then jump. One loop
  // runs at a time and re-reads the target each frame, so content that keeps
  // growing is followed to its end.
  const GLIDE_TAU_MS = 110;
  const PIN_PX = 64;
  let glideFrame = 0;
  function glideToBottom() {
    const el = scrollEl;
    if (!el) return;
    const reduced = typeof matchMedia !== 'undefined' && matchMedia('(prefers-reduced-motion: reduce)').matches;
    if (reduced || typeof requestAnimationFrame === 'undefined') {
      el.scrollTop = el.scrollHeight;
      return;
    }
    if (glideFrame) return;
    let pos = el.scrollTop;
    let last = 0;
    const step = (now) => {
      glideFrame = 0;
      if (!stuck || !scrollEl) return;
      const dt = last ? Math.min(now - last, 64) : 16;
      last = now;
      // Someone else moved the view (user, anchor): follow, do not fight.
      if (Math.abs(scrollEl.scrollTop - pos) > 1.5) pos = scrollEl.scrollTop;
      const max = scrollEl.scrollHeight - scrollEl.clientHeight;
      const gap = max - pos;
      if (gap <= 0.5) {
        scrollEl.scrollTop = max;
        return;
      }
      // Near the bottom (streaming text growing a line at a time) easing only
      // adds lag and uneven speed: pin exactly and keep following. Far away
      // (a new block, a tall card) ease in.
      if (gap < PIN_PX) pos = max;
      else pos += gap * (1 - Math.exp(-dt / GLIDE_TAU_MS));
      scrollEl.scrollTop = pos;
      glideFrame = requestAnimationFrame(step);
    };
    glideFrame = requestAnimationFrame(step);
  }

  // Glide only ever moves down, so any upward scroll is the user: unpin at
  // once. A downward scroll never unpins (mid-glide the gap is large); it
  // re-pins once near the bottom. No flag to leak when a step fires no event.
  let lastTop = 0;
  let lastHeight = 0;
  let jumpUntil = 0;
  // Whether the view sits at the end, whatever the pin state: drives the
  // jump button so it never shows while the user is already down.
  let nearBottom = $state(true);
  function onScroll() {
    if (!scrollEl) return;
    const top = scrollEl.scrollTop;
    const height = scrollEl.scrollHeight;
    // Content that shrinks (a tool stage swapping to a shorter output)
    // clamps scrollTop down by itself: that is not the user either.
    const shrunk = height < lastHeight - 1;
    const movedUp = top < lastTop - 1;
    lastTop = top;
    lastHeight = height;
    nearBottom = isNearBottom(scrollEl);
    // While a jump to latest runs, rows above the view re-measure and the
    // virtualizer shifts scrollTop up on its own: that is not the user.
    // Wheel, touch and key input still unpin.
    if (movedUp && !shrunk && !nearBottom && performance.now() > jumpUntil) {
      stuck = false;
      return;
    }
    if (!stuck && nearBottom && !movedUp) stuck = true;
  }

  // Wheel, touch and key input away from the bottom unpin before the next
  // glide frame can pull the view back.
  function onUserUp() {
    jumpUntil = 0;
    stuck = false;
  }
  function onWheel(e) {
    if (e.deltaY < 0) onUserUp();
  }
  let touchY = 0;
  function onTouchStart(e) {
    touchY = e.touches?.[0]?.clientY ?? 0;
  }
  function onTouchMove(e) {
    const y = e.touches?.[0]?.clientY ?? 0;
    if (y > touchY + 2) onUserUp();
    touchY = y;
  }
  function onKey(e) {
    if (['ArrowUp', 'PageUp', 'Home'].includes(e.key)) onUserUp();
  }

  // Follow the transcript while pinned, including while the last card grows
  // (streaming text adds no new block).
  $effect(() => {
    blocks.length;
    bottomPad;
    const last = blocks[blocks.length - 1];
    last?.body;
    const el = scrollEl;
    const pinned = stuck;
    if (!el || !pinned) return;
    untrack(() => glideToBottom());
  });

  // A card that animates its height (the tool strip) or a composer that
  // resizes grows the content with no block change. Follow every frame of it
  // while pinned, so the view glides down instead of lagging behind.
  $effect(() => {
    const node = contentEl;
    const el = scrollEl;
    if (!node || !el || typeof ResizeObserver === 'undefined') return;
    const ro = new ResizeObserver(() => {
      lastHeight = el.scrollHeight;
      nearBottom = isNearBottom(el);
      if (stuck) glideToBottom();
    });
    ro.observe(node);
    return () => ro.disconnect();
  });

  async function loadEarlier() {
    if (!hasMore || loadingEarlier || !onLoadEarlier) return;
    const el = scrollEl;
    const before = el ? { scrollHeight: el.scrollHeight, scrollTop: el.scrollTop } : null;
    await onLoadEarlier();
    await tick();
    await new Promise((r) => requestAnimationFrame(r));
    if (el && before) applyAnchor(el, before);
  }

  // Auto-load on scroll up; the button covers observers-less browsers and tests.
  $effect(() => {
    const node = sentinel;
    const root = scrollEl;
    if (!node || typeof IntersectionObserver === 'undefined') return;
    const ob = new IntersectionObserver(
      (entries) => {
        if (entries.some((e) => e.isIntersecting)) void loadEarlier();
      },
      { root },
    );
    ob.observe(node);
    return () => ob.disconnect();
  });

  // ?at= deep link: bring the target block into view once it renders.
  let atDone = 0;
  $effect(() => {
    const target = atSeq;
    const count = blocks.length;
    const el = scrollEl;
    if (!target || target === atDone || !count || !el) return;
    const idx = blockIndexForSeq(blocks, target);
    if (idx < 0) return;
    untrack(() => {
      let scrolled = false;
      try {
        $virtualizer.scrollToIndex(idx, { align: 'center' });
        scrolled = true;
      } catch {
        // Fall through to the DOM lookup below.
      }
      if (!scrolled) {
        try {
          const key = blocks[idx]?.key;
          el.querySelector(`[data-block-key="${CSS.escape(String(key))}"]`)?.scrollIntoView({ block: 'center' });
        } catch {
          // jsdom: nothing to scroll.
        }
      }
    });
    atDone = target;
  });

  function jumpLatest() {
    const el = scrollEl;
    if (!el) return;
    stuck = true;
    jumpUntil = performance.now() + 1500;
    // Render and measure the last row first, so the end is real, not estimated.
    try {
      if (blocks.length) $virtualizer.scrollToIndex(blocks.length - 1, { align: 'end' });
    } catch {
      // jsdom: the glide below covers it.
    }
    glideToBottom();
  }
</script>

<div bind:this={scrollEl} onscroll={onScroll} onwheel={onWheel} ontouchstart={onTouchStart} ontouchmove={onTouchMove} onkeydown={onKey} class="flex flex-1 flex-col overflow-auto px-3 pt-4 sm:px-4" style="padding-bottom: {bottomPad}px">
  <div bind:this={contentEl} class="mx-auto flex w-full max-w-3xl flex-col gap-4">
    {#if hasMore}
      <div bind:this={sentinel} class="flex justify-center py-1" data-testid="load-earlier">
        <Button variant="ghost" size="sm" disabled={loadingEarlier} onclick={loadEarlier}>
          {#if loadingEarlier}
            <span class="size-3.5 animate-spin rounded-full border-2 border-muted-foreground/40 border-t-foreground" aria-hidden="true"></span>
            Loading earlier…
          {:else}
            Load earlier
          {/if}
        </Button>
      </div>
    {/if}
    {#if useVirtual}
      <div style="position: relative; height: {totalSize}px; width: 100%;">
        {#each virtualRows.rows as row (row.key)}
          {@const b = blocks[row.index]}
          {#if b}
            <div data-index={row.index} data-block-key={b.key} use:measure class="absolute top-0 left-0 w-full" style="transform: translateY({row.start}px);">
              <div class="pb-4">
                <Card block={b} sessionId={sessionId} {sessions} {onOpenSession} />
              </div>
            </div>
          {/if}
        {/each}
      </div>
    {:else}
      {#each blocks as b (b.key)}
        <div data-block-key={b.key}>
          <Card block={b} sessionId={sessionId} {sessions} {onOpenSession} />
        </div>
      {:else}
        <p class="py-24 text-center text-2xl font-semibold tracking-tight text-muted-foreground">{emptyHint}</p>
      {/each}
    {/if}
  </div>
  {#if !stuck && !nearBottom && blocks.length}
    <div class="sticky bottom-2 flex justify-center">
      <Button variant="secondary" size="sm" class="shadow-lg" onclick={jumpLatest}>↓ Latest</Button>
    </div>
  {/if}
</div>
