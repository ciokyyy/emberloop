import { test } from '@e2e-dev/web';
import { expect } from 'e2e';

// Layout probe for the shell: Dashboard, Runs and Settings have one page
// header, no horizontal overflow, and on phone the drawer controls are 44 px
// targets. Each area is reached through the UI (the e2e session cookie does
// not survive a direct deep-link load).
const SIZES = [
  { name: 'phone', width: 390, height: 844 },
  { name: 'desktop', width: 1280, height: 800 },
];

for (const size of SIZES) {
  test(`shell areas have one page header and no overflow at ${size.name} (${size.width}px)`, { session: 'admin' }, async ({ app, screen, browser }) => {
    await app.open('/');
    await browser.setViewport({ width: size.width, height: size.height });
    await expect(screen.getByRole('heading', { name: 'Dashboard' })).toBeVisible();

    const probe = () =>
      browser.evaluate(() => ({
        documentX: document.documentElement.scrollWidth - window.innerWidth,
        headers: document.querySelectorAll('[data-page-header]').length,
        h1: document.querySelectorAll('h1').length,
      }));

    let state = await probe();
    expect(state.documentX).toBeLessThanOrEqual(0);
    expect(state.headers).toBe(1);
    expect(state.h1).toBe(1);

    for (const area of [
      { tab: 'Runs', desktop: 'Specs and Runs', heading: 'Runs' },
      { tab: 'Settings', desktop: 'Settings', heading: size.width < 768 ? 'Settings' : 'Agents' },
    ]) {
      await app.open('/');
      await browser.setViewport({ width: size.width, height: size.height });
      if (size.width < 768) {
        await screen.getByRole('button', 'Project switcher').click();
        await screen.getByRole('button', area.tab).click();
      } else {
        await screen.getByRole('button', area.desktop).click();
      }
      await expect(screen.getByRole('heading', { name: area.heading, level: 1 })).toBeVisible();
      state = await probe();
      expect(state.documentX).toBeLessThanOrEqual(0);
      expect(state.headers).toBe(1);
      expect(state.h1).toBe(1);
    }
  });
}

test('phone drawer controls are 44 px targets', { session: 'admin' }, async ({ app, screen, browser }) => {
  await app.open('/');
  await browser.setViewport({ width: 390, height: 844 });
  await screen.getByRole('button', 'Project switcher').click();
  await expect(screen.getByRole('navigation', 'Areas')).toBeVisible();
  const small = await browser.evaluate(() => {
    const drawer = document.querySelector('[data-testid="drawer-scroll"]')?.parentElement;
    const controls = [...(drawer?.querySelectorAll('button, a[href]') ?? [])];
    return controls.map((el) => Math.round((el as HTMLElement).getBoundingClientRect().height)).filter((h) => h > 0 && h < 44);
  });
  expect(small).toEqual([]);
});
