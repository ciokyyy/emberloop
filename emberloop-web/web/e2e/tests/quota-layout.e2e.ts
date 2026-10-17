import { test } from '@e2e-dev/web';
import { expect } from 'e2e';

// Layout probe for the design foundation's reference page: no horizontal
// overflow and 44 px phone targets at 390 px, no overflow at 800 px (rail plus
// sidebar beside the content) and 1280 px. The Daemon's quota cache may be
// empty, so the page shows cards or an empty state; the probe holds for both.
const SIZES = [
  { name: 'phone', width: 390, height: 844 },
  { name: 'narrow desktop', width: 800, height: 900 },
  { name: 'desktop', width: 1280, height: 800 },
];

for (const size of SIZES) {
  test(`quota page has no horizontal overflow at ${size.name} (${size.width}px)`, { session: 'admin' }, async ({ app, screen, browser }) => {
    await app.open('/');
    await browser.setViewport({ width: size.width, height: size.height });
    // Reach the page through the UI: the e2e session cookie does not survive a direct /quota load.
    if (size.width < 768) {
      await screen.getByRole('button', 'Project switcher').click();
      await screen.getByRole('button', 'Quota').click();
    } else {
      await screen.getByRole('button', 'Quota and Providers').click();
    }
    await expect(screen.getByRole('heading', { name: 'Quota' })).toBeVisible();
    await expect(screen.getByRole('button', { name: 'Refresh all' })).toBeVisible();

    const overflow = await browser.evaluate(() => {
      const page = document.querySelector('[data-testid="quota-page"]');
      const scroller = page?.firstElementChild as HTMLElement | null;
      return {
        documentX: document.documentElement.scrollWidth - window.innerWidth,
        scrollerX: scroller ? scroller.scrollWidth - scroller.clientWidth : 0,
      };
    });
    expect(overflow.documentX).toBeLessThanOrEqual(0);
    expect(overflow.scrollerX).toBeLessThanOrEqual(0);

    if (size.width < 768) {
      const small = await browser.evaluate(() => {
        const controls = [...document.querySelectorAll('[data-testid="quota-page"] button, [data-testid="quota-page"] a[href^="/providers"]')];
        return controls.map((el) => Math.round((el as HTMLElement).getBoundingClientRect().height)).filter((h) => h < 44);
      });
      expect(small).toEqual([]);
    }
  });
}
