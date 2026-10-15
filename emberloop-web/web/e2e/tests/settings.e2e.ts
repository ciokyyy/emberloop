import { test } from '@e2e-dev/web';
import { expect } from 'e2e';

test('compression settings save and survive a reload', { session: 'admin' }, async ({ app, screen }) => {
  await app.open('/settings/context');
  await expect(screen.getByRole('heading', { name: 'Context compression' })).toBeVisible();

  await screen.getByLabel('Threshold %').fill('65');
  await screen.getByLabel('Turns').fill('2');
  await screen.getByLabel('Summary focus').fill('keep file names');
  await screen.getByRole('button', 'Save').click();
  await expect(screen.getByText('Saved')).toBeVisible();

  await app.open('/settings/context');
  await expect(screen.getByLabel('Threshold %')).toHaveValue('65');
  await expect(screen.getByLabel('Turns')).toHaveValue('2');
  await expect(screen.getByLabel('Summary focus')).toHaveValue('keep file names');
});

test('sound preferences persist on this device', { session: 'admin' }, async ({ app, screen }) => {
  await app.open('/settings/sound-voice');
  await expect(screen.getByRole('heading', { name: 'Sounds' })).toBeVisible();
  await expect(screen.getByText('Minimal')).toBeVisible();

  const mute = screen.getByRole('switch', 'Mute all sounds');
  await mute.click();
  await expect(mute).toHaveAttribute('aria-checked', 'true');

  await app.open('/settings/sound-voice');
  await expect(screen.getByRole('switch', 'Mute all sounds')).toHaveAttribute('aria-checked', 'true');
  await expect(screen.getByText('Minimal')).toBeVisible();
});

test('the admin sign-up switch saves and survives a reload', { session: 'admin' }, async ({ app, screen }) => {
  await app.open('/settings/access');
  await expect(screen.getByRole('switch', 'Allow sign-up')).toHaveAttribute('aria-checked', 'true');
  await screen.getByRole('switch', 'Allow sign-up').click();
  await screen.getByRole('button', 'Save').click();
  await expect(screen.getByText('Saved')).toBeVisible();

  await app.open('/settings/access');
  await expect(screen.getByRole('switch', 'Allow sign-up')).toHaveAttribute('aria-checked', 'false');

  // The Daemon is shared by the suite: switch sign-up back on.
  await screen.getByRole('switch', 'Allow sign-up').click();
  await screen.getByRole('button', 'Save').click();
  await expect(screen.getByText('Saved')).toBeVisible();
});
