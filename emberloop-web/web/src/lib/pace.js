// Reveal pacing for streamed text. The claude CLI often flushes a whole
// reply at once, so the UI paces growth itself: each frame reveals a share
// of the backlog (at least MIN_STEP chars), which finishes a big burst in
// about a second and passes small live chunks through almost untouched.
// A fast model can outrun a proportional reveal forever (the backlog's
// equilibrium sits at arrival-per-frame divided by CATCHUP), so a backlog
// past MAX_BACKLOG jumps instead: the view stays at most ~MAX_BACKLOG chars
// behind, the fade span budget stays bounded, and animations can complete.
export const MIN_STEP = 3;
export const CATCHUP = 0.06;
export const MAX_BACKLOG = 2000;

// nextShown returns how many chars to show after one frame.
export function nextShown(shown, target) {
  const backlog = target - shown;
  if (backlog <= 0) return target;
  if (backlog > MAX_BACKLOG) return target - MAX_BACKLOG;
  return Math.min(target, shown + Math.max(MIN_STEP, Math.ceil(backlog * CATCHUP)));
}

// safeCut avoids ending the revealed text inside a surrogate pair.
export function safeCut(text, n) {
  if (n >= text.length) return text;
  const c = text.charCodeAt(n - 1);
  return text.slice(0, c >= 0xd800 && c <= 0xdbff ? n - 1 : n);
}
