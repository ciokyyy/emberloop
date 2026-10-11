<!-- Sign-in screen: the whole app while signed out. An empty Daemon creates the
     admin account. Otherwise it signs in, with a link to create an account
     when signup is open. -->
<script>
  import { Flame } from '@lucide/svelte';
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

<main class="flex min-h-dvh items-center justify-center px-4 py-10">
  <section class="animate-rise flex w-full max-w-sm flex-col gap-6" data-testid="auth-screen">
    <header class="flex flex-col items-center gap-3 text-center">
      <span class="grid size-12 place-items-center rounded-2xl bg-muted"><Flame class="size-6" /></span>
      <h1 class="text-2xl font-semibold tracking-tight">{copy.title}</h1>
      <p class="text-sm text-muted-foreground">{copy.note}</p>
    </header>

    <form class="flex flex-col gap-3" onsubmit={submit}>
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
        <p role="alert" class="rounded-xl bg-destructive/10 px-4 py-2 text-sm text-destructive">{error}</p>
      {/if}
      <Button type="submit" size="lg" class="mt-2 w-full" disabled={busy}>{busy ? 'Working…' : copy.submit}</Button>
    </form>

    {#if canCreateAccount(status)}
      <div class="text-center">
        <Button variant="ghost" size="sm" onclick={toggleMode}>{mode === 'login' ? 'Create an account' : 'Sign in instead'}</Button>
      </div>
    {/if}
  </section>
</main>
