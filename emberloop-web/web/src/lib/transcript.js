// Pure transcript helpers: stick-to-bottom, scroll anchoring for
// load-earlier prepends, and deep-link lookup. Kept DOM-free for tests;
// Transcript.svelte applies them to the real scroll element.
export const STICK_THRESHOLD = 160;

// isNearBottom mirrors the chat follow behavior: within threshold px of
// the bottom counts as pinned, so streaming keeps following.
export function isNearBottom({ scrollHeight, scrollTop, clientHeight }, threshold = STICK_THRESHOLD) {
  return scrollHeight - scrollTop - clientHeight < threshold;
}

// anchorDelta is how much taller the scroll content grew while earlier
// turns prepended; the caller adds it to scrollTop so the viewport stays.
export function anchorDelta(before, after) {
  return after.scrollHeight - before.scrollHeight;
}

export function applyAnchor(el, before) {
  el.scrollTop = before.scrollTop + anchorDelta(before, { scrollHeight: el.scrollHeight });
}

// blockIndexForSeq finds the render block holding an event seq for ?at=
// deep links. Blocks carry their events; keys fall back to nearest-first.
export function blockIndexForSeq(blocks, seq) {
  if (!blocks?.length || seq == null) return -1;
  for (let i = 0; i < blocks.length; i++) {
    const evs = blocks[i]?.events ?? [];
    for (const e of evs) {
      if (e?.seq === seq) return i;
    }
  }
  let idx = -1;
  for (let i = 0; i < blocks.length; i++) {
    const key = blocks[i]?.key;
    if (typeof key === 'number' && key <= seq) idx = i;
    else if (typeof key === 'number') break;
  }
  return idx;
}
