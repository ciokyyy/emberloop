import { test } from '@e2e-dev/web';
import { expect } from 'e2e';
import { createAdmin } from '../support/auth.ts';

// Runs before every other test. The empty Daemon signs up its admin, and the
// session is saved for every test that starts signed in.
test.setup('sign up the admin', { sessions: ['admin'] }, async ({ app, screen, session }) => {
  await app.open('/');
  await createAdmin(screen);
  await expect(screen.getByLabel('Sign out')).toBeVisible();
  await session.save('admin');
});
