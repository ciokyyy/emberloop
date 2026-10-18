<!-- Promotion queue: Rules proposed as project knowledge with their diffs.
  Page side of ticket 26: this queue approves (or rejects) with the diff
  preview. New proposals come from the chat Promote buttons (Card.svelte) and,
  once ticket 28 builds the Memory page rules list, from its Promote button
  via api.js proposePromotion. -->
<script>
  import { listPromotions, decidePromotion } from './api.js';
  import { FileDiff } from '@lucide/svelte';
  import Button from './ui/button.svelte';
  import Chip from './ui/chip.svelte';
  import EmptyState from './ui/empty-state.svelte';
  import ErrorState from './ui/error-state.svelte';
  import Panel from './ui/panel.svelte';
  import Section from './ui/section.svelte';
  import Skeleton from './ui/skeleton.svelte';

  let { embedded = false } = $props();
  const PROMOTION_HELP = "Rules proposed as edits to the nearest AGENTS.md or new note files. Approving writes the file in the Session's worktree and leaves it uncommitted for review.";
  const STATUS_TONE = { pending: 'needs-you', approved: 'done', rejected: 'paused' };

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

<Section
  title={`Promotions${pending.length ? ` (${pending.length})` : ''}`}
  icon={FileDiff}
  headingLevel={2}
  collapsible={embedded}
  hint={embedded ? null : PROMOTION_HELP}
  description={embedded ? PROMOTION_HELP : ''}
  open={!embedded}
  class="animate-rise"
  data-testid="promotion-queue"
>
  {#if error && !queue.length}
    <ErrorState title="Could not load promotions" detail={error} onRetry={load} />
  {:else if error}
    <ErrorState compact title={error} onRetry={load} />
  {/if}
  {#if notice}
    <p role="status" class="rounded-xl bg-status-done/10 px-4 py-2 text-body text-status-done">{notice}</p>
  {/if}
  {#if loading}
    <div role="status" aria-label="Loading promotions"><Skeleton class="h-16 rounded-2xl" /></div>
  {:else if queue.length === 0}
    <EmptyState icon={FileDiff} title="No promotions" description="Propose one from an injected Rule in the chat." class="py-6" data-testid="promotion-empty" />
  {:else}
    <Panel tag="ul">
      {#each queue as item (item.id)}
        <li class="flex flex-col gap-3 p-4" data-testid={`promotion-queue-${item.id}`}>
          <div class="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
            <Chip tone={STATUS_TONE[item.status] ?? 'neutral'} dot>{item.status}</Chip>
            <span class="text-caption tabular-nums text-fg-3">#{item.id}</span>
            <span class="text-caption text-fg-3">{item.target === 'note' ? 'note file' : 'AGENTS.md'}</span>
            <span class="min-w-0 break-all font-mono text-caption text-fg-2">{item.target_path}</span>
          </div>
          {#if item.diff}
            <pre class="max-h-56 overflow-auto rounded-xl bg-muted p-3 font-mono text-caption whitespace-pre-wrap" data-testid={`promotion-diff-${item.id}`}>{item.diff}</pre>
          {/if}
          {#if item.status === 'pending'}
            <div class="flex gap-2 max-md:[&>*]:flex-1">
              <Button size="sm" disabled={busyId === item.id} onclick={() => decide(item, 'approved')}>Approve</Button>
              <Button variant="ghost" size="sm" disabled={busyId === item.id} onclick={() => decide(item, 'rejected')}>Reject</Button>
            </div>
          {/if}
        </li>
      {/each}
    </Panel>
  {/if}
</Section>
