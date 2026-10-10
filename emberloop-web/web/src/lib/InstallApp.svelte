<!-- Settings section: install the UI as a phone app. Sits next to
     Notifications: both need a secure context (https or localhost). -->
<script>
  import { onMount } from 'svelte';
  import Button from './ui/button.svelte';
  import { isIOSDevice, isStandalone, installState } from './install.js';

  let deferred = $state(null);
  let state = $state('unavailable');
  let error = $state('');

  function refresh() {
    const standalone = isStandalone({
      matchMedia: typeof window !== 'undefined' ? window.matchMedia?.bind(window) : undefined,
      standalone: typeof navigator !== 'undefined' ? navigator.standalone : false,
    });
    const ios = isIOSDevice(typeof navigator !== 'undefined' ? navigator.userAgent : '');
    state = installState({ standalone, hasPrompt: !!deferred, ios });
  }

  onMount(() => {
    refresh();
    const onPrompt = (e) => {
      e.preventDefault();
      deferred = e;
      refresh();
    };
    const onInstalled = () => {
      deferred = null;
      refresh();
    };
    window.addEventListener('beforeinstallprompt', onPrompt);
    window.addEventListener('appinstalled', onInstalled);
    return () => {
      window.removeEventListener('beforeinstallprompt', onPrompt);
      window.removeEventListener('appinstalled', onInstalled);
    };
  });

  async function install() {
    if (!deferred) return;
    error = '';
    try {
      await deferred.prompt();
      await deferred.userChoice?.catch?.(() => {});
    } catch (err) {
      error = err.message || 'Could not open the install prompt.';
    } finally {
      deferred = null;
      refresh();
    }
  }
</script>

<section class="animate-rise flex flex-col gap-2">
  <div class="px-1">
    <h2 class="text-sm font-semibold">Install app</h2>
    <p class="text-xs text-muted-foreground">Add Emberloop to the home screen for a full-screen standalone window. Needs https or localhost.</p>
  </div>
  <div class="overflow-hidden rounded-2xl border bg-card">
    <div class="grid items-center gap-3 p-4 md:grid-cols-[1fr_auto]">
      <div class="min-w-0">
        <div class="text-sm font-medium">Emberloop on this device</div>
        <div class="text-xs text-muted-foreground">
          {#if state === 'installed'}Installed: running in the standalone window.
          {:else if state === 'prompt'}Ready to install in this browser.
          {:else if state === 'ios-hint'}On iPhone/iPad: Share, then Add to Home Screen.
          {:else}Install is not offered by this browser yet. Open this page once in Chrome/Edge on Android or Safari, then look here again.{/if}
        </div>
        {#if error}<p role="alert" class="pt-1 text-xs text-destructive">{error}</p>{/if}
      </div>
      {#if state === 'prompt'}
        <Button onclick={install}>Install app</Button>
      {/if}
    </div>
  </div>
</section>
