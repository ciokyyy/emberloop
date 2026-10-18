<!-- Sign-in screen: the whole app while signed out. An empty Daemon creates the
     admin account. Otherwise it signs in, with a link to create an account
     when signup is open. -->
<script>
  import { Flame, Loader } from '@lucide/svelte';
  import ErrorState from './ui/error-state.svelte';
  import Button from './ui/button.svelte';
  import FloatingInput from './ui/floating-input.svelte';
  import { login, signup } from './api.js';
  import { COPY, MIN_PASSWORD, canCreateAccount, errorFor, modeFor } from './authForm.js';

  let { status = { empty: false, signup_enabled: false }, onSignedIn = () => {} } = $props();

  let creating = $state(false);
  let username = $state('');
  let password = $state('');
  let busy = $state(false);
  let error = $state('');

  const mode = $derived(modeFor(status, creating));
  const copy = $derived(COPY[mode]);

  function toggleMode() {
    creating = !creating;
    error = '';
  }

  async function submit(event) {
    event.preventDefault();
    if (busy) return;
    busy = true;
    error = '';
    try {
      const who = mode === 'login' ? login : signup;
      onSignedIn(await who(username.trim(), password));
    } catch (err) {
      error = errorFor(err);
    } finally {
      busy = false;
    }
  }
</script>

<main class="flex min-h-dvh flex-col items-center justify-center gap-6 px-4 pt-[max(2.5rem,env(safe-area-inset-top))] pb-[max(2.5rem,env(safe-area-inset-bottom))]">
  <section class="animate-rise flex w-full max-w-sm flex-col gap-6" data-testid="auth-screen">
    <header class="flex flex-col items-center gap-3 text-center">
      <span class="grid size-14 place-items-center rounded-2xl bg-muted text-fg-1 shadow-elev-1" aria-hidden="true"><Flame class="size-7" /></span>
      <h1 class="text-headline text-fg-1">{copy.title}</h1>
      <p class="max-w-xs text-caption text-fg-3">{copy.note}</p>
    </header>

    <form class="flex flex-col gap-3 max-md:px-0 md:rounded-2xl md:border md:bg-card md:p-6 md:shadow-elev-1" onsubmit={submit}>
      <FloatingInput
        id="auth-username"
        label="Username"
        name="username"
        autocomplete="username"
        autocapitalize="none"
        spellcheck="false"
        required
        bind:value={username}
      />
      <FloatingInput
        id="auth-password"
        label="Password"
        type="password"
        name="password"
        autocomplete={mode === 'login' ? 'current-password' : 'new-password'}
        minlength={mode === 'login' ? undefined : MIN_PASSWORD}
        required
        bind:value={password}
      />
      {#if error}
        <ErrorState compact title={error} class="animate-rise" />
      {/if}
      <Button type="submit" size="lg" class="mt-2 w-full" disabled={busy} aria-busy={busy}>
        {#if busy}<Loader class="animate-spin" aria-hidden="true" />{/if}{busy ? 'Working…' : copy.submit}
      </Button>
    </form>

    {#if canCreateAccount(status)}
      <div class="text-center">
        <Button variant="ghost" size="sm" onclick={toggleMode}>{mode === 'login' ? 'Create an account' : 'Sign in instead'}</Button>
      </div>
    {/if}
  </section>
</main>
