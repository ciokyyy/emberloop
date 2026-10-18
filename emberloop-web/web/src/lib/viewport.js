// Visual viewport tracking for phones. The on-screen keyboard shrinks the
// visual viewport without resizing the layout viewport, so the shell reads
// --app-height and bottom sheets read --kb-inset from here instead of from
// dvh. Above the phone breakpoint nothing is set and the shell keeps 100dvh.

export const PHONE_QUERY = '(max-width: 767px)';

// One visualViewport reading: the visible height in px and the part of the
// layout viewport the keyboard covers (0 when it is closed).
export function viewportMetrics({ windowHeight = 0, height = 0, offsetTop = 0 } = {}) {
  const visible = Math.max(0, Math.round(height || windowHeight || 0));
  const inset = Math.max(0, Math.round(windowHeight - (height + offsetTop)));
  return { height: visible, inset, keyboard: inset > 0 };
}

// Whether the viewport is at the phone layout, where overlays are sheets and
// system Back closes them. Desktop dialogs keep Back for navigation.
export function isPhone(win = typeof window === 'undefined' ? null : window) {
  return Boolean(win?.matchMedia?.(PHONE_QUERY)?.matches);
}

export function trackViewport(win = typeof window === 'undefined' ? null : window) {
  const root = win?.document?.documentElement;
  if (!win || !root) return () => {};
  const view = win.visualViewport ?? null;
  const query = win.matchMedia?.(PHONE_QUERY) ?? null;

  const clear = () => {
    root.style.removeProperty('--app-height');
    root.style.removeProperty('--kb-inset');
    root.removeAttribute('data-keyboard');
  };
  const sync = () => {
    if (query && !query.matches) {
      clear();
      return;
    }
    const metrics = viewportMetrics({
      windowHeight: win.innerHeight,
      height: view?.height ?? win.innerHeight,
      offsetTop: view?.offsetTop ?? 0,
    });
    root.style.setProperty('--app-height', `${metrics.height}px`);
    root.style.setProperty('--kb-inset', `${metrics.inset}px`);
    if (metrics.keyboard) root.setAttribute('data-keyboard', 'open');
    else root.removeAttribute('data-keyboard');
  };

  sync();
  view?.addEventListener?.('resize', sync);
  view?.addEventListener?.('scroll', sync);
  win.addEventListener('resize', sync);
  query?.addEventListener?.('change', sync);
  return () => {
    view?.removeEventListener?.('resize', sync);
    view?.removeEventListener?.('scroll', sync);
    win.removeEventListener('resize', sync);
    query?.removeEventListener?.('change', sync);
    clear();
  };
}
