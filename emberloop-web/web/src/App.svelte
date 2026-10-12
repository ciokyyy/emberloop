<script>
  import { onDestroy } from 'svelte';
  import { Plus, SquarePen, Menu, MessageSquare, Settings, Layers, Gauge, Brain, FolderPlus, GitBranch, Search, LogOut } from '@lucide/svelte';
  import Button from './lib/ui/button.svelte';
  import Input from './lib/ui/input.svelte';
  import Badge from './lib/ui/badge.svelte';
  import Sheet from './lib/ui/sheet.svelte';
  import Transcript from './lib/Transcript.svelte';
  import OwnerGroups from './lib/OwnerGroups.svelte';
  import AddProjectDialog from './lib/AddProjectDialog.svelte';
  import DraftBar from './lib/DraftBar.svelte';
  import GitPanel from './lib/GitPanel.svelte';
  import CompressDialog from './lib/CompressDialog.svelte';
  import CompressBanner from './lib/CompressBanner.svelte';
  import Composer from './lib/Composer.svelte';
  import SettingsPage from './lib/SettingsPage.svelte';
  import RunsView from './lib/RunsView.svelte';
  import QuotaPage from './lib/QuotaPage.svelte';
  import MemoryPage from './lib/MemoryPage.svelte';
  import ContextMeter from './lib/ContextMeter.svelte';
  import VersionBadge from './lib/VersionBadge.svelte';
  import Rail from './lib/Rail.svelte';
  import RunsSidebar from './lib/RunsSidebar.svelte';
  import { modelLabel } from './lib/harnesses.js';
  import { groupTranscript, latestUsage, withImageNote, compressBlockReason } from './lib/events.js';
  import { parsePath, buildPath } from './lib/router.js';
  import { watch, stopLive } from './lib/live.js';
  import { soundPlayer } from './lib/sound.js';
  import { readJSON, writeJSON } from './lib/sidebar.js';
  import { listSessions, createSession, uploadAudio, getSession, postPrompt, postSteer, postFollowup, getQueue, clearQueue, interruptSession, openStream, compressSession, listProjects, addProject, renameProject, removeProject, getGit, listRuns } from './lib/api.js';
  import { people, refreshPeople, personFor } from './lib/people.svelte.js';

  // The signed-in User and the sign-out action, both supplied by Root.svelte.
  let { user = null, onSignOut = () => {} } = $props();

  // Interface sounds. Created on mount so the first-gesture unlock listens
  // from the start; every cue after that is one playEvent call. Message
  // sends and subagent finishes are silent by design (no hook here).
  const sounds = soundPlayer();

  let sessions = $state([]);
  let currentId = $state(null);
  let meta = $state(null);
  let events = $state([]);
  let lastSeq = $state(0);
  let connected = $state(true);
  let projects = $state([]);
  let runs = $state([]);
  let draftProjectId = $state(null);
  let addOpen = $state(false);
  let adding = $state(false);
  let addError = $state('');
  let gitOpen = $state(false);
  let compressOpen = $state(false);
  let compressBusy = $state(false);
  let compressError = $state('');
  // A Backend that can compress sets compressible: in place (SDK pig) or by Handoff.
  // compressNote gives the reason Compress is off, when it is off.
  let compressible = $state(false);
  let compressNote = $state('');
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
  // A dismissed block clears on the Session its banner was shown for.
  function clearCompressBlock(id) {
    if (currentId === id && meta) meta = { ...meta, compressBlock: '' };
  }
  let git = $state(null);
  const PREF_KEY = 'eb.pref';
  const pref = JSON.parse(localStorage.getItem(PREF_KEY) ?? '{}');
  let draftHarness = $state(pref.harness ?? 'claude');
  let draftModel = $state(pref.model ?? 'sonnet');
  let draftEffort = $state(pref.effort ?? 'default');
  $effect(() => {
    localStorage.setItem(PREF_KEY, JSON.stringify({ harness: draftHarness, model: draftModel, effort: draftEffort }));
  });
  const activeProjectId = $derived(currentId ? (meta?.projectId ?? null) : draftProjectId);
  const activeProject = $derived(projects.find((p) => p.id === activeProjectId) ?? null);
  const drafting = $derived(!currentId && !!draftProjectId);
  const creatorName = $derived(meta?.owner ? (personFor(meta.owner)?.username ?? meta.owner.slice(0, 8)) : '');
  let view = $state('chat');
  // Rail 2 for Settings: jump to a section by its heading.
  let settingsSections = $state([]);
  $effect(() => {
    if (view !== 'settings') return;
    const t = setTimeout(() => {
      settingsSections = [...document.querySelectorAll('[data-testid="settings-page"] h2')].map((h) => h.textContent.trim());
    }, 150);
    return () => clearTimeout(t);
  });
  function jumpSetting(title) {
    menuOpen = false;
    const h = [...document.querySelectorAll('[data-testid="settings-page"] h2')].find((x) => x.textContent.trim() === title);
    h?.scrollIntoView({ behavior: 'smooth', block: 'start' });
  }
  let menuOpen = $state(false);
  // Rail 2 (the contextual sidebar) collapses to the icon rail on desktop
  // and persists across reloads; on phones it is an off-canvas drawer.
  const SIDEBAR_KEY = 'eb.sidebar-collapsed';
  let sideCollapsed = $state(readJSON(SIDEBAR_KEY, false));
  function toggleSidebar() {
    sideCollapsed = !sideCollapsed;
    writeJSON(SIDEBAR_KEY, sideCollapsed);
  }
  // Chat filter text, shared by the desktop sidebar and the drawer.
  let chatQuery = $state('');
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
  // The echo swap must not replay the entrance animation: the pending card
  // already rose in, so the real prompt card mounts with noAnim for one
  // glide cycle instead of remounting with animate-rise (same visual key,
  // no snap). Texts leave the set after 2.5s so later identical prompts
  // still animate.
  let pendingSends = $state([]);
  let pendingKey = 0;
  let swappedTexts = $state([]);
  function noteSwapped(texts) {
    const fresh = texts.filter((t) => t && !swappedTexts.includes(t));
    if (!fresh.length) return;
    swappedTexts = [...swappedTexts, ...fresh];
    setTimeout(() => {
      swappedTexts = swappedTexts.filter((t) => !fresh.includes(t));
    }, 2500);
  }
  const sessionsById = $derived(Object.fromEntries(sessions.map((s) => [s.id, s])));
  const blocks = $derived([
    ...groupTranscript(events, sessionsById, { working: meta?.status === 'working' }).map((b) =>
      b.card === 'user' && swappedTexts.some((t) => b.body === t || String(b.body ?? '').startsWith(`${t}\n`))
        ? { ...b, noAnim: true }
        : b,
    ),
    ...pendingSends.map((p) => ({
      key: `pending-${p.key}`,
      card: 'user',
      title: p.mode === 'steer' ? 'Steer' : p.mode === 'followup' ? 'Follow-up' : 'You',
      body: withImageNote(p.text, p.images),
      imageIds: p.imageIds ?? [],
      pending: p.state,
      events: [],
    })),
  ]);
  const usage = $derived(latestUsage(events));
  const status = $derived(meta?.status ?? 'idle');
  // Height of the floating composer stack: the transcript pads by it so the last block never hides.
  let overlayH = $state(176);

  // Routes: /s/<id>, /runs[/<runId>[/<ticketId>]], /settings, /quota, /memory, / (last viewed).
  const TAIL_TURNS = 6;
  const LAST_KEY = 'eb.last';
  let hasMore = $state(false);
  let firstSeq = $state(0);
  let loadingEarlier = $state(false);
  let sessionMissing = $state(false);
  let runId = $state(null);
  let ticketId = $state(null);
  let atSeq = $state(0);
  let openToken = 0;

  const goSession = (id) => navigate(buildPath({ name: 'session', id }));
  const openRow = (row) => (row.kind === 'run' ? navigate(buildPath({ name: 'run', runId: row.id })) : goSession(row.id));
  function navigate(path, { replace = false } = {}) {
    // Any navigation closes the phone drawer (backdrop taps close it
    // through the dialog itself).
    menuOpen = false;
    if (replace) history.replaceState(null, '', path);
    else if (window.location.pathname + window.location.search !== path) history.pushState(null, '', path);
    return applyRoute(path);
  }
  async function applyRoute(path) {
    const r = parsePath(path);
    if (r.name === 'unknown') {
      history.replaceState(null, '', '/');
      return applyRoute('/');
    }
    if (r.name === 'settings') {
      view = 'settings';
      menuOpen = false;
      return;
    }
    if (r.name === 'quota') {
      view = 'quota';
      menuOpen = false;
      return;
    }
    if (r.name === 'memory') {
      view = 'memory';
      menuOpen = false;
      return;
    }
    if (r.name === 'runs' || r.name === 'run' || r.name === 'ticket') {
      view = 'runs';
      menuOpen = false;
      runId = r.runId ?? null;
      ticketId = r.ticketId ?? null;
      return;
    }
    if (r.name === 'session') {
      if (r.id !== currentId) return openSession(r.id, { at: r.at ?? 0 });
      if (r.at) atSeq = r.at;
      view = 'chat';
      return;
    }
    // Home: a draft stays, otherwise the last viewed Session else newest.
    if (draftProjectId) {
      view = 'chat';
      return;
    }
    let id = null;
    try {
      id = localStorage.getItem(LAST_KEY);
    } catch {
      // Private mode: fall through to newest.
    }
    if (!id || !sessions.some((s) => s.id === id)) id = sessions[0]?.id ?? null;
    if (id) {
      history.replaceState(null, '', buildPath({ name: 'session', id }));
      return openSession(id);
    }
    view = 'chat';
  }

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
    sessionMissing = false;
    hasMore = false;
    firstSeq = 0;
    atSeq = 0;
    loadingEarlier = false;
    draftProjectId = projectId;
  }
  function newChat() {
    const id = activeProjectId ?? projects[0]?.id ?? null;
    if (id) {
      startDraft(id);
      history.pushState(null, '', '/');
    } else addOpen = true;
  }

  async function refresh() {
    try {
      sessions = await listSessions();
      // A deleted Session never opens something random: flag it instead.
      if (currentId && !sessionMissing && !sessions.some((s) => s.id === currentId)) sessionMissing = true;
    } catch {
      loadError = 'Daemon unreachable. Start it and reload.';
    }
  }

  async function refreshRuns() {
    try {
      runs = (await listRuns()).runs ?? [];
    } catch {
      // Keep the last good Run list.
    }
  }

  function closeStream() {
    if (retryTimer) clearTimeout(retryTimer);
    retryTimer = null;
    if (source) source.close();
    source = null;
  }
  async function openSession(id, { at = 0 } = {}) {
    const token = ++openToken;
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
    sessionMissing = false;
    loadError = '';
    hasMore = false;
    firstSeq = 0;
    loadingEarlier = false;
    atSeq = at;
    try {
      const params = { tail: TAIL_TURNS };
      if (at) params.at = at;
      const body = await getSession(id, params);
      if (token !== openToken || currentId !== id) return;
      meta = body.session;
      events = body.events ?? [];
      resumable = body.resumable ?? true;
      compressible = body.compressible ?? false;
      compressNote = body.compressNote ?? '';
      resumeNote = body.resumeNote ?? '';
      hasMore = body.hasMore ?? false;
      firstSeq = body.firstSeq ?? 0;
      lastSeq = events.reduce((m, e) => Math.max(m, e.seq), 0);
      try {
        localStorage.setItem(LAST_KEY, id);
      } catch {
        // Private mode: the home route falls back to newest.
      }
      // Tail snapshot first, then the live stream replays since lastSeq.
      subscribe(id);
      stopSessionWatch();
      stopSessionWatch = watch(`session:${id}`, () => refreshQueue(id));
      await refreshQueue(id);
    } catch (err) {
      if (token !== openToken || currentId !== id) return;
      if (err?.status === 404) {
        sessionMissing = true;
        meta = null;
        events = [];
      } else {
        loadError = 'Could not load that Session.';
      }
    }
  }

  // Earlier turns prepend above the tail; the Transcript keeps the viewport
  // anchored around the load.
  async function loadEarlier() {
    const id = currentId;
    if (!id || !hasMore || loadingEarlier) return;
    loadingEarlier = true;
    try {
      const body = await getSession(id, { before: firstSeq, turns: TAIL_TURNS });
      if (id !== currentId) return;
      events = [...(body.events ?? []), ...events];
      hasMore = body.hasMore ?? false;
      firstSeq = body.firstSeq ?? firstSeq;
    } catch {
      // The sentinel stays: retry on the next scroll up.
    } finally {
      if (id === currentId) loadingEarlier = false;
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
      if (e.kind === 'prompt') {
        const before = pendingSends;
        const after = dropEchoed(before, e);
        if (after.length !== before.length) noteSwapped(before.filter((p) => !after.includes(p)).map((p) => p.text));
        pendingSends = after;
      }
      if (e.kind === 'sudo_request' && id === currentId) {
        // An approval waits: the turn cannot continue without the user.
        sounds?.playEvent('sudo', { key: sudoKey(e) });
      }
      if (e.kind === 'compress_blocked' && id === currentId && meta) {
        meta = { ...meta, compressBlock: compressBlockReason(e.text) };
      }
      if (e.kind === 'compressed' && id === currentId && meta) {
        meta = { ...meta, compressBlock: '' };
      }
      if (e.kind === 'status' && e.status && id === currentId) {
        const wasWorking = meta?.status === 'working';
        meta = { ...meta, status: e.status };
        sessions = sessions.map((s) => (s.id === id ? { ...s, status: e.status } : s));
        if (wasWorking && (e.status === 'idle' || e.status === 'failed')) {
          sounds?.playEvent(e.status === 'failed' ? 'failed' : 'turnDone', { key: `turn:${id}:${e.seq}` });
        }
        if (e.status === 'ended') {
          resumable = false;
          resumeNote = 'session ended; history is read-only';
        }
        // A turn edge may have delivered or exposed queued messages.
        refreshQueue(id);
      }
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
    return merged.length ? list.filter((p) => !merged.includes(p)) : list;
  }

  // The push key for a sudo request: the request id when it parses, so a
  // push arriving over both the stream and the service worker sounds once.
  function sudoKey(e) {
    try {
      const id = JSON.parse(String(e.text ?? '')).id;
      if (typeof id === 'string' && id) return `sudo:${id}`;
    } catch {
      // Fall through to the sequence fallback.
    }
    return `sudo:${e.sessionId ?? currentId}:${e.seq}`;
  }

  // The message shows at once; a rejection removes it and propagates so the
  // Composer restores the draft.
  async function send(text, mode, images = [], imageIds = [], audio = null) {
    if (drafting) {
      if (sending) return;
      sending = true;
      try {
        const s = await createSession({ harness: draftHarness, model: draftModel, effort: (draftHarness === 'claude' || draftHarness === 'pig') && draftEffort !== 'default' ? draftEffort : '', projectId: draftProjectId });
        await refresh();
        await navigate(buildPath({ name: 'session', id: s.id }));
      } catch (err) {
        loadError = 'Could not start the chat.';
        throw err;
      } finally {
        sending = false;
      }
      mode = 'prompt';
    }
    // Sends in flight never lock the Composer: a slow resume must not leave
    // Send disabled with no explanation. The Daemon orders and queues them.
    if (!currentId) return;
    // A voice recording uploads once the Session exists; its id rides the
    // prompt like an image id, and the model still sees only the text.
    if (audio) {
      const meta = await uploadAudio(currentId, audio);
      imageIds = [...imageIds, meta.id];
    }
    const key = ++pendingKey;
    pendingSends = [...pendingSends, { key, text, mode, images: images.length, imageIds, state: 'sending' }];
    try {
      let res;
      // Only the prompt route carries attachments; it takes the mode too.
      if (images.length || imageIds.length) res = await postPrompt(currentId, text, { mode, images, imageIds });
      else if (mode === 'steer') res = await postSteer(currentId, text);
      else if (mode === 'followup') res = await postFollowup(currentId, text);
      else res = await postPrompt(currentId, text);
      // A queued message shows in the queue panel until it is delivered;
      // an accepted one waits here for its prompt Event.
      if (res?.queued && !res?.seq) pendingSends = pendingSends.filter((p) => p.key !== key);
      else pendingSends = pendingSends.map((p) => (p.key === key ? { ...p, state: 'sent' } : p));
      // Best-effort and off the send path: a slow queue fetch must not keep
      // the Send button disabled.
      void refreshQueue(currentId);
    } catch (err) {
      pendingSends = pendingSends.filter((p) => p.key !== key);
      throw err;
    }
  }

  // The sidebar follows the Daemon: new, spawned, finished and ended Sessions
  // appear without a reload. The router owns the initial open, so refresh
  // never auto-opens here.
  watch('sessions', () => refresh());
  watch('runs', () => refreshRuns());
  watch('users', () => refreshPeople());
  loadProjects();
  // Sign-out unmounts App: close the live stream and drop its watchers.
  onDestroy(stopLive);
  onDestroy(() => {
    people.list = [];
  });

  // Back/forward follows the URL without pushing a new entry.
  const onPop = () => applyRoute(window.location.pathname + window.location.search);
  window.addEventListener('popstate', onPop);
  onDestroy(() => window.removeEventListener('popstate', onPop));
  // Ctrl/⌘+B toggles rail 2: the drawer on phones, the collapse on desktop.
  // Typing keeps the key (bold in editors, and the composer must not lose it).
  // Escape closes the Git panel like every other overlay, with focus back on
  // its toggle.
  const onKeys = (e) => {
    if (e.key === 'Escape' && gitOpen) {
      gitOpen = false;
      document.getElementById('git-toggle')?.focus();
      return;
    }
    if ((e.ctrlKey || e.metaKey) && !e.shiftKey && !e.altKey && (e.key === 'b' || e.key === 'B')) {
      const t = e.target;
      if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.isContentEditable)) return;
      e.preventDefault();
      if (window.matchMedia?.('(max-width: 767px)')?.matches) menuOpen = !menuOpen;
      else toggleSidebar();
    }
  };
  window.addEventListener('keydown', onKeys);
  onDestroy(() => window.removeEventListener('keydown', onKeys));

  // Service worker messages: push sounds play the cue (keyed, so a tab that
  // already sounded the event over its live stream stays silent) and push
  // taps route to the exact path (/s/<id> or /runs/<run>/<ticket>).
  const onSwMessage = (ev) => {
    const data = ev?.data ?? {};
    if (typeof data.cue === 'string' && data.cue) {
      sounds?.playCue(data.cue, { key: typeof data.key === 'string' ? data.key : '' });
    }
    if (typeof data.path === 'string' && data.path.startsWith('/')) {
      void navigate(data.path);
      return;
    }
    const id = data.sessionId;
    if (typeof id === 'string' && id) void navigate(buildPath({ name: 'session', id }));
  };
  if (typeof navigator !== 'undefined' && 'serviceWorker' in navigator) {
    navigator.serviceWorker.addEventListener('message', onSwMessage);
    onDestroy(() => navigator.serviceWorker?.removeEventListener('message', onSwMessage));
  }

  // Boot from the URL: sessions first (home falls back to newest), then route.
  (async () => {
    void refreshPeople();
    void refreshRuns();
    await refresh();
    await applyRoute(window.location.pathname + window.location.search);
  })();
</script>

{#snippet rail2(inDrawer = false)}
  {#if view !== 'quota' && view !== 'memory'}
  <div class="animate-rise flex min-h-0 flex-1 flex-col gap-3">
    {#if view === 'runs'}
      <div class="px-2 pt-1.5 pb-1"><h1 class="text-base font-semibold tracking-tight">Specs and Runs</h1></div>
      <RunsSidebar workDir={meta?.workDir ?? ''} {runId} {ticketId} onNavigate={navigate} />
    {:else if view === 'settings'}
      <div class="px-2 pt-1.5 pb-1"><h1 class="text-base font-semibold tracking-tight">Settings</h1></div>
      <nav class="flex min-h-0 flex-1 flex-col gap-0.5 overflow-auto" data-testid="settings-nav">
        {#each settingsSections as t (t)}
          <button type="button" class="rounded-xl px-2.5 py-2 text-left text-sm text-muted-foreground transition-colors hover:bg-accent hover:text-foreground" onclick={() => jumpSetting(t)}>{t}</button>
        {/each}
      </nav>
    {:else}
      <div class="flex items-center justify-between px-2 pt-1">
        <h1 class="text-sm font-semibold tracking-tight">Emberloop</h1>
        <Button variant="ghost" size="icon-sm" onclick={newChat} aria-label="New Session"><SquarePen /></Button>
      </div>
      <Button variant="secondary" class="justify-start" onclick={newChat}><Plus />New chat</Button>
      <div class="relative">
        <Search class="pointer-events-none absolute top-1/2 left-3.5 size-4 -translate-y-1/2 text-muted-foreground" />
        <Input type="search" placeholder="Filter chats" aria-label="Filter chats" bind:value={chatQuery} class="h-9 rounded-full bg-muted pr-3 pl-10" />
      </div>
      <div class="flex items-center justify-between px-2">
        <span class="text-xs font-medium text-muted-foreground">Sessions</span>
        <Button variant="ghost" size="icon-sm" class="size-6" onclick={() => { if (inDrawer) menuOpen = false; addOpen = true; }} aria-label="Add project"><FolderPlus class="size-3.5" /></Button>
      </div>
      <div class="min-h-0 flex-1 overflow-auto">
        <OwnerGroups users={people.list} me={user} {sessions} {runs} {projects} {currentId} {activeProjectId} query={chatQuery} onSelect={openRow} onNew={startDraft} onRename={rename} onRemove={remove} />
      </div>
    {/if}
  </div>
  {/if}
{/snippet}

{#snippet accountFooter()}
  {#if user}
    <div class="animate-rise flex items-center gap-2 border-t pt-3" data-testid="account">
      <span class="grid size-8 shrink-0 place-items-center rounded-full bg-muted text-xs font-semibold uppercase" aria-hidden="true">{user.username.slice(0, 1)}</span>
      <div class="min-w-0 flex-1 leading-tight">
        <p class="truncate text-sm font-medium">{user.username}</p>
        {#if user.admin}<p class="text-xs text-muted-foreground">Admin</p>{/if}
      </div>
      <Button variant="ghost" size="icon-sm" onclick={onSignOut} aria-label="Sign out" title="Sign out"><LogOut /></Button>
    </div>
  {/if}
{/snippet}

<div class="flex h-dvh">
  <Rail view={view === 'runs' ? 'runs' : view === 'settings' ? 'settings' : view === 'quota' ? 'quota' : view === 'memory' ? 'memory' : 'chat'} collapsed={sideCollapsed} onToggleSidebar={toggleSidebar} onSelect={(id) => navigate(id === 'chat' ? '/' : id === 'runs' ? '/runs' : id === 'quota' ? '/quota' : id === 'memory' ? '/memory' : '/settings')} />
  <aside data-testid="sidebar" inert={sideCollapsed || view === 'quota' || view === 'memory'} aria-hidden={sideCollapsed || view === 'quota' || view === 'memory'} class="flex shrink-0 flex-col overflow-hidden border-r bg-sidebar transition-[width] duration-300 ease-[var(--ease-smooth)] max-md:hidden {sideCollapsed || view === 'quota' || view === 'memory' ? 'w-0 border-transparent' : 'w-64'}">
    <div class="flex min-h-0 w-64 flex-1 flex-col gap-3 p-3" aria-hidden={sideCollapsed}>
      {#key view}
        {@render rail2(false)}
      {/key}
      {@render accountFooter()}
    </div>
  </aside>
  <div class="flex min-w-0 flex-1 flex-col">
    {#if view === 'settings'}
      <header class="flex items-center gap-3 px-4 py-2.5 md:hidden">
        <Button variant="ghost" size="icon-sm" onclick={() => (menuOpen = true)} aria-label="Sessions"><Menu /></Button>
      </header>
      <SettingsPage onBack={() => navigate('/')} admin={!!user?.admin} />
    {:else if view === 'quota'}
      <header class="flex items-center gap-3 px-4 py-2.5 md:hidden">
        <Button variant="ghost" size="icon-sm" onclick={() => (menuOpen = true)} aria-label="Sessions"><Menu /></Button>
      </header>
      <QuotaPage onBack={() => navigate('/')} />
    {:else if view === 'memory'}
      <header class="flex items-center gap-3 px-4 py-2.5 md:hidden">
        <Button variant="ghost" size="icon-sm" onclick={() => (menuOpen = true)} aria-label="Sessions"><Menu /></Button>
      </header>
      <MemoryPage onBack={() => navigate('/')} />
    {:else if view === 'runs'}
      <header class="flex items-center gap-3 px-4 py-2.5 md:hidden">
        <Button variant="ghost" size="icon-sm" onclick={() => (menuOpen = true)} aria-label="Sessions"><Menu /></Button>
      </header>
      <RunsView workDir={meta?.workDir ?? ''} {runId} {ticketId} onOpenSession={goSession} onNavigate={navigate} onOpenMenu={() => (menuOpen = true)} />
    {:else}
    <header class="flex items-center gap-2 px-3 py-2.5 backdrop-blur-xl sm:gap-3 sm:px-4">
      <Button variant="ghost" size="icon-sm" class="md:hidden" onclick={() => (menuOpen = true)} aria-label="Sessions"><Menu /></Button>
      <div class="flex min-w-0 flex-col leading-tight" data-testid="header-title">
        <span class="truncate text-sm font-semibold">{meta?.title || activeProject?.name || (drafting ? 'New chat' : 'No Session')}</span>
        <span class="truncate text-xs text-muted-foreground">{[creatorName ? `created by ${creatorName}` : '', meta?.title ? activeProject?.name : '', meta ? `${meta.harness} · ${modelLabel(meta.harness, meta.model)}${meta.effort ? ` · ${meta.effort}` : ''}` : drafting ? `${draftHarness} · ${modelLabel(draftHarness, draftModel)}${(draftHarness === 'claude' || draftHarness === 'pig') && draftEffort !== 'default' ? ` · ${draftEffort}` : ''}` : ''].filter(Boolean).join(' · ')}</span>
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
          <Button variant="outline" size="sm" id="git-toggle" onclick={() => (gitOpen = !gitOpen)} aria-pressed={gitOpen}>
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
        {#if sessionMissing}
          <div class="mx-auto flex w-full max-w-3xl flex-col items-center gap-3 px-4 py-24 text-center" data-testid="session-missing">
            <p class="text-lg font-semibold tracking-tight">Session not found</p>
            <p class="text-sm text-muted-foreground">It was deleted or the link is wrong. Pick another chat.</p>
            <Button variant="secondary" onclick={() => navigate('/')}>Back to chats</Button>
          </div>
        {:else}
        {#if currentId && !resumable}
          <p data-testid="read-only" class="mx-4 rounded-xl bg-amber-500/10 px-4 py-2 text-sm text-amber-300">
            Read-only history. {resumeNote}
          </p>
        {:else if currentId && meta?.scout}
          <p data-testid="scout-read-only" class="mx-4 rounded-xl bg-amber-500/10 px-4 py-2 text-sm text-amber-300">
            Scout session. Read-only transcript.
          </p>
        {/if}
        <Transcript
          {blocks}
          {hasMore}
          {loadingEarlier}
          onLoadEarlier={loadEarlier}
          {atSeq}
          bottomPad={overlayH + 16}
          emptyHint={currentId ? 'Send a prompt to start.' : drafting ? `What should we do in ${activeProject?.name ?? 'this project'}?` : projects.length ? 'Pick a project, then start a chat.' : 'Add a project to begin.'}
          sessionId={currentId}
          {sessions}
          onOpenSession={goSession}
        />
        {/if}
        <div bind:clientHeight={overlayH} class="pointer-events-none absolute inset-x-0 bottom-0 bg-gradient-to-t from-background via-background/90 to-transparent px-3 pb-[max(1.25rem,env(safe-area-inset-bottom))] pt-10 sm:px-4">
          <div class="pointer-events-auto mx-auto flex w-full max-w-3xl flex-col gap-2">
            {#if currentId && meta?.compressBlock}
              {@const bannerFor = currentId}
              {#key bannerFor}
                <CompressBanner sessionId={bannerFor} onChangeModel={() => navigate('/settings')} onDismissed={() => clearCompressBlock(bannerFor)} />
              {/key}
            {/if}
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
            {#if !sessionMissing}
            {#if drafting}
              <DraftBar bind:harness={draftHarness} bind:model={draftModel} bind:effort={draftEffort} />
              <Composer status="idle" busy={sending} onSend={send} />
            {:else if currentId && resumable && !meta?.scout}
              <Composer {status} busy={sending} sessionId={currentId} onSend={send} onStop={() => interruptSession(currentId)} />
              <ContextMeter model={modelLabel(meta?.harness, meta?.model ?? '')} {usage} onCompress={canCompress ? () => { compressError = ''; compressOpen = true; } : null} busy={status === 'working'} {compressing} {compressNote} />
            {/if}
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

<Sheet bind:open={menuOpen} title={view === 'runs' ? 'Specs and Runs' : view === 'settings' ? 'Settings' : view === 'quota' ? 'Quota' : view === 'memory' ? 'Memory' : 'Sessions'}>
  {#key view}
    {@render rail2(true)}
  {/key}
  <Button variant="ghost" class="justify-start" onclick={() => navigate('/')}><MessageSquare />Chat</Button>
  <Button variant="ghost" class="justify-start" onclick={() => navigate('/runs')}><Layers />Runs</Button>
  <Button variant="ghost" class="justify-start" onclick={() => navigate('/memory')}><Brain />Memory</Button>
  <Button variant="ghost" class="justify-start" onclick={() => navigate('/quota')}><Gauge />Quota</Button>
  <Button variant="ghost" class="justify-start" onclick={() => navigate('/settings')}><Settings />Settings</Button>
  {@render accountFooter()}
</Sheet>
<CompressDialog open={compressOpen} busy={compressBusy} error={compressError} {usage} onClose={() => (compressOpen = false)} onCompress={runCompress} />
<AddProjectDialog open={addOpen} busy={adding} error={addError} onClose={() => (addOpen = false)} onAdd={add} />
<VersionBadge panelOpen={gitOpen} connected={connected} />
