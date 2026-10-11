// Pure helpers for the sign-in screen. The Daemon owns the rules (who may sign
// up, password length, username shape). These pick the copy and turn the
// Daemon's answer into the line the screen shows.

export const MIN_PASSWORD = 8;

// modeFor: an empty Daemon creates the admin account. Otherwise the screen
// signs in, unless the user asked to create an account and signup is open.
export function modeFor(status, creating) {
  if (status.empty) return 'admin';
  return creating && status.signup_enabled ? 'signup' : 'login';
}

// Only a Daemon with accounts and signup open offers the link to create one.
export const canCreateAccount = (status) => !status.empty && !!status.signup_enabled;

export const COPY = {
  admin: {
    title: 'Create the admin account',
    note: 'This Daemon has no accounts yet. The first account is the admin.',
    submit: 'Create admin account',
  },
  signup: {
    title: 'Create an account',
    note: `Passwords need at least ${MIN_PASSWORD} characters.`,
    submit: 'Create account',
  },
  login: {
    title: 'Sign in',
    note: 'Use the username and password of an account on this Daemon.',
    submit: 'Sign in',
  },
};

// errorFor turns a failed sign-in or sign-up into the line the screen shows.
// A 400 carries the Daemon's own reason, such as the username or password rule.
export function errorFor(err) {
  if (!err.status) return "Can't reach the Daemon. Check the connection and try again.";
  switch (err.status) {
    case 400:
      return err.message;
    case 401:
      return 'Wrong username or password.';
    case 403:
      return 'Sign-up is turned off on this Daemon.';
    case 409:
      return 'That username is taken.';
    default:
      return `Sign-in failed (${err.status}). Try again.`;
  }
}
