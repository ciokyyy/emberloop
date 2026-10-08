<script>
  import { Plus, SquarePen, PanelRight, Menu, Settings } from '@lucide/svelte';
  import Button from './lib/ui/button.svelte';
  import Badge from './lib/ui/badge.svelte';
  import Sheet from './lib/ui/sheet.svelte';
  import Card from './lib/Card.svelte';
  import SessionList from './lib/SessionList.svelte';
  import Composer from './lib/Composer.svelte';
  import NewSessionDialog from './lib/NewSessionDialog.svelte';
  import SettingsPage from './lib/SettingsPage.svelte';
  import ContextMeter from './lib/ContextMeter.svelte';
  import DetailsPane from './lib/DetailsPane.svelte';
  import { modelLabel } from './lib/harnesses.js';
  import { groupTranscript, latestUsage } from './lib/events.js';
  import { listSessions, createSession, getSession, postPrompt, postSteer, postFollowup, getQueue, clearQueue, openStream } from './lib/api.js';

  let sessions = $state([]);
  let currentId = $state(null);
  let meta = $state(null);
  let events = $state([]);
  let lastSeq = $state(0);
  let connected = $state(true);
  let dialogOpen = $state(false);
  let view = $state('chat');
  let detailsOpen = $state(false);
  let menuOpen = $state(false);
  let creating = $state(false);
  let createError = $state('');
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

  let source = null;
  let retryTimer = null;
  let backoff = 1;

  const blocks = $derived(groupTranscript(events));
  const usage = $derived(latestUsage(events));
  const status = $derived(meta?.status ?? 'idle');
  let scroller = $state(null);

  $effect(() => {
    // Keep the newest card in view when the transcript grows.
    blocks.length;
    const el = scroller;
    if (el && el.scrollHeight - el.scrollTop - el.clientHeight < 200) {
      el.scrollTop = el.scrollHeight;
    }
  });

  async function refresh(autoOpen = false) {
    try {
      sessions = await listSessions();
      if (autoOpen && !currentId && sessions.length) openSession(sessions[0].id);
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
    view = 'chat';
    currentId = id;
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
      resumeNote = body.resumeNote ?? '';
      lastSeq = events.reduce((m, e) => Math.max(m, e.seq), 0);
      // Whole-log replay first, so reload shows the same transcript.
      subscribe(id);
      await refreshQueue(id);
    } catch {
      loadError = 'Could not load that Session.';
    }
  }

  function subscribe(id) {
    source = openStream(id, lastSeq, (e) => {
      if (e.seq <= lastSeq) return;
      lastSeq = e.seq;
      events = [...events, e];
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

  async function create(data) {
    creating = true;
    createError = '';
    try {
      const s = await createSession(data);
      dialogOpen = false;
      await refresh();
      openSession(s.id);
    } catch {
      createError = 'Could not create the Session.';
    } finally {
      creating = false;
    }
  }
  // Rejections propagate to the Composer, which keeps the draft.
  async function send(text, mode) {
    if (!currentId || sending) return;
    sending = true;
    try {
      if (mode === 'steer') await postSteer(currentId, text);
      else if (mode === 'followup') await postFollowup(currentId, text);
      else await postPrompt(currentId, text);
      await refreshQueue(currentId);
    } finally {
      sending = false;
    }
  }

  refresh(true);
</script>

<div class="flex h-dvh">
  <aside class="flex w-64 shrink-0 flex-col gap-3 border-r bg-sidebar p-3 max-md:hidden">
    <div class="flex items-center justify-between px-2 pt-1">
      <h1 class="text-sm font-semibold tracking-tight">Enderbloom</h1>
      <Button variant="ghost" size="icon-sm" onclick={() => { view = 'chat'; dialogOpen = true; }} aria-label="New Session"><SquarePen /></Button>
    </div>
    <Button variant="secondary" class="justify-start" onclick={() => { view = 'chat'; dialogOpen = true; }}><Plus />New Session</Button>
    <div class="min-h-0 flex-1 overflow-auto">
      <SessionList {sessions} {currentId} onSelect={openSession} />
    </div>
    <Button variant={view === 'settings' ? 'secondary' : 'ghost'} class="justify-start" onclick={() => (view = 'settings')}><Settings />Settings</Button>
  </aside>

  <div class="flex min-w-0 flex-1 flex-col">
    {#if view === 'settings'}
      <header class="flex items-center gap-3 px-4 py-2.5 md:hidden">
        <Button variant="ghost" size="icon-sm" onclick={() => (menuOpen = true)} aria-label="Sessions"><Menu /></Button>
      </header>
      <SettingsPage onBack={() => (view = 'chat')} />
    {:else}
    <header class="flex items-center gap-3 px-4 py-2.5 backdrop-blur-xl">
      <Button variant="ghost" size="icon-sm" class="md:hidden" onclick={() => (menuOpen = true)} aria-label="Sessions"><Menu /></Button>
      <span class="text-sm font-medium text-muted-foreground">{meta ? `${meta.harness} · ${modelLabel(meta.harness, meta.model)}` : 'No Session'}</span>
      {#if connected}
        <Badge variant="success"><span class="size-1.5 rounded-full bg-green-500"></span>Live</Badge>
      {:else}
        <Badge variant="warning" data-testid="reconnect"><span class="size-1.5 animate-pulse rounded-full bg-amber-500"></span>Reconnecting…</Badge>
      {/if}
      <div class="ml-auto flex gap-2">
        <Button variant="ghost" size="sm" class="md:hidden" onclick={() => { view = 'chat'; dialogOpen = true; }}><Plus />New</Button>
        <Button variant="outline" size="sm" onclick={() => (detailsOpen = !detailsOpen)} aria-pressed={detailsOpen}>
          <PanelRight />Details
        </Button>
      </div>
    </header>

    {#if loadError}
      <p role="alert" class="mx-4 rounded-xl bg-destructive/10 px-4 py-2 text-sm text-destructive">{loadError}</p>
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
        <div bind:this={scroller} class="flex flex-1 flex-col gap-3 overflow-auto px-4 pt-4 pb-44">
          <div class="mx-auto flex w-full max-w-3xl flex-col gap-4">
            {#each blocks as b (b.key)}
              <Card block={b} />
            {:else}
              <p class="py-24 text-center text-2xl font-semibold tracking-tight text-muted-foreground">
                {currentId ? 'Send a prompt to start.' : 'Select a Session or create one.'}
              </p>
            {/each}
          </div>
        </div>
        <div class="pointer-events-none absolute inset-x-0 bottom-0 bg-gradient-to-t from-background via-background/90 to-transparent px-4 pb-5 pt-10">
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
            {#if currentId && resumable && !meta?.scout}
              <Composer {status} busy={sending} onSend={send} />
              <ContextMeter model={modelLabel(meta?.harness, meta?.model ?? '')} {usage} />
            {/if}
          </div>
        </div>
      </main>

      <div class="p-3 pl-0">
        <DetailsPane session={meta} eventCount={events.length} open={detailsOpen} onToggle={() => (detailsOpen = false)} />
      </div>
    </div>
    {/if}
  </div>
</div>

<Sheet bind:open={menuOpen} title="Sessions">
  <div class="flex items-center justify-between px-2 pt-1">
    <span class="text-sm font-semibold tracking-tight">Enderbloom</span>
  </div>
  <Button variant="secondary" class="justify-start" onclick={() => { menuOpen = false; dialogOpen = true; }}><Plus />New Session</Button>
  <div class="min-h-0 flex-1 overflow-auto">
    <SessionList {sessions} {currentId} onSelect={(id) => { menuOpen = false; openSession(id); }} />
  </div>
  <Button variant="ghost" class="justify-start" onclick={() => { menuOpen = false; view = 'settings'; }}><Settings />Settings</Button>
</Sheet>

<NewSessionDialog open={dialogOpen} busy={creating} error={createError} onClose={() => (dialogOpen = false)} onCreate={create} />
