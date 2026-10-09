import { test } from '@e2e-dev/web';
import { expect } from 'e2e';

test('compression settings save and survive a reload', async ({ app, screen }) => {
  await app.open('/');
  await screen.getByRole('button', 'Settings').click();
  await expect(screen.getByText('Context compression')).toBeVisible();

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
