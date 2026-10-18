<!-- Settings section: install the UI as a phone app. Sits next to
     Notifications: both need a secure context (https or localhost). -->
<script>
  import { onMount } from 'svelte';
  import { Smartphone } from '@lucide/svelte';
  import Button from './ui/button.svelte';
  import Panel from './ui/panel.svelte';
  import ErrorState from './ui/error-state.svelte';
  import Section from './ui/section.svelte';
  import SettingRow from './ui/setting-row.svelte';
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

<Section
  title="Install app"
  icon={Smartphone}
  hint="Add Emberloop to the home screen for a full-screen standalone window. Needs https or localhost."
  class="animate-rise"
>
  <Panel>
    <SettingRow
      label="Emberloop on this device"
      note={state === 'installed'
        ? 'Installed: running in the standalone window.'
        : state === 'prompt'
          ? 'Ready to install in this browser.'
          : state === 'ios-hint'
            ? 'On iPhone/iPad: Share, then Add to Home Screen.'
            : 'Install is not offered by this browser yet. Open this page once in Chrome/Edge on Android or Safari, then look here again.'}
      inline
    >
      {#if state === 'prompt'}
        <Button onclick={install}>Install app</Button>
      {/if}
    </SettingRow>
  </Panel>
  {#if error}<ErrorState compact title={error} class="animate-rise" />{/if}
</Section>
