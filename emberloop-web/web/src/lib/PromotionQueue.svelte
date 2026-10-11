<!-- Promotion queue: Rules proposed as project knowledge with their diffs.
  Page side of ticket 26: this queue approves (or rejects) with the diff
  preview. New proposals come from the chat Promote buttons (Card.svelte) and,
  once ticket 28 builds the Memory page rules list, from its Promote button
  via api.js proposePromotion. -->
<script>
  import { listPromotions, decidePromotion } from './api.js';
  import Button from './ui/button.svelte';

  let queue = $state([]);
  let loading = $state(true);
  let error = $state('');
  let notice = $state('');
  let busyId = $state(null);

  async function load() {
    loading = true;
    error = '';
    try {
      const q = await listPromotions();
      queue = Array.isArray(q) ? q : [];
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  }
  load();

  // Refresh the queue from the Daemon. The Memory page calls this after
  // proposing so the new item appears without a full reload.
  export async function refresh() {
    await load();
  }

  async function decide(item, status) {
    error = '';
    notice = '';
    busyId = item.id;
    try {
      await decidePromotion(item.id, status);
      notice = status === 'approved' ? 'Promotion approved. The worktree file holds the edit, uncommitted.' : 'Promotion rejected. It stays as a label.';
      await load();
    } catch (e) {
      error = e.message;
    } finally {
      busyId = null;
    }
  }

  const pending = $derived(queue.filter((q) => q.status === 'pending'));
</script>

<section class="animate-rise flex flex-col gap-2" data-testid="promotion-queue">
  <div class="px-1">
    <h2 class="text-sm font-semibold">Promotions to project knowledge</h2>
    <p class="text-xs text-muted-foreground">Rules proposed as edits to the nearest AGENTS.md or new note files. Approving writes the file in the Session's worktree and leaves it uncommitted for review.</p>
  </div>
  <div class="flex flex-col gap-3 rounded-2xl border bg-card p-4">
    {#if error}
      <p role="alert" class="rounded-xl bg-destructive/10 px-4 py-2 text-sm text-destructive">{error}</p>
    {/if}
    {#if notice}
      <p role="status" class="rounded-xl bg-green-500/10 px-4 py-2 text-sm text-green-300">{notice}</p>
    {/if}
    {#if loading}
      <div class="h-16 animate-pulse rounded-xl bg-muted"></div>
    {:else if queue.length === 0}
      <p class="text-sm text-muted-foreground" data-testid="promotion-empty">No promotions. Propose one from an injected Rule in the chat.</p>
    {:else}
      <h3 class="text-sm font-medium">Waiting for approval ({pending.length})</h3>
      <ul class="flex flex-col gap-2">
        {#each queue as item (item.id)}
          <li class="rounded-xl border p-3" data-testid={`promotion-queue-${item.id}`}>
            <div class="text-xs text-muted-foreground">#{item.id} · {item.status} · {item.target === 'note' ? 'note file' : 'AGENTS.md'} · {item.target_path}</div>
            {#if item.diff}
              <pre class="mt-2 max-h-56 overflow-auto rounded-xl bg-muted/60 p-3 font-mono text-xs leading-relaxed whitespace-pre-wrap" data-testid={`promotion-diff-${item.id}`}>{item.diff}</pre>
            {/if}
            {#if item.status === 'pending'}
              <div class="mt-2 flex gap-2">
                <Button size="sm" disabled={busyId === item.id} onclick={() => decide(item, 'approved')}>Approve</Button>
                <Button variant="ghost" size="sm" disabled={busyId === item.id} onclick={() => decide(item, 'rejected')}>Reject</Button>
              </div>
            {/if}
          </li>
        {/each}
      </ul>
    {/if}
  </div>
</section>
