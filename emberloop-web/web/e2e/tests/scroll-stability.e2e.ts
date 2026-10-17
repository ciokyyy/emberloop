import { test } from '@e2e-dev/web';
import { expect } from 'e2e';

test('phone transcript keeps its viewport anchored through disclosure height changes', { session: 'admin' }, async ({ app, browser, screen }) => {
  await browser.setViewport({ width: 390, height: 844 });
  await app.open('/');
  await browser.evaluate(() => localStorage.setItem('eb.pref', JSON.stringify({ harness: 'fake', model: 'fake-model' })));
  await app.open('/p/none');

  const transcript = screen.getByRole('region', 'Transcript');
  await expect(transcript).toBeVisible();
  const size = await browser.evaluate(() => {
    const el = document.querySelector<HTMLElement>('[data-testid="transcript-scroll"]');
    if (!el) throw new Error('Transcript scroller was not rendered');
    return { width: window.innerWidth, scrollWidth: document.documentElement.scrollWidth, height: el.clientHeight };
  });
  expect(size.width).toBe(390);
  expect(size.scrollWidth).toBeLessThanOrEqual(size.width);
  expect(size.height).toBeGreaterThan(0);

  const anchored = await browser.evaluate(async () => {
    const scroller = document.querySelector<HTMLElement>('[data-testid="transcript-scroll"]');
    const content = document.querySelector<HTMLElement>('[data-testid="transcript-content"]');
    if (!scroller || !content) throw new Error('Transcript content was not rendered');
    scroller.dispatchEvent(new WheelEvent('wheel', { deltaY: -10, bubbles: true }));
    const earlier = document.createElement('div');
    earlier.textContent = 'Earlier history';
    earlier.style.height = '1000px';
    const disclosure = document.createElement('details');
    disclosure.innerHTML = '<summary>Disclosure fixture</summary><div style="height: 180px">Details</div>';
    const spacer = document.createElement('div');
    spacer.style.height = '200px';
    const visible = document.createElement('div');
    visible.textContent = 'Visible reading position';
    visible.style.height = '1000px';
    content.replaceChildren(earlier, disclosure, spacer, visible);

    // Put the disclosure fully above the scrollport and a visible block below
    // it. Resizing the disclosure must preserve that block's viewport position.
    const rootTop = scroller.getBoundingClientRect().top;
    scroller.scrollTop += disclosure.getBoundingClientRect().bottom - (rootTop - 20);
    await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)));
    const disclosureAbove = disclosure.getBoundingClientRect().bottom <= scroller.getBoundingClientRect().top;
    const before = visible.getBoundingClientRect().top;
    const visibleInPort = before >= scroller.getBoundingClientRect().top && before < scroller.getBoundingClientRect().bottom;
    disclosure.open = true;
    await new Promise((resolve) => setTimeout(resolve, 400));
    const opened = visible.getBoundingClientRect().top;
    disclosure.open = false;
    await new Promise((resolve) => setTimeout(resolve, 400));
    const closed = visible.getBoundingClientRect().top;
    return { disclosureAbove, visibleInPort, before, opened, closed };
  });
  expect(anchored.disclosureAbove).toBe(true);
  expect(anchored.visibleInPort).toBe(true);
  expect(Math.abs(anchored.opened - anchored.before)).toBeLessThanOrEqual(1);
  expect(Math.abs(anchored.closed - anchored.before)).toBeLessThanOrEqual(1);
});
