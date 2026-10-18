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

  // inline: static muted line for the phone drawer footer (the fixed corner
  // stamp stays hidden on phones where the composer owns the bottom edge).
  let { panelOpen = false, connected = true, inline = false } = $props();

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

{#if commit && inline}
<div class="flex shrink-0 items-center justify-center gap-2 pt-1 text-[11px] leading-none text-muted-foreground" data-testid="version-inline">
  <button class="min-h-11 cursor-pointer px-1 transition-colors hover:text-foreground" onclick={copy}>{copied ? 'copied' : label}</button>
  {#if stale}
    <button class="min-h-11 cursor-pointer px-1 underline decoration-dotted underline-offset-2 transition-colors hover:text-foreground" onclick={() => window.location.reload()}>update available — reload</button>
  {/if}
</div>
{:else if commit}
{#if stale}
  <button
    class="fixed top-[max(0.5rem,env(safe-area-inset-top))] left-1/2 z-40 min-h-11 -translate-x-1/2 animate-rise cursor-pointer rounded-full border bg-popover px-4 text-xs font-medium shadow-lg sm:hidden"
    data-testid="update-pill"
    onclick={() => window.location.reload()}
  >Update available — reload</button>
{/if}
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
