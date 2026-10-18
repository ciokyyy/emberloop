<!-- Phone chat menu: the header's one menu button. One bottom sheet holds the
     Session actions (New Session, Git), the model and context row and the
     Project row; the Project row switches the same sheet to the scope list, so
     no sheet ever stacks on another. Desktop keeps the inline header icons. -->
<script>
  import { ArrowLeft, EllipsisVertical, Plus } from '@lucide/svelte';
  import IconButton from './ui/icon-button.svelte';
  import BottomSheet from './ui/bottom-sheet.svelte';
  import Row from './ui/row.svelte';
  import ProjectScopeList from './ProjectScopeList.svelte';
  import { icons } from './icons.js';
  import { carryBack } from './overlayBack.js';

  let {
    model = '',
    context = '',
    projectName = '',
    projects = [],
    selectedId = null,
    git = null,
    onNewSession = () => {},
    onOpenGit = null,
    onOpenContext = null,
    onSelectScope = () => {},
  } = $props();

  let open = $state(false);
  let view = $state('session');
  const gitMeta = $derived(
    git?.isRepo ? `${git.branch}${git.files?.length ? ` · ${git.files.length} changed` : ' · clean'}` : 'Not a repository',
  );

  function show() {
    view = 'session';
    open = true;
  }
  // Git and the context sheet take over the back entry. New Session closes the
  // sheet itself instead of relying on the navigation newChat happens to do; a
  // Project pick navigates, which closes it through the router.
  function handOver(run) {
    carryBack();
    open = false;
    run?.();
  }
  function startNewSession() {
    open = false;
    onNewSession();
  }
</script>

<IconButton icon={EllipsisVertical} label="Session menu" aria-haspopup="dialog" aria-expanded={open} class="md:hidden" onclick={show} data-testid="chat-menu" />

<BottomSheet bind:open title={view === 'project' ? 'Project' : 'Session'} description={view === 'project' ? 'Switch the Project scope.' : 'Session actions, model and Project.'}>
  {#if view === 'project'}
    <div class="flex items-center gap-1">
      <IconButton icon={ArrowLeft} label="Back to Session" onclick={() => (view = 'session')} />
      <p class="min-w-0 flex-1 truncate text-caption text-fg-3">Switch Project scope</p>
    </div>
    <ProjectScopeList {projects} {selectedId} onSelect={(value) => onSelectScope(value)} />
  {:else}
    <div class="-mx-1 flex flex-col gap-0.5">
      <Row icon={Plus} title="New Session" meta="Start another chat" onclick={startNewSession} />
      {#if onOpenGit}
        <Row icon={icons.git} title="Git" meta={gitMeta} onclick={() => handOver(onOpenGit)} />
      {/if}
      {#if onOpenContext}
        <Row icon={icons.model} title={model || 'Unknown model'} meta={context} onclick={() => handOver(onOpenContext)} />
      {/if}
      <Row icon={icons.project} title={projectName || 'No project'} meta="Switch Project" onclick={() => (view = 'project')} />
    </div>
  {/if}
</BottomSheet>
