import { afterEach, describe, expect, it, vi } from 'vitest';
import { PHONE_QUERY, isPhone, trackViewport, viewportMetrics } from './viewport.js';

afterEach(() => {
  vi.restoreAllMocks();
});

// A window shape with a controllable visual viewport and one phone media query.
function fakeWindow({ matches = true, height = 844, offsetTop = 0, innerHeight = 844 } = {}) {
  const listeners = new Map();
  const view = {
    height,
    offsetTop,
    addEventListener: (type, fn) => listeners.set(`view:${type}`, fn),
    removeEventListener: (type) => listeners.delete(`view:${type}`),
    resize(next) {
      this.height = next.height ?? this.height;
      this.offsetTop = next.offsetTop ?? this.offsetTop;
      listeners.get('view:resize')?.();
    },
  };
  const query = {
    matches,
    addEventListener: (type, fn) => listeners.set(`query:${type}`, fn),
    removeEventListener: (type) => listeners.delete(`query:${type}`),
    set(next) {
      this.matches = next;
      listeners.get('query:change')?.();
    },
  };
  const root = document.createElement('div');
  const win = {
    innerHeight,
    visualViewport: view,
    matchMedia: (quoted) => {
      expect(quoted).toBe(PHONE_QUERY);
      return query;
    },
    document: { documentElement: root },
    addEventListener: (type, fn) => listeners.set(`window:${type}`, fn),
    removeEventListener: (type) => listeners.delete(`window:${type}`),
    root,
    view,
    query,
    listeners,
  };
  return win;
}

describe('viewportMetrics', () => {
  it('reports no keyboard when the visual viewport fills the layout viewport', () => {
    expect(viewportMetrics({ windowHeight: 844, height: 844, offsetTop: 0 })).toEqual({ height: 844, inset: 0, keyboard: false });
  });

  it('reports the keyboard inset while it covers part of the layout viewport', () => {
    expect(viewportMetrics({ windowHeight: 844, height: 508, offsetTop: 0 })).toEqual({ height: 508, inset: 336, keyboard: true });
  });

  it('subtracts the visual viewport offset from the inset', () => {
    expect(viewportMetrics({ windowHeight: 844, height: 500, offsetTop: 44 })).toEqual({ height: 500, inset: 300, keyboard: true });
  });
});

describe('isPhone', () => {
  it('is true at the phone breakpoint and false above it', () => {
    const win = fakeWindow({ matches: true });
    expect(isPhone(win)).toBe(true);
    win.query.set(false);
    expect(isPhone(win)).toBe(false);
  });

  it('is false without a media query implementation', () => {
    expect(isPhone({})).toBe(false);
    expect(isPhone(null)).toBe(false);
  });
});

describe('trackViewport', () => {
  it('publishes the visible height and the keyboard inset on phones', () => {
    const win = fakeWindow();
    const stop = trackViewport(win);
    expect(win.root.style.getPropertyValue('--app-height')).toBe('844px');
    expect(win.root.style.getPropertyValue('--kb-inset')).toBe('0px');
    expect(win.root.hasAttribute('data-keyboard')).toBe(false);

    win.view.resize({ height: 512 });
    expect(win.root.style.getPropertyValue('--app-height')).toBe('512px');
    expect(win.root.style.getPropertyValue('--kb-inset')).toBe('332px');
    expect(win.root.getAttribute('data-keyboard')).toBe('open');

    win.view.resize({ height: 844 });
    expect(win.root.style.getPropertyValue('--kb-inset')).toBe('0px');
    expect(win.root.hasAttribute('data-keyboard')).toBe(false);
    stop();
  });

  it('leaves the desktop shell alone and clears phone values when the query stops matching', () => {
    const desktop = fakeWindow({ matches: false });
    const stopDesktop = trackViewport(desktop);
    expect(desktop.root.style.getPropertyValue('--app-height')).toBe('');
    expect(desktop.root.hasAttribute('data-keyboard')).toBe(false);
    stopDesktop();

    const win = fakeWindow();
    const stop = trackViewport(win);
    expect(win.root.style.getPropertyValue('--app-height')).toBe('844px');
    win.query.set(false);
    expect(win.root.style.getPropertyValue('--app-height')).toBe('');
    expect(win.root.style.getPropertyValue('--kb-inset')).toBe('');
    stop();
  });

  it('drops its listeners and values on stop', () => {
    const win = fakeWindow();
    const stop = trackViewport(win);
    stop();
    expect(win.listeners.size).toBe(0);
    expect(win.root.style.getPropertyValue('--app-height')).toBe('');
    expect(win.root.hasAttribute('data-keyboard')).toBe(false);
  });
});
