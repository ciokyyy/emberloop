import { test } from '@e2e-dev/web';
import { expect } from 'e2e';
import { adminCookie } from '../support/auth.ts';

// Phone chat layout seam: at 390 px the header is one line, the composer sits
// in the safe area and above the on-screen keyboard, primary targets are 44 px,
// the Latest jump clears the composer dock, and the Session sheets close on
// system Back. The keyboard is emulated by shrinking the visual viewport, which
// is what a phone reports while the keyboard is open.

async function api(app: { baseUrl: string | undefined }, path: string, body: unknown | undefined, cookie: string) {
  const res = await fetch(`${app.baseUrl}${path}`, {
    method: body ? 'POST' : 'GET',
    headers: { 'Content-Type': 'application/json', Cookie: cookie },
    body: body ? JSON.stringify(body) : undefined,
  });
  if (!res.ok) throw new Error(`${path} failed: ${res.status} ${await res.text()}`);
  return res.json();
}

async function waitForTurn(app: { baseUrl: string | undefined }, id: string, seq: number, cookie: string) {
  for (let attempt = 0; attempt < 80; attempt++) {
    const view = await api(app, `/api/sessions/${id}`, undefined, cookie);
    if (view.session?.status === 'idle' && view.events?.some((event: { seq: number; kind: string }) => event.seq > seq && event.kind === 'assistant_chunk')) return;
    await new Promise((resolve) => setTimeout(resolve, 25));
  }
  throw new Error(`Session ${id} did not finish the turn after seq ${seq}`);
}

const waitForGone = (selector: string) => `new Promise((resolve) => {
  const check = () => (document.querySelector(${JSON.stringify(selector)}) ? setTimeout(check, 50) : resolve(true));
  check();
})`;

test('phone chat fits 390 px, keeps the composer above the keyboard and closes sheets on Back', { session: 'admin' }, async ({ app, browser, screen }) => {
  await browser.setViewport({ width: 390, height: 844 });
  await app.open('/');
  const cookie = await adminCookie(app.baseUrl);

  const session = await api(app, '/api/sessions', { harness: 'fake', model: 'fake-model', workDir: '/tmp/el-e2e/repo' }, cookie);
  for (let i = 0; i < 4; i++) {
    const text = `Phone chat fixture ${i + 1}: the composer stays above the keyboard and the Latest jump clears it.`;
    const sent = await api(app, `/api/sessions/${session.id}/prompt`, { text }, cookie);
    await waitForTurn(app, session.id, sent.seq, cookie);
  }
  await app.open(`/s/${session.id}`);
  await expect(screen.getByRole('region', 'Transcript')).toBeVisible();

  // No horizontal overflow, and every visible control is at least 44 px.
  const layout = await browser.evaluate(() => {
    const small = [...document.querySelectorAll('button, a[href], [role="button"]')]
      .filter((el) => (el as HTMLElement).offsetParent !== null)
      .map((el) => ({ label: (el.getAttribute('aria-label') || el.textContent || '').trim().slice(0, 40), height: Math.round(el.getBoundingClientRect().height) }))
      .filter((el) => el.height < 44);
    const header = document.querySelector('[data-page-header]');
    return {
      overflowX: document.documentElement.scrollWidth - window.innerWidth,
      small,
      headerLines: header ? Math.round(header.getBoundingClientRect().height) : 0,
    };
  });
  expect(layout.overflowX).toBeLessThanOrEqual(0);
  expect(layout.small).toEqual([]);
  expect(layout.headerLines).toBeLessThanOrEqual(56);

  // The Latest jump sits above the composer dock, never on top of it. Wait for
  // the transcript to stop growing first: a wheel that lands while the last
  // reply settles would fight the follow-to-bottom glide.
  const placement = await browser.evaluate(async () => {
    const scroller = document.querySelector<HTMLElement>('[data-testid="transcript-scroll"]');
    if (!scroller) throw new Error('Transcript scroller was not rendered');
    let stable = 0;
    let height = -1;
    let settled = false;
    for (let attempt = 0; attempt < 60; attempt++) {
      await new Promise((resolve) => setTimeout(resolve, 50));
      if (scroller.scrollHeight === height) stable++;
      else {
        stable = 0;
        height = scroller.scrollHeight;
      }
      if (stable >= 6) {
        settled = true;
        break;
      }
    }
    if (!settled) throw new Error('Transcript kept growing; the Latest jump would race the glide');
    const port = scroller.getBoundingClientRect();
    return { x: port.left + port.width / 2, y: port.top + port.height / 2 };
  });
  await browser.mouse.move(placement.x, placement.y);
  await browser.mouse.wheel(0, -700);
  await browser.evaluate(() => new Promise((resolve) => setTimeout(resolve, 400)));
  const jump = await browser.evaluate(() => {
    const button = document.querySelector('[data-testid="latest-jump"]');
    const dock = document.querySelector('[data-testid="composer-dock"]');
    if (!button || !dock) return null;
    const jumpBox = button.getBoundingClientRect();
    const dockBox = dock.getBoundingClientRect();
    return { jumpBottom: Math.round(jumpBox.bottom), jumpHeight: Math.round(jumpBox.height), dockTop: Math.round(dockBox.top) };
  });
  expect(jump).not.toBeNull();
  expect(jump!.jumpHeight).toBeGreaterThanOrEqual(44);
  expect(jump!.jumpBottom).toBeLessThanOrEqual(jump!.dockTop);

  // Keyboard emulation: the visual viewport shrinks without resizing the layout
  // viewport, exactly what iOS and Android report while the keyboard is open.
  const keyboard = await browser.evaluate(async () => {
    const view = window.visualViewport!;
    for (const [key, value] of Object.entries({ height: 508, offsetTop: 0, scale: 1 })) {
      Object.defineProperty(view, key, { configurable: true, value });
    }
    window.dispatchEvent(new Event('resize'));
    await new Promise((resolve) => setTimeout(resolve, 200));
    const shell = document.querySelector('.app-shell')!.getBoundingClientRect();
    const dock = document.querySelector('[data-testid="composer-dock"]')!.getBoundingClientRect();
    const composer = document.querySelector('[aria-label="composer"]')!.getBoundingClientRect();
    return {
      keyboard: document.documentElement.getAttribute('data-keyboard'),
      shellHeight: Math.round(shell.height),
      dockBottom: Math.round(dock.bottom),
      composerBottom: Math.round(composer.bottom),
      visible: 508,
    };
  });
  expect(keyboard.keyboard).toBe('open');
  expect(keyboard.shellHeight).toBeLessThanOrEqual(keyboard.visible);
  expect(keyboard.dockBottom).toBeLessThanOrEqual(keyboard.visible);
  expect(keyboard.composerBottom).toBeLessThanOrEqual(keyboard.visible);

  // Session sheet: rows for New Session, Git and Project. The explicit close
  // icon drops it in place, and system Back closes it too.
  await browser.evaluate(() => {
    for (const [key, value] of Object.entries({ height: window.innerHeight, offsetTop: 0, scale: 1 })) {
      Object.defineProperty(window.visualViewport!, key, { configurable: true, value });
    }
    window.dispatchEvent(new Event('resize'));
  });
  await screen.getByRole('button', 'Session menu').click();
  await expect(screen.getByRole('dialog', 'Session')).toBeVisible();
  await expect(screen.getByRole('button', 'New Session Start another chat')).toBeVisible();
  await screen.getByRole('button', 'Close').click();
  await browser.evaluate(waitForGone('[role="dialog"]'));
  expect(await browser.evaluate(() => window.location.pathname)).toBe(`/s/${session.id}`);

  await screen.getByRole('button', 'Session menu').click();
  await expect(screen.getByRole('dialog', 'Session')).toBeVisible();
  await browser.evaluate(() => window.history.back());
  await browser.evaluate(waitForGone('[role="dialog"]'));

  // Message actions open as a phone sheet and Back closes it too.
  await browser.evaluate(() => document.querySelector<HTMLElement>('button[aria-label="Message actions for You"]')?.click());
  await browser.evaluate(() => new Promise((resolve) => setTimeout(resolve, 200)));
  const actions = await browser.evaluate(() => {
    const menu = document.querySelector('[data-testid="message-action-menu"]');
    if (!menu) return null;
    return { buttons: [...menu.querySelectorAll('button')].map((b) => b.getAttribute('aria-label')) };
  });
  expect(actions).not.toBeNull();
  expect(actions!.buttons).toContain('Copy text');
  await browser.evaluate(() => window.history.back());
  await browser.evaluate(waitForGone('[data-testid="message-action-menu"]'));
  expect(await browser.evaluate(() => window.location.pathname)).toBe(`/s/${session.id}`);
});
