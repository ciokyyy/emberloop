<!-- Root gate. Signed out, the sign-in screen is the whole app. Signed in, App
     mounts with the User; sign out unmounts App, which closes its live stream. -->
<script>
  import { onDestroy, onMount } from 'svelte';
  import App from './App.svelte';
  import AuthScreen from './lib/AuthScreen.svelte';
  import ErrorState from './lib/ui/error-state.svelte';
  import Skeleton from './lib/ui/skeleton.svelte';
  import { UNAUTHORIZED, getAuthState, getMe, logout } from './lib/api.js';

  // checking: probing the session. out: sign-in screen. in: the app.
  // offline: the Daemon did not answer.
  let phase = $state('checking');
  let user = $state(null);
  let status = $state({ empty: false, signup_enabled: false });

  async function showSignIn() {
    try {
      status = await getAuthState();
      user = null;
      phase = 'out';
    } catch {
      phase = 'offline';
    }
  }

  async function boot() {
    phase = 'checking';
    try {
      user = await getMe();
      phase = 'in';
    } catch (err) {
      if (err.status === 401) await showSignIn();
      else phase = 'offline';
    }
  }

  function signedIn(who) {
    user = who;
    phase = 'in';
  }

  async function signOut() {
    // The cookie is cleared even when the Daemon cannot revoke it now.
    await logout().catch(() => {});
    await showSignIn();
  }

  // A 401 from any other route means the session ended elsewhere: a sign-out
  // on another tab, or a password reset from the CLI.
  const onUnauthorized = () => {
    if (phase === 'in') void showSignIn();
  };

  onMount(() => {
    window.addEventListener(UNAUTHORIZED, onUnauthorized);
    void boot();
  });
  onDestroy(() => window.removeEventListener(UNAUTHORIZED, onUnauthorized));
</script>

{#if phase === 'in'}
  <App {user} onSignOut={signOut} />
{:else if phase === 'out'}
  <AuthScreen {status} onSignedIn={signedIn} />
{:else if phase === 'offline'}
  <main class="flex min-h-dvh items-center justify-center p-4">
    <ErrorState
      title="Can't reach the Daemon"
      detail="The local server did not answer. Check that it is running, then try again."
      onRetry={boot}
      class="animate-rise w-full max-w-sm"
    />
  </main>
{:else}
  <!-- Probing the session: the first paint is a placeholder, never a blank page. -->
  <main class="flex min-h-dvh items-center justify-center p-4">
    <div role="status" aria-label="Checking the session" class="flex w-full max-w-sm flex-col gap-3">
      <Skeleton class="h-14 rounded-2xl" />
      <Skeleton class="h-11 rounded-xl" />
      <Skeleton class="h-11 rounded-xl" />
      <Skeleton class="h-11 rounded-xl" />
    </div>
  </main>
{/if}
