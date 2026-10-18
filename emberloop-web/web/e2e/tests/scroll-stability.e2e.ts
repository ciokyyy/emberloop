import { test } from '@e2e-dev/web';
import { expect } from 'e2e';
import { adminCookie } from '../support/auth.ts';

async function api(app: { baseUrl: string | undefined }, path: string, body?: unknown, cookie?: string) {
  const res = await fetch(`${app.baseUrl}${path}`, {
    method: body ? 'POST' : 'GET',
    headers: { 'Content-Type': 'application/json', Cookie: cookie ?? (await adminCookie(app.baseUrl)) },
    body: body ? JSON.stringify(body) : undefined,
  });
  if (!res.ok) throw new Error(`${path} failed: ${res.status} ${await res.text()}`);
  return res.json();
}

async function waitForTurn(app: { baseUrl: string | undefined }, id: string, seq: number, cookie: string) {
  for (let attempt = 0; attempt < 80; attempt++) {
    const view = await api(app, `/api/sessions/${id}`, undefined, cookie);
    if (view.session?.status === 'idle' && view.events?.some((event: any) => event.seq > seq && event.kind === 'assistant_chunk')) return;
    await new Promise((resolve) => setTimeout(resolve, 25));
  }
  throw new Error(`Session ${id} did not finish the turn after seq ${seq}`);
}

test('phone transcript anchors a visible virtualized row as a disclosure above it changes height', { session: 'admin' }, async ({ app, browser, screen }) => {
  await browser.setViewport({ width: 390, height: 844 });
  await app.open('/');
  await browser.evaluate(() => localStorage.setItem('eb.pref', JSON.stringify({ harness: 'fake', model: 'fake-model' })));
  const cookie = await adminCookie(app.baseUrl);

  const session = await api(app, '/api/sessions', { harness: 'fake', model: 'fake-model', workDir: '/tmp/el-e2e/repo' }, cookie);
  await app.open(`/s/${session.id}`);
  const transcript = screen.getByRole('region', 'Transcript');
  await expect(transcript).toBeVisible();

  // Older real Session rows give the virtualizer room to scroll above the report.
  for (let i = 0; i < 8; i++) {
    const text = `scroll seed ${i}`;
    const sent = await api(app, `/api/sessions/${session.id}/prompt`, { text }, cookie);
    await waitForTurn(app, session.id, sent.seq, cookie);
  }

  const reportPrompt = Array.from({ length: 24 }, (_, i) => `Disclosure fixture line ${i + 1}`).join('\n');
  const spawned = await api(app, `/api/sessions/${session.id}/spawn`, {
    agents: [{ name: 'Disclosure fixture', prompt: reportPrompt }],
  }, cookie);
  expect(spawned.sessions).toHaveLength(1);
  await expect(screen.getByText('Subagent report')).toBeVisible();
  await expect(screen.getByText(/fake reply to: Subagents/)).toBeVisible();

  const anchorText = 'visible anchor beneath report disclosure SCROLL_ANCHOR_02';
  const sent = await api(app, `/api/sessions/${session.id}/prompt`, { text: anchorText }, cookie);
  await waitForTurn(app, session.id, sent.seq, cookie);
  await expect(screen.getByText(anchorText)).toBeVisible();
  const tailPrompt = 'scroll stability tail '.repeat(100);
  const tail = await api(app, `/api/sessions/${session.id}/prompt`, { text: tailPrompt }, cookie);
  await waitForTurn(app, session.id, tail.seq, cookie);

  const placement = await browser.evaluate(async () => {
    const scroller = document.querySelector<HTMLElement>('[data-testid="transcript-scroll"]');
    const content = document.querySelector<HTMLElement>('[data-testid="transcript-content"]');
    if (!scroller || !content) throw new Error('Transcript scroller was not rendered');
    const disclosure = content.querySelector<HTMLDetailsElement>('details[data-card="report"]');
    const rows = Array.from(content.querySelectorAll<HTMLElement>('[data-block-key]'));
    const reportRow = disclosure?.closest<HTMLElement>('[data-block-key]');
    const reportIndex = reportRow ? rows.indexOf(reportRow) : -1;
    const anchorRow = rows.find((row) => row.querySelector('[data-card="user"]')?.textContent?.includes('SCROLL_ANCHOR_02'));
    const anchor = anchorRow?.querySelector<HTMLElement>('[data-card="user"]');
    if (!disclosure || !reportRow || !anchorRow || !anchor || reportIndex < 0 || rows.indexOf(anchorRow) <= reportIndex) {
      throw new Error('Real report and following user row were not rendered');
    }

    const virtualized = !!content.querySelector('[data-testid="transcript-virtual-list"]')?.contains(reportRow) && reportRow.hasAttribute('data-index') && reportRow.hasAttribute('data-block-key');
    for (let attempt = 0; attempt < 5; attempt++) {
      const port = scroller.getBoundingClientRect();
      const delta = anchor.getBoundingClientRect().top - (port.top + 200);
      if (Math.abs(delta) < 1) break;
      scroller.scrollTop += delta;
      await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)));
    }
    await new Promise((resolve) => setTimeout(resolve, 500));
    const port = scroller.getBoundingClientRect();
    return { virtualized, x: port.left + port.width / 2, y: port.top + port.height / 2 };
  });

  await browser.mouse.move(placement.x, placement.y);
  await browser.mouse.wheel(0, -8);
  await browser.evaluate(async () => new Promise((resolve) => setTimeout(resolve, 500)));

  const anchored = await browser.evaluate(async () => {
    const scroller = document.querySelector<HTMLElement>('[data-testid="transcript-scroll"]');
    const content = document.querySelector<HTMLElement>('[data-testid="transcript-content"]');
    if (!scroller || !content) throw new Error('Transcript scroller was not rendered');
    const disclosure = content.querySelector<HTMLDetailsElement>('details[data-card="report"]');
    const anchorRow = Array.from(content.querySelectorAll<HTMLElement>('[data-block-key]')).find((row) => row.querySelector('[data-card="user"]')?.textContent?.includes('SCROLL_ANCHOR_02'));
    const anchor = anchorRow?.querySelector<HTMLElement>('[data-card="user"]');
    if (!disclosure || !anchor) throw new Error('Report disclosure and visible user row were not rendered');

    const markerRect = () => {
      const walker = document.createTreeWalker(anchor, NodeFilter.SHOW_TEXT);
      while (walker.nextNode()) {
        const text = walker.currentNode.textContent ?? '';
        const start = text.indexOf('SCROLL_ANCHOR_02');
        if (start < 0) continue;
        const range = document.createRange();
        range.setStart(walker.currentNode, start);
        range.setEnd(walker.currentNode, start + 'SCROLL_ANCHOR_02'.length);
        return range.getBoundingClientRect();
      }
      throw new Error('Visible transcript anchor text was not rendered');
    };

    const port = scroller.getBoundingClientRect();
    const disclosureAbove = disclosure.getBoundingClientRect().bottom <= port.top;
    const beforeRect = markerRect();
    const anchorVisible = beforeRect.top >= port.top && beforeRect.top < port.bottom;
    const before = anchor.getBoundingClientRect().top;
    const closedHeight = disclosure.getBoundingClientRect().height;
    disclosure.open = true;
    await new Promise((resolve) => setTimeout(resolve, 100));
    const midOpenHeight = disclosure.getBoundingClientRect().height;
    await new Promise((resolve) => setTimeout(resolve, 300));
    const openedHeight = disclosure.getBoundingClientRect().height;
    const opened = anchor.getBoundingClientRect().top;
    disclosure.open = false;
    await new Promise((resolve) => setTimeout(resolve, 100));
    const midCloseHeight = disclosure.getBoundingClientRect().height;
    await new Promise((resolve) => setTimeout(resolve, 400));
    const closed = anchor.getBoundingClientRect().top;
    return { disclosureAbove, anchorVisible, closedHeight, midOpenHeight, openedHeight, midCloseHeight, before, opened, closed };
  });
  expect(placement.virtualized).toBe(true);
  expect(anchored.disclosureAbove).toBe(true);
  expect(anchored.anchorVisible).toBe(true);
  expect(anchored.openedHeight - anchored.closedHeight).toBeGreaterThan(50);
  expect(anchored.midOpenHeight).toBeGreaterThan(anchored.closedHeight + 1);
  expect(anchored.midOpenHeight).toBeLessThan(anchored.openedHeight - 1);
  expect(anchored.midCloseHeight).toBeGreaterThan(anchored.closedHeight + 1);
  expect(anchored.midCloseHeight).toBeLessThan(anchored.openedHeight - 1);
  if (Math.abs(anchored.opened - anchored.before) > 1 || Math.abs(anchored.closed - anchored.before) > 1) {
    throw new Error(`Disclosure shifted the visible virtual row: ${JSON.stringify(anchored)}`);
  }
});
