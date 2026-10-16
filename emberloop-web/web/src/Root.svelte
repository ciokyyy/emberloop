<!-- Root gate. Signed out, the sign-in screen is the whole app. Signed in, App
     mounts with the User; sign out unmounts App, which closes its live stream. -->
<script>
  import { onDestroy, onMount, tick } from 'svelte';
  import App from './App.svelte';
  import AuthScreen from './lib/AuthScreen.svelte';
  import Button from './lib/ui/button.svelte';
  import { UNAUTHORIZED, getAuthState, getMe, logout } from './lib/api.js';

  // checking: probing the session. out: sign-in screen. in: the app.
  // offline: the Daemon did not answer.
  let phase = $state('checking');
  let user = $state(null);
  let status = $state({ empty: false, signup_enabled: false });


  function removeServerSnapshot() {
    if (typeof document !== 'undefined') document.getElementById('el-ssr-dashboard')?.remove();
  }
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
      await tick();
      removeServerSnapshot();
    } catch (err) {
      if (err.status === 401) await showSignIn();
      else phase = 'offline';
    }
  }

  async function signedIn(who) {
    user = who;
    phase = 'in';
    await tick();
    removeServerSnapshot();
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
  <main class="animate-rise flex min-h-dvh flex-col items-center justify-center gap-4 px-4 text-center">
    <p class="text-sm text-muted-foreground">Can't reach the Daemon.</p>
    <Button variant="secondary" onclick={boot}>Try again</Button>
  </main>
{/if}
