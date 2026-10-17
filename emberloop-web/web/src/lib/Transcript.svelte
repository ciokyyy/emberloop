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
  import { getQueue } from './api.js';

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
  let queueItems = $state([]);
  let queueAssignments = $state({});
  let canceledBlockKeys = $state([]);
  let queueRequest = 0;
  let queueSession = '';
  const queueLabel = (mode) => mode === 'steer' ? 'Steer' : mode === 'followup' ? 'Follow-up' : 'Prompt';

  function matchesQueueBlock(block, item) {
    if (block.title !== queueLabel(item.mode) || String(block.body ?? '') !== String(item.text ?? '')) return false;
    const blockImages = block.imageIds ?? [];
    const queueImages = item.imageIds ?? [];
    return blockImages.length === queueImages.length && blockImages.every((id, index) => id === queueImages[index]);
  }

  async function refreshQueue(id = sessionId) {
    if (!id) {
      queueItems = [];
      queueAssignments = {};
      return;
    }
    const request = ++queueRequest;
    try {
      const body = await getQueue(id);
      if (request !== queueRequest || id !== sessionId) return;
      queueItems = body.queue ?? [];
      const ids = new Set(queueItems.map((item) => item.id));
      queueAssignments = Object.fromEntries(Object.entries(queueAssignments).filter(([itemId]) => ids.has(itemId)));
    } catch {
      // Queue controls are best-effort; a failed read keeps the last good rows.
    }
  }

  function queueChanged(change) {
    if (change.type === 'edit') {
      const block = queueView.byId.get(change.id);
      if (block) queueAssignments = { ...queueAssignments, [change.id]: block.key };
      queueItems = queueItems.map((item) => item.id === change.id ? { ...item, text: change.text } : item);
    } else if (change.type === 'remove') {
      const block = queueView.byId.get(change.id);
      if (block) canceledBlockKeys = [...new Set([...canceledBlockKeys, block.key])];
      queueItems = queueItems.filter((item) => item.id !== change.id);
      const next = { ...queueAssignments };
      delete next[change.id];
      queueAssignments = next;
    }
    void refreshQueue(sessionId);
  }

  const queueView = $derived.by(() => {
    const available = queueItems.filter((item) => item.mode === 'steer' || item.mode === 'followup');
    const remaining = available.slice();
    const byKey = new Map();
    const byId = new Map();
    for (const item of available) {
      const assignedKey = queueAssignments[item.id];
      if (assignedKey == null) continue;
      const block = blocks.find((candidate) => String(candidate.key) === String(assignedKey));
      if (!block || canceledBlockKeys.includes(block.key) || block.card !== 'user' || block.sender !== 'user') continue;
      byKey.set(block.key, item);
      byId.set(item.id, block);
      const index = remaining.findIndex((candidate) => candidate.id === item.id);
      if (index !== -1) remaining.splice(index, 1);
    }
    for (let i = blocks.length - 1; i >= 0; i--) {
      const block = blocks[i];
      if (block.card !== 'user' || block.sender !== 'user' || byKey.has(block.key) || canceledBlockKeys.includes(block.key)) continue;
      let index = -1;
      for (let j = remaining.length - 1; j >= 0; j--) {
        if (matchesQueueBlock(block, remaining[j])) { index = j; break; }
      }
      if (index === -1) continue;
      const [item] = remaining.splice(index, 1);
      byKey.set(block.key, item);
      byId.set(item.id, block);
    }
    return { byKey, byId, extra: remaining };
  });
  $effect(() => {
    const assignments = {};
    for (const [id, block] of queueView.byId) {
      if (queueAssignments[id] == null) assignments[id] = block.key;
    }
    if (Object.keys(assignments).length) queueAssignments = { ...queueAssignments, ...assignments };
  });

  $effect(() => {
    const id = sessionId;
    blocks.length;
    blocks[blocks.length - 1]?.key;
    if (id !== queueSession) {
      queueSession = id;
      queueItems = [];
      canceledBlockKeys = [];
      queueAssignments = {};
    }
    if (!id) return;
    void refreshQueue(id);
  });
  $effect(() => {
    const id = sessionId;
    if (!id) return;
    const timer = setInterval(() => void refreshQueue(id), 3000);
    return () => clearInterval(timer);
  });

  let scrollEl = $state(null);
  let sentinel = $state(null);
  let contentEl = $state(null);
  let stuck = $state(true);

  // Measuring every streaming delta re-layouts the whole list per flush: a
  // recent height is reused unless it moved enough or enough time passed.
  // Zero-size (tests) always measures, exactly like the default.
  const MEASURE_MS = 200;
  const MEASURE_PX = 32;
  const measuredAt = new Map();
  function throttledMeasure(el, entry, inst) {
    let size = 0;
    const box = entry?.borderBoxSize?.[0];
    if (box) size = Math.round(box.blockSize);
    else {
      try {
        size = Math.round(el.getBoundingClientRect()?.height ?? 0);
      } catch {
        size = 0;
      }
      if (!size) size = el.offsetHeight || 0;
    }
    if (!size) return size;
    const index = inst.indexFromElement(el);
    const key = inst.options.getItemKey(index);
    const prev = measuredAt.get(key);
    const now = typeof performance !== 'undefined' ? performance.now() : 0;
    if (prev && now - prev.t < MEASURE_MS && Math.abs(size - prev.size) < MEASURE_PX) return prev.size;
    measuredAt.set(key, { size, t: now });
    if (measuredAt.size > 500) measuredAt.delete(measuredAt.keys().next().value);
    return size;
  }

  const noScrollAdjustment = () => false;
  const virtualizer = createVirtualizer({
    count: blocks.length,
    getScrollElement: () => scrollEl,
    estimateSize: () => ESTIMATE,
    overscan: OVERSCAN,
    getItemKey: (i) => blocks[i]?.key ?? i,
    measureElement: throttledMeasure,
    // Height changes are anchored by the browser; only history prepends use
    // the explicit correction in loadEarlier().
    shouldAdjustScrollPositionOnItemSizeChange: noScrollAdjustment,
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
          measureElement: throttledMeasure,
          shouldAdjustScrollPositionOnItemSizeChange: noScrollAdjustment,
        });
      } catch {
        // Unmounted scroll element (or jsdom): the next run rebinds.
      }
    });
  });

  const virtualRows = $derived.by(() => {
    try {
      const rows = $virtualizer.getVirtualItems();
      if (rows.length > 0) return { virtual: true, rows: rows.map((r) => ({ index: r.index, start: r.start, end: r.end, key: r.key })) };
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

  // Input releases the pin synchronously, before a queued follow frame can
  // write a new scroll position. Browser anchoring remains responsible for
  // height changes while the reader is away from the bottom.
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
      cancelGlide();
      return;
    }
    if (!stuck && nearBottom && !movedUp) stuck = true;
  }

  function cancelGlide() {
    if (!glideFrame) return;
    if (typeof cancelAnimationFrame !== 'undefined') cancelAnimationFrame(glideFrame);
    glideFrame = 0;
  }

  function onUserUp() {
    jumpUntil = 0;
    stuck = false;
    cancelGlide();
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
    const spaceUp = e.shiftKey && (e.key === ' ' || e.code === 'Space' || e.key === 'Spacebar');
    if (['ArrowUp', 'PageUp', 'Home'].includes(e.key) || spaceUp) onUserUp();
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

  // Observe the transcript's full content for layout changes while pinned:
  // tool output, card disclosures and the bottom-pad resize all keep the last
  // row in view. Once unpinned, native scroll anchoring owns height changes.
  $effect(() => {
    const content = contentEl;
    const el = scrollEl;
    if (!content || !el || typeof ResizeObserver === 'undefined') return;
    const ro = new ResizeObserver(() => {
      lastHeight = el.scrollHeight;
      nearBottom = isNearBottom(el);
      if (stuck) glideToBottom();
    });
    ro.observe(content);
    return () => ro.disconnect();
  });

  async function loadEarlier() {
    if (!hasMore || loadingEarlier || !onLoadEarlier) return;
    const el = scrollEl;
    const before = el ? { scrollHeight: el.scrollHeight, scrollTop: el.scrollTop } : null;
    const previousOverflowAnchor = el?.style.overflowAnchor ?? '';
    if (el) el.style.overflowAnchor = 'none';
    try {
      await onLoadEarlier();
      await tick();
      await new Promise((resolve) => {
        if (typeof requestAnimationFrame === 'undefined') resolve();
        else requestAnimationFrame(resolve);
      });
      if (el && before) applyAnchor(el, before);
    } finally {
      if (el) el.style.overflowAnchor = previousOverflowAnchor;
    }
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
    cancelGlide();
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

<!-- Keyboard-focusable scroll region; arrow keys release stick-to-bottom. -->
<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<div bind:this={scrollEl} data-testid="transcript-scroll" role="region" aria-label="Transcript" tabindex="0" onscroll={onScroll} onwheel={onWheel} ontouchstart={onTouchStart} ontouchmove={onTouchMove} onkeydown={onKey} class="transcript-scroll flex flex-1 scroll-auto flex-col overflow-auto px-3 pt-4 sm:px-4" style="padding-bottom: {bottomPad}px">
  <div bind:this={contentEl} data-testid="transcript-content" class="mx-auto flex w-full max-w-3xl flex-col gap-4">
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
      <div data-testid="transcript-virtual-list" class="flex w-full flex-col">
        <div aria-hidden="true" class="shrink-0" style="height: {virtualRows.rows[0]?.start ?? 0}px;"></div>
        {#each virtualRows.rows as row, rowIndex (row.key)}
          {@const previous = virtualRows.rows[rowIndex - 1]}
          {@const b = blocks[row.index]}
          {#if previous}
            <div aria-hidden="true" class="shrink-0" style="height: {Math.max(0, row.start - previous.end)}px;"></div>
          {/if}
          {#if b}
            <div data-index={row.index} data-block-key={b.key} use:measure class="w-full shrink-0">
              <div class="pb-4">
                <Card block={b} sessionId={sessionId} {sessions} {onOpenSession} queuedItem={queueView.byKey.get(b.key) ?? null} cancelled={canceledBlockKeys.includes(b.key)} onQueueChange={queueChanged} />
              </div>
            </div>
          {/if}
        {/each}
        {#if virtualRows.rows.length > 0}
          <div aria-hidden="true" class="shrink-0" style="height: {Math.max(0, totalSize - virtualRows.rows[virtualRows.rows.length - 1].end)}px;"></div>
        {/if}
      </div>
    {:else}
      {#each blocks as b (b.key)}
        <div data-block-key={b.key} style="content-visibility: auto; contain-intrinsic-size: auto 320px;">
          <Card block={b} sessionId={sessionId} {sessions} {onOpenSession} queuedItem={queueView.byKey.get(b.key) ?? null} cancelled={canceledBlockKeys.includes(b.key)} onQueueChange={queueChanged} />
        </div>
      {:else}
        <p class="py-24 text-center text-2xl font-semibold tracking-tight text-muted-foreground">{emptyHint}</p>
      {/each}
    {/if}
    {#each queueView.extra as item (item.id)}
      {@const queuedBlock = { key: `queue-${item.id}`, card: 'user', title: queueLabel(item.mode), body: item.text, imageIds: item.imageIds ?? [], sender: 'user', senderId: '' }}
      <Card block={queuedBlock} {sessionId} queuedItem={item} onQueueChange={queueChanged} />
    {/each}
  </div>
  {#if !stuck && blocks.length}
    <div class="sticky bottom-2 flex justify-center">
      <Button variant="secondary" size="sm" class="shadow-lg" onclick={jumpLatest}>↓ Latest</Button>
    </div>
  {/if}
</div>
