<!-- One-line banner for a Session whose Compress stopped on a credit or auth failure.
     Retry, Change model and Dismiss are icon actions with tooltips. -->
<script>
  import { CircleAlert } from '@lucide/svelte';
  import IconButton from './ui/icon-button.svelte';
  import InfoHint from './ui/info-hint.svelte';
  import ErrorState from './ui/error-state.svelte';
  import { icons } from './icons.js';
  import { compressSession, dismissCompress } from './api.js';
  let { sessionId, onChangeModel = () => {}, onDismissed = () => {} } = $props();
  let busy = $state(false);
  let error = $state('');
  // run takes one request: the buttons stay off while it runs, and its refusal shows in place.
  async function run(request) {
    busy = true;
    error = '';
    try {
      await request();
    } catch (err) {
      error = err.message;
    } finally {
      busy = false;
    }
  }
  const retry = () => run(() => compressSession(sessionId));
  const dismiss = () =>
    run(async () => {
      await dismissCompress(sessionId);
      onDismissed();
    });
</script>

<div class="animate-rise flex flex-col gap-1 rounded-2xl border border-status-needs-you/40 bg-card py-1 pl-3 pr-1" role="status" data-testid="compress-banner">
  <div class="flex min-w-0 items-center gap-1">
    <CircleAlert class="mr-1 size-4 shrink-0 text-status-needs-you" aria-hidden="true" />
    <span class="min-w-0 flex-1 truncate text-body font-medium"><span class="sm:hidden" aria-hidden="true">Needs pig credits</span><span class="max-sm:sr-only">compress needs pig credits</span></span>
    <span class="max-sm:hidden"><InfoHint label="About the compress block">Messages wait until compress succeeds or this banner is dismissed.</InfoHint></span>
    <IconButton icon={icons.retry} label="Retry" disabled={busy} onclick={retry} />
    <IconButton icon={icons.model} label="Change compress model" onclick={() => onChangeModel()} />
    <IconButton icon={icons.dismiss} label="Dismiss" disabled={busy} onclick={dismiss} />
  </div>
  {#if error}<ErrorState compact title={error} class="mx-1 mb-1" />{/if}
</div>
