<!-- Settings section: push notifications when a chat turn finishes. -->
<script>
  import { onMount } from 'svelte';
  import Button from './ui/button.svelte';
  import { pushUnsupported, pushEnabled, enablePush, disablePush } from './push.js';
  import { getMemoryDigestPreference, setMemoryDigestPreference } from './api.js';
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

<section class="animate-rise flex flex-col gap-2">
  <div class="px-1">
    <h2 class="text-sm font-semibold">Notifications</h2>
    <p class="text-xs text-muted-foreground">Push a notification to this device when a chat turn finishes or fails.</p>
  </div>
  <div class="overflow-hidden rounded-2xl border bg-card">
    <div class="grid items-center gap-3 p-4 md:grid-cols-[1fr_auto]">
      <div class="min-w-0">
        <div class="text-sm font-medium">Push on this device</div>
        <div class="text-xs text-muted-foreground">{unsupported || (enabled ? 'On for this browser.' : 'Off for this browser.')}</div>
        {#if error}<p role="alert" class="pt-1 text-xs text-destructive">{error}</p>{/if}
      </div>
      <Button variant={enabled ? 'outline' : 'default'} disabled={busy || !!unsupported} onclick={toggle}>{enabled ? 'Turn off' : 'Turn on'}</Button>
    </div>
  </div>
    <div class="grid items-center gap-3 border-t p-4 md:grid-cols-[1fr_auto]">
      <div class="min-w-0"><div class="text-sm font-medium">Daily Memory digest</div><div class="text-xs text-muted-foreground">A daily “learned N things today” notification. Off by default.</div></div>
      <Switch aria-label="Daily Memory digest" checked={digestEnabled} disabled={digestBusy} onCheckedChange={toggleDigest} />
    </div>
</section>
