<!-- Build stamp: tiny fixed bottom-right text naming the UI build and the
Daemon build (ui <hash> · be <commit>). A click copies the line and flashes
"copied"; when the Daemon serves a newer UI build a subtle pill offers a
reload. Muted 10px text, hidden on phones where the full-width composer owns
the bottom edge, shifted left while the right panel is open so it never
covers panel content. Native buttons, not the Button primitive: a full
button here would stop being a tiny diagnostic line. -->
<script>
  import { untrack } from 'svelte';
  import { fetchVersion, isStale, uiHash, versionText, copyText } from './version.js';

  let { panelOpen = false, connected = true } = $props();

  const own = uiHash();
  let commit = $state('');
  let remoteHash = $state('');
  let copied = $state(false);
  let copyTimer = null;

  async function refresh() {
    const v = await fetchVersion();
    if (v) {
      commit = v.commit;
      remoteHash = v.uiHash;
    }
  }

  const stale = $derived(isStale(own, remoteHash));
  const label = $derived(versionText(own, commit));

  $effect(() => {
    refresh();
    const t = setInterval(refresh, 30000);
    return () => clearInterval(t);
  });

  // The session stream reconnects on its own; a fresh connection may serve a
  // new Daemon build, so refetch on every reconnect.
  let was = $state(true);
  $effect(() => {
    const now = connected;
    if (now && !untrack(() => was)) refresh();
    was = now;
  });

  async function copy() {
    if (await copyText(label)) {
      copied = true;
      if (copyTimer) clearTimeout(copyTimer);
      copyTimer = setTimeout(() => (copied = false), 1200);
    }
  }
</script>

{#if commit}
<div
  class="fixed bottom-1 z-10 hidden text-[10px] leading-none text-muted-foreground sm:block {panelOpen
    ? 'right-[25.5rem]'
    : 'right-2'}"
  data-testid="version-badge"
  title="UI build and Daemon build — click to copy"
>
  <button class="cursor-pointer transition-colors hover:text-foreground" onclick={copy}>
    {copied ? 'copied' : label}
  </button>
  {#if stale}
    <button
      class="ml-1 cursor-pointer underline decoration-dotted underline-offset-2 transition-colors hover:text-foreground"
      onclick={() => window.location.reload()}
    >
      update available — reload
    </button>
  {/if}
</div>
{/if}
