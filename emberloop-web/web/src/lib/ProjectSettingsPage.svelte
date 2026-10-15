<script>
  import { ArrowLeft, Save, Trash2 } from '@lucide/svelte';
  import Button from './ui/button.svelte';
  import Input from './ui/input.svelte';

  let {
    project = null,
    loading = false,
    onBack = () => {},
    onRename = async () => {},
    onSaveGist = async () => {},
    onRemove = async () => {},
  } = $props();

  let name = $state('');
  let gist = $state('');
  let savingName = $state(false);
  let savingGist = $state(false);
  let removing = $state(false);
  let error = $state('');
  let initializedProject = '';
  let syncedProjectName = '';
  let syncedProjectGist = '';

  $effect(() => {
    const current = project;
    if (!current) return;
    if (initializedProject !== current.id) {
      initializedProject = current.id;
      name = current.name ?? '';
      gist = current.gist ?? '';
    } else {
      if (!savingName && name.trim() === syncedProjectName) name = current.name ?? '';
      if (!savingGist && gist === syncedProjectGist) gist = current.gist ?? '';
    }
    syncedProjectName = current.name ?? '';
    syncedProjectGist = current.gist ?? '';
  });

  const nameDirty = $derived(name.trim() !== (project?.name ?? ''));
  const gistDirty = $derived(gist !== (project?.gist ?? ''));

  async function saveName() {
    if (!project || savingName || !name.trim() || !nameDirty) return;
    savingName = true;
    error = '';
    try {
      const updated = await onRename(project, name.trim());
      name = updated?.name ?? name.trim();
    } catch {
      error = 'Could not rename this Project.';
    } finally {
      savingName = false;
    }
  }

  async function saveGist() {
    if (!project || savingGist || !gistDirty) return;
    savingGist = true;
    error = '';
    try {
      const updated = await onSaveGist(project, gist);
      gist = updated?.gist ?? gist;
    } catch {
      error = 'Could not save the Project gist.';
    } finally {
      savingGist = false;
    }
  }

  async function remove() {
    if (!project || removing) return;
    removing = true;
    error = '';
    try {
      await onRemove(project);
    } catch {
      error = 'Could not remove this Project.';
    } finally {
      removing = false;
    }
  }
</script>

<main class="min-h-0 flex-1 overflow-auto" data-testid="project-settings-page">
  <div class="mx-auto flex w-full max-w-3xl flex-col gap-6 px-4 py-6 sm:px-6">
    <Button variant="ghost" class="-ml-3 w-fit" onclick={onBack}>
      <ArrowLeft />Back to Sessions
    </Button>

    {#if loading}
      <p role="status">Loading Project settings…</p>
    {:else if !project}
      <section class="flex flex-col gap-2" data-testid="project-settings-missing">
        <h1 class="text-xl font-semibold tracking-tight">Project not found</h1>
        <p class="text-sm text-muted-foreground">This Project may have been removed.</p>
      </section>
    {:else}
      <header class="flex flex-col gap-1">
        <h1 class="text-xl font-semibold tracking-tight">{project.name} settings</h1>
        <p class="break-all text-sm text-muted-foreground">{project.path}</p>
      </header>

      {#if error}
        <p role="alert" class="rounded-xl bg-destructive/10 px-4 py-2 text-sm text-destructive">{error}</p>
      {/if}

      <section class="flex flex-col gap-3 rounded-2xl border bg-card p-4" aria-labelledby="project-name-heading">
        <div>
          <h2 id="project-name-heading" class="font-semibold">Project name</h2>
          <p class="text-sm text-muted-foreground">Rename this Project without changing its stable URL.</p>
        </div>
        <form class="flex flex-col gap-3 sm:flex-row" onsubmit={(event) => { event.preventDefault(); void saveName(); }}>
          <Input aria-label="Project name" bind:value={name} autocomplete="off" />
          <Button type="submit" disabled={savingName || !name.trim() || !nameDirty}>
            <Save />{savingName ? 'Saving…' : 'Save name'}
          </Button>
        </form>
      </section>

      <section class="flex flex-col gap-3 rounded-2xl border bg-card p-4" aria-labelledby="project-gist-heading">
        <div>
          <h2 id="project-gist-heading" class="font-semibold">Project gist</h2>
          <p class="text-sm text-muted-foreground">A one-line summary used when suggesting a Project for a new Session.</p>
        </div>
        <form class="flex flex-col gap-3 sm:flex-row" onsubmit={(event) => { event.preventDefault(); void saveGist(); }}>
          <Input aria-label="Project gist" bind:value={gist} autocomplete="off" />
          <Button type="submit" disabled={savingGist || !gistDirty}>
            <Save />{savingGist ? 'Saving…' : 'Save gist'}
          </Button>
        </form>
      </section>

      <section class="flex flex-col gap-3 rounded-2xl border border-destructive/30 bg-card p-4" aria-labelledby="remove-project-heading">
        <div>
          <h2 id="remove-project-heading" class="font-semibold">Remove Project</h2>
          <p class="text-sm text-muted-foreground">Remove it from Emberloop. Its chats and files stay.</p>
        </div>
        <Button variant="destructive" class="w-fit" disabled={removing} onclick={remove}>
          <Trash2 />{removing ? 'Removing…' : 'Remove project'}
        </Button>
      </section>
    {/if}
  </div>
</main>
