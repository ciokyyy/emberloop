import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, within, fireEvent, cleanup } from '@testing-library/svelte';
import AuthScreen from './AuthScreen.svelte';

const alice = { id: 'u1', username: 'alice', admin: false, created: '2026-10-11T00:00:00Z' };
const open = { empty: false, signup_enabled: true };
const closed = { empty: false, signup_enabled: false };
const empty = { empty: true, signup_enabled: true };

// stubAuth answers each auth path with { status, text } and records the calls.
function stubAuth(answers) {
  const calls = [];
  vi.stubGlobal(
    'fetch',
    vi.fn(async (u, opts = {}) => {
      const path = new URL(String(u), 'http://daemon.test').pathname;
      calls.push({ path, method: opts.method ?? 'GET', body: opts.body ? JSON.parse(opts.body) : null });
      const { status = 200, text = '' } = answers[path] ?? {};
      return {
        ok: status >= 200 && status < 300,
        status,
        json: async () => JSON.parse(text),
        text: async () => text,
      };
    }),
  );
  return calls;
}

function fill(container, username, password) {
  const scope = within(container);
  fireEvent.input(scope.getByLabelText('Username'), { target: { value: username } });
  fireEvent.input(scope.getByLabelText('Password'), { target: { value: password } });
}

afterEach(() => cleanup());

describe('AuthScreen', () => {
  it('asks for the admin account on an empty Daemon, with no link', () => {
    const { container } = render(AuthScreen, { props: { status: empty } });
    const scope = within(container);
    expect(scope.getByRole('heading', { name: 'Create the admin account' })).toBeTruthy();
    expect(scope.getByRole('button', { name: 'Create admin account' })).toBeTruthy();
    expect(scope.queryByRole('button', { name: 'Create an account' })).toBeNull();
    expect(scope.getByLabelText('Password').getAttribute('autocomplete')).toBe('new-password');
  });

  it('signs in and reports the User it gets back', async () => {
    const calls = stubAuth({ '/api/auth/login': { text: JSON.stringify(alice) } });
    const onSignedIn = vi.fn();
    const { container } = render(AuthScreen, { props: { status: closed, onSignedIn } });
    fill(container, ' alice ', 'correct horse battery');
    await fireEvent.click(within(container).getByRole('button', { name: 'Sign in' }));
    await vi.waitFor(() => expect(onSignedIn).toHaveBeenCalledWith(alice));
    expect(calls.at(-1)).toEqual({
      path: '/api/auth/login',
      method: 'POST',
      body: { username: 'alice', password: 'correct horse battery' },
    });
  });

  it('shows the create-account link only when signup is open, and switches mode', async () => {
    const { container } = render(AuthScreen, { props: { status: open } });
    const scope = within(container);
    await fireEvent.click(scope.getByRole('button', { name: 'Create an account' }));
    expect(scope.getByRole('heading', { name: 'Create an account' })).toBeTruthy();
    expect(scope.getByRole('button', { name: 'Create account' })).toBeTruthy();
    expect(scope.getByLabelText('Password').getAttribute('minlength')).toBe('8');
    await fireEvent.click(scope.getByRole('button', { name: 'Sign in instead' }));
    expect(scope.getByRole('heading', { name: 'Sign in' })).toBeTruthy();
  });

  it('hides the link when signup is closed', () => {
    const { container } = render(AuthScreen, { props: { status: closed } });
    expect(within(container).queryByRole('button', { name: 'Create an account' })).toBeNull();
  });

  it('creates an account and reports the new User', async () => {
    const calls = stubAuth({ '/api/auth/signup': { status: 201, text: JSON.stringify(alice) } });
    const onSignedIn = vi.fn();
    const { container } = render(AuthScreen, { props: { status: empty, onSignedIn } });
    fill(container, 'alice', 'correct horse battery');
    await fireEvent.click(within(container).getByRole('button', { name: 'Create admin account' }));
    await vi.waitFor(() => expect(onSignedIn).toHaveBeenCalledWith(alice));
    expect(calls.at(-1).path).toBe('/api/auth/signup');
  });

  it('maps a wrong password to one line', async () => {
    stubAuth({ '/api/auth/login': { status: 401, text: 'invalid username or password\n' } });
    const { container } = render(AuthScreen, { props: { status: closed } });
    fill(container, 'alice', 'nope nope nope');
    await fireEvent.click(within(container).getByRole('button', { name: 'Sign in' }));
    expect((await within(container).findByRole('alert')).textContent).toBe('Wrong username or password.');
  });

  it('maps a taken username and a closed signup to their lines', async () => {
    stubAuth({ '/api/auth/signup': { status: 409, text: 'username taken\n' } });
    const taken = render(AuthScreen, { props: { status: open } });
    await fireEvent.click(within(taken.container).getByRole('button', { name: 'Create an account' }));
    fill(taken.container, 'alice', 'correct horse battery');
    await fireEvent.click(within(taken.container).getByRole('button', { name: 'Create account' }));
    expect((await within(taken.container).findByRole('alert')).textContent).toBe('That username is taken.');
    cleanup();

    stubAuth({ '/api/auth/signup': { status: 403, text: 'signup is disabled\n' } });
    const off = render(AuthScreen, { props: { status: empty } });
    fill(off.container, 'alice', 'correct horse battery');
    await fireEvent.click(within(off.container).getByRole('button', { name: 'Create admin account' }));
    expect((await within(off.container).findByRole('alert')).textContent).toBe('Sign-up is turned off on this Daemon.');
  });

  it('shows the Daemon reason for a rejected password', async () => {
    stubAuth({ '/api/auth/signup': { status: 400, text: 'password must be 8 to 1024 characters\n' } });
    const { container } = render(AuthScreen, { props: { status: empty } });
    fill(container, 'alice', 'short');
    await fireEvent.click(within(container).getByRole('button', { name: 'Create admin account' }));
    expect((await within(container).findByRole('alert')).textContent).toBe('password must be 8 to 1024 characters');
  });

  it('says the Daemon is unreachable when the request never answers', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => {
        throw new TypeError('fetch failed');
      }),
    );
    const { container } = render(AuthScreen, { props: { status: closed } });
    fill(container, 'alice', 'correct horse battery');
    await fireEvent.click(within(container).getByRole('button', { name: 'Sign in' }));
    expect((await within(container).findByRole('alert')).textContent).toContain("Can't reach the Daemon");
  });
});
