import { test } from '@e2e-dev/web';
import { expect } from 'e2e';
import { signIn } from '../support/auth.ts';

// No saved session here: this test signs in and out itself, so the sign-out
// revokes its own login and not the admin session the other tests restore.
test('sign in opens the shell, and sign out returns to the sign-in screen', async ({ app, screen }) => {
  await app.open('/');
  await expect(screen.getByRole('heading', { name: 'Sign in' })).toBeVisible();
  await signIn(screen);
  await expect(screen.getByLabel('Sign out')).toBeVisible();
  await screen.getByLabel('Sign out').click();
  await expect(screen.getByRole('heading', { name: 'Sign in' })).toBeVisible();
});
