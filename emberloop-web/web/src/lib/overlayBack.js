// Overlay back stack: the open sheets share one transient history entry, so
// system Back (or the swipe-back gesture) closes the top sheet instead of
// leaving the page. The last sheet closed from its own UI drops that entry.
//
// The router (App) asks `selfPopPending()` before it reads a popstate and
// `backOpen()` before it routes; this module closes the top sheet in its own
// popstate listener. Without a router (a sheet rendered alone) the same
// listener still closes it. A navigation that closes a sheet goes through
// `closeBacks()` so its entry becomes the destination instead of racing a Back
// traversal with a push.

const stack = []; // { close, popped }
let entry = false; // one history entry of ours is current
let carry = false; // the entry passes to the sheet opening next
let selfPops = 0;
let listening = false;

function historyRef() {
  try {
    return typeof window !== 'undefined' ? window.history : null;
  } catch {
    return null;
  }
}

function pushEntry() {
  const history = historyRef();
  try {
    history?.pushState({ ...(history.state ?? {}), elSheet: stack.length }, '', window.location.href);
    entry = true;
  } catch {
    // Sandboxed or file:// history: Back still closes through the listener.
  }
}

function onPop() {
  if (takeSelfPop()) return;
  popBack();
}

function listen() {
  if (listening || typeof window === 'undefined') return;
  listening = true;
  window.addEventListener('popstate', onPop);
}

// Hold the shared back entry for an open overlay. `close` runs on system Back.
// The returned release must run when the overlay closes: it drops the entry
// unless Back already popped it.
export function holdBack(close) {
  const token = { close, popped: false };
  listen();
  stack.push(token);
  if (!entry) pushEntry();
  return () => releaseBack(token);
}

// The sheet about to open replaces the one closing now: keep the entry instead
// of popping it and pushing another one.
export function carryBack() {
  carry = entry;
}

function releaseBack(token) {
  const index = stack.indexOf(token);
  if (index === -1) return;
  stack.splice(index, 1);
  if (token.popped) return;
  // A sheet underneath still owns the entry.
  if (stack.length) return;
  if (carry) {
    carry = false;
    return;
  }
  // Closed from its own UI: drop the entry it held.
  entry = false;
  selfPops += 1;
  try {
    historyRef()?.back();
  } catch {
    selfPops = Math.max(0, selfPops - 1);
  }
}

// Close the top overlay from a Back event. Returns false when nothing is open.
export function popBack() {
  const token = stack.pop();
  if (!token) return false;
  token.popped = true;
  token.close();
  entry = false;
  // The browser consumed the entry; a sheet underneath needs one again.
  if (stack.length) pushEntry();
  return true;
}

// Close every overlay without touching history, for a navigation that takes
// the entry over. Returns how many overlays were open.
export function closeBacks() {
  const tokens = stack.splice(0, stack.length);
  for (const token of tokens) {
    token.popped = true;
    token.close();
  }
  entry = false;
  carry = false;
  return tokens.length;
}

// A sheet is open, so the router leaves the popstate to this module.
export function backOpen() {
  return stack.length > 0;
}

// Peek at a pop this module caused itself, so the router ignores it.
export function selfPopPending() {
  return selfPops > 0;
}

// Consume one self-caused pop (this module's listener does it).
export function takeSelfPop() {
  if (!selfPops) return false;
  selfPops -= 1;
  return true;
}

// Test seam: forget every entry without touching history.
export function resetBackStack() {
  stack.length = 0;
  entry = false;
  carry = false;
  selfPops = 0;
  if (listening && typeof window !== 'undefined') {
    window.removeEventListener('popstate', onPop);
    listening = false;
  }
}
