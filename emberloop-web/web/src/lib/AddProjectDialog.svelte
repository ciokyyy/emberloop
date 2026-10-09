<!-- Add-project dialog: a folder path becomes a recorded Project. -->
<script>
  import Button from './ui/button.svelte';
  import Dialog from './ui/dialog.svelte';
  import FloatingInput from './ui/floating-input.svelte';

  let { open = false, busy = false, error = '', onClose = () => {}, onAdd = () => {} } = $props();
  let path = $state('');
  let name = $state('');

  $effect(() => {
    if (open) {
      path = '';
      name = '';
    }
  });

  function submit(ev) {
    ev.preventDefault();
    if (!path.trim()) return;
    onAdd({ path: path.trim(), name: name.trim() });
  }
</script>

<Dialog {open} onOpenChange={(v) => !v && onClose()} title="Add project" description="Chats started in a project run in its folder.">
  <form onsubmit={submit} class="flex flex-col gap-4">
    <FloatingInput id="project-path" label="Folder" bind:value={path} placeholder="~/Projects/my-app" inputClass="font-mono" autofocus />
    <FloatingInput id="project-name" label="Name (optional)" bind:value={name} placeholder="folder name" />
    {#if error}
      <p role="alert" class="text-sm text-destructive">{error}</p>
    {/if}
    <div class="mt-1 flex justify-end gap-2">
      <Button type="button" variant="ghost" onclick={onClose}>Cancel</Button>
      <Button type="submit" disabled={busy || !path.trim()}>{busy ? 'Adding…' : 'Add project'}</Button>
    </div>
  </form>
</Dialog>
