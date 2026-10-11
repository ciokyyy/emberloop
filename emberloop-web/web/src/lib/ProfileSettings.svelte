<!-- Profile settings: per-User standing instructions with versions and approvals. -->
<script>
  import { getProfile, putProfile, getProfileVersions, restoreProfile, getProfileQueue, decideProfileQueue } from './api.js';
  import Button from './ui/button.svelte';
  import FloatingInput from './ui/floating-input.svelte';
  import Select from './ui/select.svelte';

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

<section class="animate-rise flex flex-col gap-2" data-testid="profile-settings">
  <div class="px-1">
    <h2 class="text-sm font-semibold">Profile</h2>
    <p class="text-xs text-muted-foreground">Standing instructions injected into every Session. Changes are versions; retired lines stay in history. Workflow and style lines also go to reviewers.</p>
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
    {:else}
      {#if liveLines.length === 0}
        <p class="text-sm text-muted-foreground" data-testid="profile-empty">No profile yet. Add the first line below.</p>
      {:else}
        <ul class="flex flex-col gap-2" data-testid="profile-lines">
          {#each liveLines as line (line.id)}
            <li class="flex flex-col gap-2 rounded-xl border p-3">
              {#if editingId === line.id}
                <FloatingInput label="Line text" bind:value={editText} />
                <Select aria-label={`Tag for ${line.id}`} bind:value={editTag} items={tagItems} />
                <div class="flex gap-2">
                  <Button size="sm" onclick={() => saveEdit(line)}>Save</Button>
                  <Button variant="ghost" size="sm" onclick={() => (editingId = '')}>Cancel</Button>
                </div>
              {:else}
                <div class="flex items-start justify-between gap-3">
                  <div class="min-w-0">
                    <div class="text-sm">{line.text}</div>
                    <div class="text-xs text-muted-foreground">{line.tag} · {line.id}</div>
                  </div>
                  <div class="flex shrink-0 gap-2">
                    <Button variant="ghost" size="sm" aria-label={`Edit ${line.id}`} onclick={() => startEdit(line)}>Edit</Button>
                    <Button variant="ghost" size="sm" aria-label={`Retire ${line.id}`} onclick={() => retireLine(line)}>Retire</Button>
                  </div>
                </div>
              {/if}
            </li>
          {/each}
        </ul>
      {/if}
      <div class="flex flex-col gap-2 border-t pt-3">
        <FloatingInput id="profile-new-text" label="New line" bind:value={newText} placeholder="Prefer tabs for Go files." />
        <Select aria-label="Tag for new line" bind:value={newTag} items={tagItems} />
        <div><Button size="sm" onclick={addLine}>Add line</Button></div>
      </div>
      <div class="border-t pt-3" data-testid="profile-queue">
        <h3 class="text-sm font-medium">Proposals waiting for approval ({queue.filter((q) => q.status === 'pending').length})</h3>
        {#if queue.length === 0}
          <p class="text-xs text-muted-foreground">No proposals. Agent patches appear here after an explicit request.</p>
        {:else}
          <ul class="mt-2 flex flex-col gap-2">
            {#each queue as item (item.id)}
              <li class="rounded-xl border p-3" data-testid={`profile-queue-${item.id}`}>
                <div class="text-xs text-muted-foreground">#{item.id} · {item.status}</div>
                <div class="mt-1 text-xs break-words">{item.payload}</div>
                {#if item.status === 'pending'}
                  <div class="mt-2 flex gap-2">
                    <Button size="sm" onclick={() => approveItem(item)}>Approve</Button>
                    <Button variant="ghost" size="sm" onclick={() => rejectItem(item)}>Reject</Button>
                  </div>
                {/if}
              </li>
            {/each}
          </ul>
        {/if}
      </div>
      <div class="border-t pt-3" data-testid="profile-versions">
        <h3 class="text-sm font-medium">Versions ({versions.length})</h3>
        {#if versions.length === 0}
          <p class="text-xs text-muted-foreground">Every approval writes a version.</p>
        {:else}
          <ul class="mt-2 flex flex-col gap-1">
            {#each versions as v (v.version)}
              <li class="flex items-center justify-between gap-3 text-sm">
                <span>v{v.version} · {(v.lines ?? []).filter((l) => !l.retired).length} lines</span>
                <Button variant="ghost" size="sm" aria-label={`Restore version ${v.version}`} onclick={() => restoreVersion(v.version)}>Restore</Button>
              </li>
            {/each}
          </ul>
        {/if}
      </div>
    {/if}
  </div>
</section>
