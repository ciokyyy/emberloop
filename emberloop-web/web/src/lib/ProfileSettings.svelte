<!-- Profile settings: per-User standing instructions with versions and approvals.
     `embedded` (Memory page) nests the Sections one heading level lower. -->
<script>
  import { Archive, Undo2, UserRound } from '@lucide/svelte';
  import { getProfile, putProfile, getProfileVersions, restoreProfile, getProfileQueue, decideProfileQueue } from './api.js';
  import Button from './ui/button.svelte';
  import EmptyState from './ui/empty-state.svelte';
  import FloatingInput from './ui/floating-input.svelte';
  import IconButton from './ui/icon-button.svelte';
  import Panel from './ui/panel.svelte';
  import Section from './ui/section.svelte';
  import Select from './ui/select.svelte';
  import Skeleton from './ui/skeleton.svelte';
  import { icons } from './icons.js';

  let { embedded = false } = $props();
  const level = $derived(embedded ? 3 : 2);

  let lines = $state([]);
  let version = $state(0);
  let rendered = $state('');
  let versions = $state([]);
  let queue = $state([]);
  let loading = $state(true);
  let error = $state('');
  let notice = $state('');
  let newText = $state('');
  let newTag = $state('workflow');
  let editingId = $state('');
  let editText = $state('');
  let editTag = $state('workflow');

  const tagItems = [
    { value: 'workflow', label: 'Workflow' },
    { value: 'style', label: 'Style' },
    { value: 'other', label: 'Other' },
  ];

  async function load() {
    loading = true;
    error = '';
    try {
      const [prof, hist, q] = await Promise.all([getProfile(), getProfileVersions(), getProfileQueue()]);
      lines = Array.isArray(prof?.lines) ? prof.lines : [];
      version = prof.version ?? 0;
      rendered = prof.rendered ?? '';
      versions = Array.isArray(hist) ? hist : [];
      queue = Array.isArray(q) ? q : [];
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  }
  load();

  async function run(fn) {
    error = '';
    notice = '';
    try {
      await fn();
      await load();
    } catch (e) {
      error = e.message;
    }
  }

  const liveLines = $derived((Array.isArray(lines) ? lines : []).filter((l) => !l?.retired));

  async function addLine() {
    const text = newText.trim();
    if (!text) {
      error = 'Write the line first.';
      return;
    }
    await run(async () => {
      await putProfile([{ op: 'add', text, tag: newTag }]);
      newText = '';
      notice = 'Profile saved.';
    });
  }

  function startEdit(line) {
    editingId = line.id;
    editText = line.text;
    editTag = line.tag;
  }

  async function saveEdit(line) {
    const text = editText.trim();
    if (!text) {
      error = 'A line cannot be empty.';
      return;
    }
    await run(async () => {
      await putProfile([{ op: 'edit', id: line.id, text, tag: editTag }]);
      editingId = '';
      notice = 'Profile saved.';
    });
  }

  async function retireLine(line) {
    await run(async () => {
      await putProfile([{ op: 'retire', id: line.id }]);
      notice = 'Line retired. History keeps it.';
    });
  }

  async function approveItem(item, ops = null) {
    await run(async () => {
      await decideProfileQueue(item.id, 'approved', ops);
      notice = 'Patch approved.';
    });
  }

  async function rejectItem(item) {
    await run(async () => {
      await decideProfileQueue(item.id, 'rejected');
      notice = 'Patch rejected. It stays as a label.';
    });
  }

  async function restoreVersion(v) {
    await run(async () => {
      await restoreProfile(v);
      notice = `Restored version ${v} as a new version.`;
    });
  }
</script>

{#snippet body()}
<div class="animate-rise flex flex-col gap-6" data-testid="profile-settings">
  {#if error}
    <p role="alert" class="rounded-xl bg-status-failed/10 px-4 py-2 text-body text-status-failed">{error}</p>
  {/if}
  {#if notice}
    <p role="status" class="rounded-xl bg-status-done/10 px-4 py-2 text-body text-status-done">{notice}</p>
  {/if}
  {#if loading}
    <div role="status" aria-label="Loading profile" class="flex flex-col gap-3">
      <Skeleton class="h-16 rounded-2xl" />
      <Skeleton class="h-16 rounded-2xl" />
    </div>
  {:else}
    <Section
      title="Standing lines"
      icon={UserRound}
      headingLevel={level}
      hint="Standing instructions injected into every Session. Changes are versions; retired lines stay in history. Workflow and style lines also go to reviewers."
    >
      {#if liveLines.length === 0}
        <EmptyState icon={UserRound} title="No profile yet" description="Add the first line below." class="py-6" data-testid="profile-empty" />
      {:else}
        <Panel tag="ul" data-testid="profile-lines">
          {#each liveLines as line (line.id)}
            <li class="flex flex-col gap-3 p-4">
              {#if editingId === line.id}
                <FloatingInput label="Line text" bind:value={editText} />
                <Select aria-label={`Tag for ${line.id}`} bind:value={editTag} items={tagItems} />
                <div class="flex gap-2 max-md:[&>*]:flex-1">
                  <Button size="sm" onclick={() => saveEdit(line)}>Save</Button>
                  <Button variant="ghost" size="sm" onclick={() => (editingId = '')}>Cancel</Button>
                </div>
              {:else}
                <div class="flex items-start gap-3">
                  <div class="flex min-w-0 flex-1 flex-col gap-0.5">
                    <span class="text-body break-words text-fg-1">{line.text}</span>
                    <span class="text-caption text-fg-3">{line.tag} · {line.id}</span>
                  </div>
                  <div class="flex shrink-0 items-center">
                    <IconButton icon={icons.edit} label={`Edit ${line.id}`} onclick={() => startEdit(line)} />
                    <IconButton icon={Archive} label={`Retire ${line.id}`} onclick={() => retireLine(line)} />
                  </div>
                </div>
              {/if}
            </li>
          {/each}
        </Panel>
      {/if}
    </Section>

    <Section title="Add a line" headingLevel={level}>
      <Panel padded divided={false}>
        <div class="flex flex-col gap-3 md:grid md:grid-cols-[minmax(0,1fr)_10rem_auto] md:items-center">
          <FloatingInput id="profile-new-text" label="New line" bind:value={newText} placeholder="Prefer tabs for Go files." />
          <Select aria-label="Tag for new line" bind:value={newTag} items={tagItems} />
          <Button onclick={addLine}>Add line</Button>
        </div>
      </Panel>
    </Section>

    <Section title={`Proposals waiting (${queue.filter((q) => q.status === 'pending').length})`} headingLevel={level} data-testid="profile-queue">
      {#if queue.length === 0}
        <p class="rounded-2xl border border-dashed px-4 py-6 text-center text-caption text-fg-3">No proposals. Agent patches appear here after an explicit request.</p>
      {:else}
        <Panel tag="ul">
          {#each queue as item (item.id)}
            <li class="flex flex-col gap-2 p-4" data-testid={`profile-queue-${item.id}`}>
              <span class="text-caption text-fg-3">#{item.id} · {item.status}</span>
              <span class="rounded-lg bg-muted px-3 py-2 font-mono text-caption break-words text-fg-2">{item.payload}</span>
              {#if item.status === 'pending'}
                <div class="flex gap-2 max-md:[&>*]:flex-1">
                  <Button size="sm" onclick={() => approveItem(item)}>Approve</Button>
                  <Button variant="ghost" size="sm" onclick={() => rejectItem(item)}>Reject</Button>
                </div>
              {/if}
            </li>
          {/each}
        </Panel>
      {/if}
    </Section>

    <Section title={`Versions (${versions.length})`} headingLevel={level} data-testid="profile-versions">
      {#if versions.length === 0}
        <p class="rounded-2xl border border-dashed px-4 py-6 text-center text-caption text-fg-3">Every approval writes a version.</p>
      {:else}
        <Panel tag="ul">
          {#each versions as v (v.version)}
            <li class="flex items-center gap-3 px-4 py-2">
              <span class="min-w-0 flex-1 text-body tabular-nums">v{v.version} · {(v.lines ?? []).filter((l) => !l.retired).length} lines</span>
              <IconButton icon={Undo2} label={`Restore version ${v.version}`} onclick={() => restoreVersion(v.version)} />
            </li>
          {/each}
        </Panel>
      {/if}
    </Section>
  {/if}
</div>
{/snippet}

{#if embedded}
  <Section title="Profile" icon={UserRound} collapsible open={false} class="animate-rise">{@render body()}</Section>
{:else}
  {@render body()}
{/if}
