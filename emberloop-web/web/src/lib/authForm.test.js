import { describe, it, expect } from 'vitest';
import { COPY, MIN_PASSWORD, canCreateAccount, errorFor, modeFor } from './authForm.js';

describe('modeFor', () => {
  it('creates the admin account on an empty Daemon, whatever the user asked for', () => {
    expect(modeFor({ empty: true, signup_enabled: true }, false)).toBe('admin');
    expect(modeFor({ empty: true, signup_enabled: false }, true)).toBe('admin');
  });

  it('signs in by default and creates an account only when asked and signup is open', () => {
    expect(modeFor({ empty: false, signup_enabled: true }, false)).toBe('login');
    expect(modeFor({ empty: false, signup_enabled: true }, true)).toBe('signup');
    expect(modeFor({ empty: false, signup_enabled: false }, true)).toBe('login');
  });
});

describe('canCreateAccount', () => {
  it('offers the link only with accounts present and signup open', () => {
    expect(canCreateAccount({ empty: false, signup_enabled: true })).toBe(true);
    expect(canCreateAccount({ empty: false, signup_enabled: false })).toBe(false);
    expect(canCreateAccount({ empty: true, signup_enabled: true })).toBe(false);
  });
});

describe('copy', () => {
  it('states the password rule on the signup screen', () => {
    expect(COPY.signup.note).toContain(`${MIN_PASSWORD} characters`);
  });
});

describe('errorFor', () => {
  it('maps each status to one line', () => {
    expect(errorFor({ status: 401, message: 'invalid username or password' })).toBe('Wrong username or password.');
    expect(errorFor({ status: 409, message: 'username taken' })).toBe('That username is taken.');
    expect(errorFor({ status: 403, message: 'signup is disabled' })).toBe('Sign-up is turned off on this Daemon.');
    expect(errorFor({ status: 500, message: 'boom' })).toBe('Sign-in failed (500). Try again.');
  });

  it('shows the Daemon reason for a 400', () => {
    expect(errorFor({ status: 400, message: 'password must be 8 to 1024 characters' })).toBe('password must be 8 to 1024 characters');
  });

  it('says the Daemon is unreachable when no answer came back', () => {
    expect(errorFor(new TypeError('fetch failed'))).toBe("Can't reach the Daemon. Check the connection and try again.");
  });
});
