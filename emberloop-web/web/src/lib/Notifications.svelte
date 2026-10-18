<!-- Settings section: push notifications when a chat turn finishes. -->
<script>
  import { onMount } from 'svelte';
  import Button from './ui/button.svelte';
  import { pushUnsupported, pushEnabled, enablePush, disablePush } from './push.js';
  import { getMemoryDigestPreference, setMemoryDigestPreference } from './api.js';
  import { Bell } from '@lucide/svelte';
  import Panel from './ui/panel.svelte';
  import Section from './ui/section.svelte';
  import ErrorState from './ui/error-state.svelte';
  import SettingRow from './ui/setting-row.svelte';
  import Switch from './ui/switch.svelte';

  let enabled = $state(false);
  let digestEnabled = $state(false);
  let busy = $state(false);
  let digestBusy = $state(false);
  let error = $state('');
  const unsupported = pushUnsupported();

  onMount(async () => {
    enabled = await pushEnabled().catch(() => false);
    digestEnabled = (await getMemoryDigestPreference().catch(() => ({ enabled: false }))).enabled;
  });

  async function toggle() {
    busy = true;
    error = '';
    try {
      if (enabled) await disablePush();
      else await enablePush();
      enabled = await pushEnabled();
    } catch (err) {
      error = err.message || 'Could not change notifications.';
    } finally {
      busy = false;
    }
  }

  async function toggleDigest(value) {
    digestBusy = true; error = '';
    try { digestEnabled = (await setMemoryDigestPreference(value)).enabled; }
    catch (err) { error = err.message || 'Could not change digest preference.'; }
    finally { digestBusy = false; }
  }
</script>

<Section title="On this device" icon={Bell} class="animate-rise" hint="Push a notification to this device when a chat turn finishes or fails.">
  <Panel>
    <SettingRow
      label="Push on this device"
      note={unsupported || (enabled ? 'On for this browser.' : 'Off for this browser.')}
      inline
    >
      <Button variant={enabled ? 'outline' : 'default'} disabled={busy || !!unsupported} onclick={toggle}>{enabled ? 'Turn off' : 'Turn on'}</Button>
    </SettingRow>
    <SettingRow label="Daily Memory digest" hint="A daily “learned N things today” notification. Off by default." inline>
      <Switch aria-label="Daily Memory digest" checked={digestEnabled} disabled={digestBusy} onCheckedChange={toggleDigest} />
    </SettingRow>
  </Panel>
  {#if error}<ErrorState compact title={error} class="animate-rise" />{/if}
</Section>
