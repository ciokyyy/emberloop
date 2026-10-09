<script>
  import { Plus, SquarePen, Menu, Settings, Layers, FolderPlus, GitBranch } from '@lucide/svelte';
  import Button from './lib/ui/button.svelte';
  import Badge from './lib/ui/badge.svelte';
  import Sheet from './lib/ui/sheet.svelte';
  import Card from './lib/Card.svelte';
  import ProjectList from './lib/ProjectList.svelte';
  import AddProjectDialog from './lib/AddProjectDialog.svelte';
  import DraftBar from './lib/DraftBar.svelte';
  import GitPanel from './lib/GitPanel.svelte';
  import CompressDialog from './lib/CompressDialog.svelte';
  import Composer from './lib/Composer.svelte';
  import SettingsPage from './lib/SettingsPage.svelte';
  import RunsView from './lib/RunsView.svelte';
  import ContextMeter from './lib/ContextMeter.svelte';
  import { modelLabel } from './lib/harnesses.js';
  import { groupTranscript, latestUsage, withImageNote } from './lib/events.js';
  import { watch } from './lib/live.js';
  import { listSessions, createSession, getSession, postPrompt, postSteer, postFollowup, getQueue, clearQueue, interruptSession, openStream, compressSession, listProjects, addProject, renameProject, removeProject, getGit } from './lib/api.js';

  let sessions = $state([]);
  let currentId = $state(null);
  let meta = $state(null);
  let events = $state([]);
  let lastSeq = $state(0);
  let connected = $state(true);
  let projects = $state([]);
  let draftProjectId = $state(null);
  let addOpen = $state(false);
  let adding = $state(false);
  let addError = $state('');
  let gitOpen = $state(false);
  let compressOpen = $state(false);
  let compressBusy = $state(false);
  let compressError = $state('');
  // Only a Backend that compacts in place compresses (the SDK pig Backend);
  // the Daemon reports it per Session. Claude and pig over ACP have no hook.
  let compressible = $state(false);
  const canCompress = $derived(compressible && !meta?.scout && resumable);
  const compressing = $derived(
    events.findLast((e) => e.kind === 'compaction_started' || e.kind === 'compaction')?.kind === 'compaction_started',
  );
  async function runCompress(instructions) {
    compressBusy = true;
    compressError = '';
    try {
      await compressSession(currentId, instructions);
      compressOpen = false;
    } catch (err) {
      compressError = err.message;
    } finally {
      compressBusy = false;
    }
  }
  let git = $state(null);
  const PREF_KEY = 'eb.pref';
  const pref = JSON.parse(localStorage.getItem(PREF_KEY) ?? '{}');
  let draftHarness = $state(pref.harness ?? 'claude');
  let draftModel = $state(pref.model ?? 'sonnet');
  $effect(() => {
    localStorage.setItem(PREF_KEY, JSON.stringify({ harness: draftHarness, model: draftModel }));
  });
  const activeProjectId = $derived(currentId ? (meta?.projectId ?? null) : draftProjectId);
  const activeProject = $derived(projects.find((p) => p.id === activeProjectId) ?? null);
  const drafting = $derived(!currentId && !!draftProjectId);
  let view = $state('chat');
  let menuOpen = $state(false);
  let sending = $state(false);
  let loadError = $state('');
  let resumable = $state(true);
  let resumeNote = $state('');
  let queue = $state([]);

  const queueLabel = (m) => (m.mode === 'steer' ? 'Steer' : m.mode === 'followup' ? 'Follow-up' : 'Prompt');

  async function refreshQueue(id) {
    if (!id) return;
    try {
      const body = await getQueue(id);
      if (id === currentId) queue = body.queue ?? [];
    } catch {
      // Queue is best-effort: the transcript stays the source of truth.
    }
  }

  async function clear(id) {
    try {
      await clearQueue(id);
    } finally {
      await refreshQueue(id);
    }
  }

  let stopSessionWatch = () => {};
  let source = null;
  let retryTimer = null;
  let backoff = 1;

  // Messages the user just sent that the Daemon has not echoed yet. They show
  // in the transcript at once, then give way to the real prompt Event.
  let pendingSends = $state([]);
  let pendingKey = 0;
  const blocks = $derived([
    ...groupTranscript(events),
    ...pendingSends.map((p) => ({
      key: `pending-${p.key}`,
      card: 'user',
      title: p.mode === 'steer' ? 'Steer' : p.mode === 'followup' ? 'Follow-up' : 'You',
      body: withImageNote(p.text, p.images),
      pending: p.state,
      events: [],
    })),
  ]);
  const usage = $derived(latestUsage(events));
  const status = $derived(meta?.status ?? 'idle');
  let scroller = $state(null);
  // Height of the floating composer stack: the transcript pads by it so the last block never hides.
  let overlayH = $state(176);

  let stickToBottom = true;
  function onScroll() {
    const el = scroller;
    if (el) stickToBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 160;
  }
  $effect(() => {
    // Follow the transcript while the user is at the bottom, including while
    // the last card grows (streaming text adds no new card).
    events.length;
    pendingSends.length;
    overlayH;
    const last = events[events.length - 1];
    last?.text;
    const el = scroller;
    if (!el || !stickToBottom) return;
    requestAnimationFrame(() => {
      el.scrollTop = el.scrollHeight;
    });
  });

  async function loadProjects() {
    try {
      projects = await listProjects();
    } catch {
      // The Session list reports an unreachable Daemon.
    }
  }

  async function loadGit() {
    const id = activeProjectId;
    if (!id) {
      git = null;
      return;
    }
    try {
      const g = await getGit(id);
      if (id === activeProjectId) git = g;
    } catch {
      git = null;
    }
  }
  $effect(() => {
    // Follow the active Project: refetch on switch, on every turn edge, and
    // on a slow timer while the panel is open.
    activeProjectId;
    status;
    git = null;
    loadGit();
    if (!gitOpen) return;
    const t = setInterval(loadGit, 8000);
    return () => clearInterval(t);
  });

  async function add(data) {
    adding = true;
    addError = '';
    try {
      const p = await addProject(data);
      await loadProjects();
      addOpen = false;
      startDraft(p.id);
    } catch (err) {
      addError = err.message || 'Could not add the project.';
    } finally {
      adding = false;
    }
  }
  async function rename(p) {
    const name = window.prompt('Rename project', p.name);
    if (name && name.trim() && name !== p.name) {
      await renameProject(p.id, name.trim());
      await loadProjects();
    }
  }
  async function remove(p) {
    if (!window.confirm(`Remove "${p.name}" from the list? Its chats and files stay.`)) return;
    await removeProject(p.id);
    if (draftProjectId === p.id) draftProjectId = null;
    await loadProjects();
  }

  // A draft chat: the Project is chosen, the Session starts on first send.
  function startDraft(projectId) {
    closeStream();
    stopSessionWatch();
    batch = [];
    pendingSends = [];
    queue = [];
    view = 'chat';
    menuOpen = false;
    currentId = null;
    meta = null;
    events = [];
    lastSeq = 0;
    resumable = true;
    resumeNote = '';
    draftProjectId = projectId;
  }
  function newChat() {
    const id = activeProjectId ?? projects[0]?.id ?? null;
    if (id) startDraft(id);
    else addOpen = true;
  }

  async function refresh(autoOpen = false) {
    try {
      sessions = await listSessions();
      if (autoOpen && !currentId && !draftProjectId && sessions.length) openSession(sessions[0].id);
    } catch {
      loadError = 'Daemon unreachable. Start it and reload.';
    }
  }

  function closeStream() {
    if (retryTimer) clearTimeout(retryTimer);
    retryTimer = null;
    if (source) source.close();
    source = null;
  }
  async function openSession(id) {
    closeStream();
    batch = [];
    pendingSends = [];
    view = 'chat';
    currentId = id;
    draftProjectId = null;
    events = [];
    lastSeq = 0;
    connected = true;
    backoff = 1;
    resumable = true;
    resumeNote = '';
    try {
      const body = await getSession(id);
      meta = body.session;
      events = body.events ?? [];
      resumable = body.resumable ?? true;
      compressible = body.compressible ?? false;
      resumeNote = body.resumeNote ?? '';
      lastSeq = events.reduce((m, e) => Math.max(m, e.seq), 0);
      // Whole-log replay first, so reload shows the same transcript.
      subscribe(id);
      stopSessionWatch();
      stopSessionWatch = watch(`session:${id}`, () => refreshQueue(id));
      await refreshQueue(id);
    } catch {
      loadError = 'Could not load that Session.';
    }
  }

  // Events arrive per delta; applying each one re-groups the whole
  // transcript. Collect them and apply once per frame.
  let batch = [];
  let batchFrame = 0;
  function flushBatch() {
    batchFrame = 0;
    if (!batch.length) return;
    const fresh = batch;
    batch = [];
    events = [...events, ...fresh];
  }

  function subscribe(id) {
    source = openStream(id, lastSeq, (e) => {
      if (e.seq <= lastSeq) return;
      lastSeq = e.seq;
      batch.push(e);
      if (!batchFrame) batchFrame = requestAnimationFrame(flushBatch);
      if (e.kind === 'prompt') pendingSends = dropEchoed(pendingSends, e);
      if (e.kind === 'status' && e.status && id === currentId) {
        meta = { ...meta, status: e.status };
        sessions = sessions.map((s) => (s.id === id ? { ...s, status: e.status } : s));
        if (e.status === 'ended') {
          resumable = false;
          resumeNote = 'session ended; history is read-only';
        }
        // A turn edge may have delivered or exposed queued messages.
        refreshQueue(id);
      }
      if (e.kind === 'prompt' && id === currentId) refreshQueue(id);
    });
    source.onopen = () => {
      connected = true;
      backoff = 1;
    };
    source.onerror = () => {
      // Dropped connection: mark it, back off, and resubscribe from
      // lastSeq so nothing is missed.
      connected = false;
      if (source) source.close();
      source = null;
      const delay = Math.min(backoff * 1000, 10000);
      backoff += 1;
      retryTimer = setTimeout(() => {
        if (currentId === id) subscribe(id);
      }, delay);
    };
  }

  // The echoed prompt Event replaces its pending card: first match by text.
  // Messages queued behind a turn are echoed as one merged Event, which
  // replaces every pending card whose text it holds.
  function dropEchoed(list, e) {
    const sent = String(e.text ?? '');
    const i = list.findIndex((p) => sent === p.text);
    if (i !== -1) return list.filter((_, j) => j !== i);
    const merged = list.filter((p) => p.text && sent.includes(p.text));
    return merged.length ? list.filter((p) => !merged.includes(p)) : list;
  }

  // The message shows at once; a rejection removes it and propagates so the
  // Composer restores the draft.
  async function send(text, mode, images = []) {
    if (drafting) {
      if (sending) return;
      sending = true;
      try {
        const s = await createSession({ harness: draftHarness, model: draftModel, projectId: draftProjectId });
        await refresh();
        await openSession(s.id);
      } catch (err) {
        loadError = 'Could not start the chat.';
        throw err;
      } finally {
        sending = false;
      }
      mode = 'prompt';
    }
    if (!currentId || sending) return;
    sending = true;
    const key = ++pendingKey;
    pendingSends = [...pendingSends, { key, text, mode, images: images.length, state: 'sending' }];
    try {
      let res;
      // Only the prompt route carries images; it takes the mode too.
      if (images.length) res = await postPrompt(currentId, text, { mode, images });
      else if (mode === 'steer') res = await postSteer(currentId, text);
      else if (mode === 'followup') res = await postFollowup(currentId, text);
      else res = await postPrompt(currentId, text);
      // A queued message shows in the queue panel until it is delivered;
      // an accepted one waits here for its prompt Event.
      if (res?.queued && !res?.seq) pendingSends = pendingSends.filter((p) => p.key !== key);
      else pendingSends = pendingSends.map((p) => (p.key === key ? { ...p, state: 'sent' } : p));
      await refreshQueue(currentId);
    } catch (err) {
      pendingSends = pendingSends.filter((p) => p.key !== key);
      throw err;
    } finally {
      sending = false;
    }
  }

  // The sidebar follows the Daemon: new, spawned, finished and ended Sessions
  // appear without a reload.
  watch('sessions', () => refresh(!currentId));
  loadProjects();
</script>

<div class="flex h-dvh">
  <aside class="flex w-64 shrink-0 flex-col gap-3 border-r bg-sidebar p-3 max-md:hidden">
    <div class="flex items-center justify-between px-2 pt-1">
      <h1 class="text-sm font-semibold tracking-tight">Enderbloom</h1>
      <Button variant="ghost" size="icon-sm" onclick={newChat} aria-label="New Session"><SquarePen /></Button>
    </div>
    <Button variant="secondary" class="justify-start" onclick={newChat}><Plus />New chat</Button>
    <div class="flex items-center justify-between px-2">
      <span class="text-xs font-medium text-muted-foreground">Projects</span>
      <Button variant="ghost" size="icon-sm" class="size-6" onclick={() => (addOpen = true)} aria-label="Add project"><FolderPlus class="size-3.5" /></Button>
    </div>
    <div class="min-h-0 flex-1 overflow-auto">
      <ProjectList {projects} {sessions} {currentId} {activeProjectId} onSelect={openSession} onNew={startDraft} onRename={rename} onRemove={remove} />
    </div>
    <Button variant={view === 'runs' ? 'secondary' : 'ghost'} class="justify-start" onclick={() => (view = 'runs')}><Layers />Runs</Button>
    <Button variant={view === 'settings' ? 'secondary' : 'ghost'} class="justify-start" onclick={() => (view = 'settings')}><Settings />Settings</Button>
  </aside>
  <div class="flex min-w-0 flex-1 flex-col">
    {#if view === 'settings'}
      <header class="flex items-center gap-3 px-4 py-2.5 md:hidden">
        <Button variant="ghost" size="icon-sm" onclick={() => (menuOpen = true)} aria-label="Sessions"><Menu /></Button>
      </header>
      <SettingsPage onBack={() => (view = 'chat')} />
    {:else if view === 'runs'}
      <header class="flex items-center gap-3 px-4 py-2.5 md:hidden">
        <Button variant="ghost" size="icon-sm" onclick={() => (menuOpen = true)} aria-label="Sessions"><Menu /></Button>
      </header>
      <RunsView workDir={meta?.workDir ?? ''} onOpenSession={openSession} />
    {:else}
    <header class="flex items-center gap-2 px-3 py-2.5 backdrop-blur-xl sm:gap-3 sm:px-4">
      <Button variant="ghost" size="icon-sm" class="md:hidden" onclick={() => (menuOpen = true)} aria-label="Sessions"><Menu /></Button>
      <div class="flex min-w-0 flex-col leading-tight" data-testid="header-title">
        <span class="truncate text-sm font-semibold">{meta?.title || activeProject?.name || (drafting ? 'New chat' : 'No Session')}</span>
        <span class="truncate text-xs text-muted-foreground">{[meta?.title ? activeProject?.name : '', meta ? `${meta.harness} · ${modelLabel(meta.harness, meta.model)}` : drafting ? `${draftHarness} · ${modelLabel(draftHarness, draftModel)}` : ''].filter(Boolean).join(' · ')}</span>
      </div>
      {#if connected}
        <span class="size-2 shrink-0 rounded-full bg-green-500" title="Live" role="status" aria-label="Live" data-testid="live"></span>
      {:else}
        <Badge variant="warning" class="shrink-0" data-testid="reconnect"><span class="size-1.5 animate-pulse rounded-full bg-amber-500"></span>Reconnecting…</Badge>
      {/if}
      <div class="ml-auto flex shrink-0 gap-2">
        {#if currentId && usage}
          <span class="flex items-center self-center text-xs tabular-nums text-muted-foreground max-sm:hidden" title="{usage.used} of {usage.size} tokens" data-testid="header-context">{Math.min(100, Math.round((usage.used / usage.size) * 100))}% ctx</span>
        {/if}
        <Button variant="ghost" size="sm" class="md:hidden" onclick={newChat}><Plus />New</Button>
        {#if activeProject}
          <Button variant="outline" size="sm" onclick={() => (gitOpen = !gitOpen)} aria-pressed={gitOpen} aria-label="Git">
            <GitBranch /><span class="max-w-24 truncate max-sm:hidden">{git?.isRepo ? git.branch : 'Git'}</span>{#if git?.isRepo && git.files.length}<Badge class="px-1.5 py-0 text-[10px]">{git.files.length}</Badge>{/if}
          </Button>
        {/if}
      </div>
    </header>

    {#if loadError}
      <p role="alert" class="mx-3 rounded-xl bg-destructive/10 px-4 py-2 text-sm text-destructive sm:mx-4">{loadError}</p>
    {/if}

    <div class="flex min-h-0 flex-1">
      <main class="relative flex min-w-0 flex-1 flex-col">
        {#if currentId && !resumable}
          <p data-testid="read-only" class="mx-4 rounded-xl bg-amber-500/10 px-4 py-2 text-sm text-amber-300">
            Read-only history. {resumeNote}
          </p>
        {:else if currentId && meta?.scout}
          <p data-testid="scout-read-only" class="mx-4 rounded-xl bg-amber-500/10 px-4 py-2 text-sm text-amber-300">
            Scout session. Read-only transcript.
          </p>
        {/if}
        <div bind:this={scroller} onscroll={onScroll} class="flex flex-1 flex-col gap-3 overflow-auto px-3 pt-4 sm:px-4" style="padding-bottom: {overlayH + 16}px">
          <div class="mx-auto flex w-full max-w-3xl flex-col gap-4">
            {#each blocks as b (b.key)}
              <Card block={b} sessionId={currentId} />
            {:else}
              <p class="py-24 text-center text-2xl font-semibold tracking-tight text-muted-foreground">
                {currentId ? 'Send a prompt to start.' : drafting ? `What should we do in ${activeProject?.name ?? 'this project'}?` : projects.length ? 'Pick a project, then start a chat.' : 'Add a project to begin.'}
              </p>
            {/each}
          </div>
        </div>
        <div bind:clientHeight={overlayH} class="pointer-events-none absolute inset-x-0 bottom-0 bg-gradient-to-t from-background via-background/90 to-transparent px-3 pb-[max(1.25rem,env(safe-area-inset-bottom))] pt-10 sm:px-4">
          <div class="pointer-events-auto mx-auto flex w-full max-w-3xl flex-col gap-2">
            {#if currentId && queue.length}
              <div class="animate-rise rounded-2xl border bg-card p-2" data-testid="queue-panel">
                <div class="flex items-center gap-2 px-2 pt-1">
                  <span class="text-xs font-medium text-muted-foreground">Queued ({queue.length})</span>
                  <Button variant="ghost" size="sm" class="ml-auto h-6 px-2" onclick={() => clear(currentId)}>Clear</Button>
                </div>
                <ul class="flex flex-col gap-1 p-1">
                  {#each queue as m, i (i)}
                    <li class="flex items-baseline gap-2 px-2 text-sm">
                      <Badge class="shrink-0">{queueLabel(m)}</Badge>
                      <span class="min-w-0 flex-1 truncate">{m.text}</span>
                    </li>
                  {/each}
                </ul>
              </div>
            {/if}
            {#if drafting}
              <DraftBar bind:harness={draftHarness} bind:model={draftModel} />
              <Composer status="idle" busy={sending} onSend={send} />
            {:else if currentId && resumable && !meta?.scout}
              <Composer {status} busy={sending} onSend={send} onStop={() => interruptSession(currentId)} />
              <ContextMeter model={modelLabel(meta?.harness, meta?.model ?? '')} {usage} onCompress={canCompress ? () => { compressError = ''; compressOpen = true; } : null} busy={status === 'working'} {compressing} />
            {/if}
          </div>
        </div>
      </main>

      <div class="flex min-h-0 p-3 pl-0 max-lg:contents">
        {#if gitOpen && activeProject}
          <GitPanel projectId={activeProject.id} {git} onClose={() => (gitOpen = false)} onRefresh={loadGit} />
        {/if}
      </div>
    </div>
    {/if}
  </div>
</div>

<Sheet bind:open={menuOpen} title="Sessions">
  <div class="flex items-center justify-between px-2 pt-1">
    <span class="text-sm font-semibold tracking-tight">Enderbloom</span>
  </div>
  <Button variant="secondary" class="justify-start" onclick={() => { menuOpen = false; newChat(); }}><Plus />New chat</Button>
  <div class="flex items-center justify-between px-2">
    <span class="text-xs font-medium text-muted-foreground">Projects</span>
    <Button variant="ghost" size="icon-sm" class="size-6" onclick={() => { menuOpen = false; addOpen = true; }} aria-label="Add project"><FolderPlus class="size-3.5" /></Button>
  </div>
  <div class="min-h-0 flex-1 overflow-auto">
    <ProjectList {projects} {sessions} {currentId} {activeProjectId} onSelect={(id) => { menuOpen = false; openSession(id); }} onNew={startDraft} onRename={rename} onRemove={remove} />
  </div>
  <Button variant="ghost" class="justify-start" onclick={() => { menuOpen = false; view = 'runs'; }}><Layers />Runs</Button>
  <Button variant="ghost" class="justify-start" onclick={() => { menuOpen = false; view = 'settings'; }}><Settings />Settings</Button>
</Sheet>
<CompressDialog open={compressOpen} busy={compressBusy} error={compressError} {usage} onClose={() => (compressOpen = false)} onCompress={runCompress} />
<AddProjectDialog open={addOpen} busy={adding} error={addError} onClose={() => (addOpen = false)} onAdd={add} />
