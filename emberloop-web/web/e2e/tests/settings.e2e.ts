import { test } from '@e2e-dev/web';
import { expect } from 'e2e';

test('compression settings save and survive a reload', async ({ app, screen }) => {
  await app.open('/');
  await screen.getByRole('button', 'Settings').click();
  await expect(screen.getByRole('heading', { name: 'Context compression' })).toBeVisible();

  await screen.getByLabel('Threshold %').fill('65');
  await screen.getByLabel('Turns').fill('2');
  await screen.getByLabel('Summary focus').fill('keep file names');
  await screen.getByRole('button', 'Save').click();
  await expect(screen.getByText('Saved')).toBeVisible();

  await app.open('/');
  await screen.getByRole('button', 'Settings').click();
  await expect(screen.getByLabel('Threshold %')).toHaveValue('65');
  await expect(screen.getByLabel('Turns')).toHaveValue('2');
  await expect(screen.getByLabel('Summary focus')).toHaveValue('keep file names');
});

test('sound preferences persist on this device', async ({ app, screen }) => {
  await app.open('/');
  await screen.getByRole('button', 'Settings').click();
  await expect(screen.getByRole('heading', { name: 'Sounds' })).toBeVisible();
  await expect(screen.getByText('Minimal')).toBeVisible();

  const mute = screen.getByRole('switch', 'Mute all sounds');
  await mute.click();
  await expect(mute).toHaveAttribute('aria-checked', 'true');

  await app.open('/');
  await screen.getByRole('button', 'Settings').click();
  await expect(screen.getByRole('switch', 'Mute all sounds')).toHaveAttribute('aria-checked', 'true');
  await expect(screen.getByText('Minimal')).toBeVisible();
});
