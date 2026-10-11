// Sign-in helpers for the e2e suite. serve.sh starts a fresh Daemon with a fresh
// state dir on every run, so the admin password is a fixed test value.

export const ADMIN = { username: 'admin', password: 'e2e-admin-pass' };

type Field = { fill(value: string): Promise<unknown> };
type Control = { click(): Promise<unknown> };
export type Screen = {
  getByLabel(label: string): Field;
  getByRole(role: string, name: string): Control;
};

// createAdmin signs up the first account on the empty Daemon. The first account is the admin.
export async function createAdmin(screen: Screen): Promise<void> {
  await screen.getByLabel('Username').fill(ADMIN.username);
  await screen.getByLabel('Password').fill(ADMIN.password);
  await screen.getByRole('button', 'Create admin account').click();
}

// signIn fills the sign-in form with the admin account.
export async function signIn(screen: Screen): Promise<void> {
  await screen.getByLabel('Username').fill(ADMIN.username);
  await screen.getByLabel('Password').fill(ADMIN.password);
  await screen.getByRole('button', 'Sign in').click();
}

// adminCookie signs the admin in over the API, for calls made from Node.
export async function adminCookie(baseUrl: string | undefined): Promise<string> {
  const res = await fetch(`${baseUrl}/api/auth/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(ADMIN),
  });
  const pair = (res.headers.get('set-cookie') ?? '').split(';')[0];
  if (!res.ok || !pair.startsWith('el_session=')) throw new Error(`admin sign-in failed: ${res.status}`);
  return pair;
}
