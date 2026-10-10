// Reveal pacing for streamed text. The claude CLI often flushes a whole
// reply at once, so the UI paces growth itself: each frame reveals a share
// of the backlog (at least MIN_STEP chars), which finishes a big burst in
// about a second and passes small live chunks through almost untouched.
export const MIN_STEP = 3;
export const CATCHUP = 0.06;

// nextShown returns how many chars to show after one frame.
export function nextShown(shown, target) {
  const backlog = target - shown;
  if (backlog <= 0) return target;
  return Math.min(target, shown + Math.max(MIN_STEP, Math.ceil(backlog * CATCHUP)));
}

// safeCut avoids ending the revealed text inside a surrogate pair.
export function safeCut(text, n) {
  if (n >= text.length) return text;
  const c = text.charCodeAt(n - 1);
  return text.slice(0, c >= 0xd800 && c <= 0xdbff ? n - 1 : n);
}
