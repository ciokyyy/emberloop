// Reactive phone flag (viewport under 768 px, the Tailwind `md` breakpoint).
// Call once while a component initialises: it follows the media query until
// the component is destroyed. Where `matchMedia` is missing (SSR, jsdom) the
// flag stays false, so the desktop layout renders.
const QUERY = '(max-width: 767px)';

export function createPhone() {
  const state = $state({ value: typeof window !== 'undefined' && !!window.matchMedia?.(QUERY)?.matches });
  $effect(() => {
    const mq = typeof window !== 'undefined' ? window.matchMedia?.(QUERY) : null;
    if (!mq) return;
    const sync = () => (state.value = mq.matches);
    sync();
    mq.addEventListener?.('change', sync);
    return () => mq.removeEventListener?.('change', sync);
  });
  return state;
}
