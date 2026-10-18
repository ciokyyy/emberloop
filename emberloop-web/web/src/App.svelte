<script>
  import { onDestroy, setContext } from 'svelte';
  import { Plus, FolderPlus, Search, LogOut, Settings } from '@lucide/svelte';
  import Button from './lib/ui/button.svelte';
  import Input from './lib/ui/input.svelte';
  import Badge from './lib/ui/badge.svelte';
  import Sheet from './lib/ui/sheet.svelte';
  import PageHeader from './lib/ui/page-header.svelte';
  import IconButton from './lib/ui/icon-button.svelte';
  import Skeleton from './lib/ui/skeleton.svelte';
  import EmptyState from './lib/ui/empty-state.svelte';
  import ErrorState from './lib/ui/error-state.svelte';
  import { SHELL_KEY } from './lib/shell.js';
  import { icons } from './lib/icons.js';
  import { dotState } from './lib/owners.js';
  import Transcript from './lib/Transcript.svelte';
  import OwnerGroups from './lib/OwnerGroups.svelte';
  import AddProjectDialog from './lib/AddProjectDialog.svelte';
  import DraftBar from './lib/DraftBar.svelte';
  import GitPanel from './lib/GitPanel.svelte';
  import CompressDialog from './lib/CompressDialog.svelte';
  import CompressBanner from './lib/CompressBanner.svelte';
  import DiskBanner from './lib/DiskBanner.svelte';
  import Composer from './lib/Composer.svelte';
  import SettingsPage from './lib/SettingsPage.svelte';
  import SettingsHub from './lib/SettingsHub.svelte';
  import SettingsNav from './lib/SettingsNav.svelte';
  import { SETTINGS_PAGES } from './lib/settings.js';
  import RunsView from './lib/RunsView.svelte';
  import QuotaPage from './lib/QuotaPage.svelte';
  import ProvidersPage from './lib/ProvidersPage.svelte';
  import MemoryPage from './lib/MemoryPage.svelte';
  import ContextMeter from './lib/ContextMeter.svelte';
  import ChatStatus from './lib/ChatStatus.svelte';
  import { activeTool } from './lib/toolstrip.js';
  import VersionBadge from './lib/VersionBadge.svelte';
  import Rail from './lib/Rail.svelte';
  import QuotaSidebar from './lib/QuotaSidebar.svelte';
  import RunsSidebar from './lib/RunsSidebar.svelte';
  import ProjectSwitcher from './lib/ProjectSwitcher.svelte';
  import DashboardPage from './lib/DashboardPage.svelte';
  import ProjectSettingsPage from './lib/ProjectSettingsPage.svelte';
  import DrawerNav from './lib/DrawerNav.svelte';
  import { modelLabel } from './lib/harnesses.js';
  import { groupTranscriptCached, createTranscriptCache, latestUsageCached, createUsageCache, withImageNote, compressBlockReason, parseReport } from './lib/events.js';
  import { parsePath, buildPath } from './lib/router.js';
  import { watch, stopLive } from './lib/live.js';
  import { soundPlayer } from './lib/sound.js';
  import { readJSON, writeJSON } from './lib/sidebar.js';
  import { listSessions, createSession, uploadAudio, getSession, postPrompt, postSteer, postFollowup, getQueue, clearQueue, interruptSession, openStream, compressSession, listProjects, suggestProject, addProject, renameProject, removeProject, updateProject, getGit, getDisk } from './lib/api.js';
  import { people, refreshPeople, personFor } from './lib/people.svelte.js';
  import { runsStore } from './lib/runsStore.js';
  import { readReportState, writeReportState, rememberReports, receiveReport, setReportDismissed, markReportsRead } from './lib/reportState.js';

  // The signed-in User and the sign-out action, both supplied by Root.svelte.
  let { user = null, onSignOut = () => {} } = $props();

  // Interface sounds. Created on mount so the first-gesture unlock listens
  // from the start; every cue after that is one playEvent call. Message
  // sends and subagent finishes are silent by design (no hook here).
  const sounds = soundPlayer();

  let sessions = $state([]);
  let sessionsLoaded = $state(false);
  let sessionsDirty = false;
  let sessionsRevision = 0;
  let currentId = $state(null);
  let meta = $state(null);
  let events = $state([]);
  let reportState = $state({ reports: [], dismissed: [], readSeq: 0, latestSeq: 0 });
  let lastSeq = $state(0);
  let connected = $state(true);
  let projects = $state([]);
  let projectsLoaded = $state(false);
  let runs = $state([]);
  let draftProjectId = $state(null);
  let draftOpen = $state(false);
  let draftPick = $state(null);
  let draftPickGeneration = 0;
  let failedDraftPrompt = $state(null);
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
  let disk = $state(null);
  async function refreshDisk() {
    try {
      disk = await getDisk();
    } catch {
      disk = null;
    }
  }
  const PREF_KEY = 'eb.pref';
  const pref = JSON.parse(localStorage.getItem(PREF_KEY) ?? '{}');
  let draftHarness = $state(pref.harness ?? 'claude');
  let draftModel = $state(pref.model ?? 'sonnet');
  let draftEffort = $state(pref.effort ?? 'default');
  $effect(() => {
    localStorage.setItem(PREF_KEY, JSON.stringify({ harness: draftHarness, model: draftModel, effort: draftEffort }));
  });
  const activeProjectId = $derived(currentId ? (meta?.projectId ?? null) : draftOpen ? draftProjectId : null);
  const activeProject = $derived(projects.find((p) => p.id === activeProjectId) ?? null);
  const drafting = $derived(!currentId && draftOpen);
  const creatorName = $derived(meta?.owner ? (personFor(meta.owner)?.username ?? meta.owner.slice(0, 8)) : '');
  const initialPath = window.location.pathname + window.location.search;
  const initialRoute = parsePath(initialPath);
  let route = $state(initialRoute);
  const viewForRoute = (r) => {
    if (['runs', 'run', 'ticket', 'project-runs', 'project-run', 'project-ticket'].includes(r.name)) return 'runs';
    if (r.name === 'project-settings') return 'project-settings';
    if (r.name === 'settings') return 'settings';
    if (r.name === 'quota') return 'quota';
    if (r.name === 'providers') return 'providers';
    if (r.name === 'memory') return 'memory';
    if (r.name === 'home') return 'dashboard';
    return 'chat';
  };
  let view = $state(viewForRoute(initialRoute));
  const areaView = $derived(view === 'providers' ? 'quota' : view === 'dashboard' || view === 'project-settings' ? 'chat' : view);
  $effect(() => {
    // Keep the shared Run-list request alive while moving between its two
    // consumers; global pages should not load summaries they never display.
    if (view !== 'chat' && view !== 'runs') {
      runs = [];
      return;
    }
    return runsStore.subscribe((value) => { runs = value; });
  });
  const isPhone = () => !!window.matchMedia?.('(max-width: 767px)').matches;
  let menuOpen = $state(false);
  // Every page header (ui/page-header.svelte) renders the phone menu button
  // from this context, so no page repeats the drawer wiring.
  const MENU_LABELS = {
    settings: 'Settings menu',
    providers: 'Quota and Providers menu',
    dashboard: 'Project switcher',
    'project-settings': 'Project switcher',
  };
  setContext(SHELL_KEY, {
    openMenu: () => (menuOpen = true),
    get menuLabel() {
      return MENU_LABELS[view] ?? 'Sessions';
    },
  });
  // Areas where something needs the User: a needs-you dot on the Rail and the
  // drawer tabs. Runs are known only while the Run list is subscribed.
  const attention = $derived({
    chat: sessions.some((s) => dotState(s) === 'needs' && (!user || !s.owner || s.owner === user.id)),
    runs: runs.some((r) => (r.counts?.needsYou ?? 0) > 0),
  });
  // The phone drawer owns one transient history entry while open, so system
  // Back closes it instead of leaving the app. A navigation from the drawer
  // reuses that entry (replaceState); a plain close (scrim, Escape, X) pops it.
  let drawerEntry = false;
  let ignorePop = false;
  $effect(() => {
    if (menuOpen && !drawerEntry) {
      history.pushState({ drawer: true }, '', window.location.href);
      drawerEntry = true;
    } else if (!menuOpen && drawerEntry) {
      drawerEntry = false;
      if (history.state?.drawer) {
        ignorePop = true;
        history.back();
      }
    }
  });
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
  const scopedRouteNames = ['project', 'project-runs', 'project-run', 'project-ticket', 'project-settings'];
  const isProjectRoute = $derived(scopedRouteNames.includes(route.name));
  const scopeProjectId = $derived.by(() => {
    if (isProjectRoute) return route.projectId === 'none' ? '' : route.projectId;
    if (route.name !== 'session') return null;
    if (meta?.id === route.id) return meta.projectId ?? '';
    const summary = sessionsById[route.id];
    return summary ? (summary.projectId ?? '') : null;
  });
  const isProjectScoped = $derived(isProjectRoute || (route.name === 'session' && scopeProjectId !== null));
  const switcherProjectId = $derived(isProjectScoped ? scopeProjectId : null);
  const scopedProjectRouteId = $derived(isProjectScoped ? (scopeProjectId || 'none') : null);
  const scopedProject = $derived(scopedProjectRouteId && scopedProjectRouteId !== 'none'
    ? projects.find((project) => project.id === scopedProjectRouteId) ?? null
    : null);
  // Grouping memoizes the settled prefix: a chunk burst only replaces the
  // tail block, so settled cards keep their object identity (no re-render,
  // no re-measure, no restarted entrance animation). The caches live outside
  // the deriveds on purpose: regroups stay driven by events/sessions/status.
  let transcriptCache = createTranscriptCache();
  let usageCache = createUsageCache();
  const rawBlocks = $derived([
    ...groupTranscriptCached(events, sessionsById, { working: meta?.status === 'working' }, transcriptCache).map((b) =>
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
  const blocks = $derived(rawBlocks.filter((block) => !block.report || !reportState.dismissed.includes(Number(block.report.seq))));
  const sidebarSessions = $derived(sessions.map((session) => {
    const state = session.id === currentId ? reportState : readReportState(user?.id, session.id);
    return {
      ...session,
      reportUnread: Math.max(Number(session.reportSeq) || 0, state.latestSeq) > state.readSeq,
    };
  }));
  function syncReportState(id, eventList, readThrough = 0) {
    if (!id || !user?.id) return;
    const reports = eventList
      .filter((event) => event.kind === 'prompt')
      .map((event) => {
        const report = parseReport(event.text);
        return report ? { ...report, seq: event.seq } : null;
      })
      .filter(Boolean);
    let next = rememberReports(readReportState(user.id, id), reports);
    const latest = Math.max(Number(readThrough) || 0, next.latestSeq, ...reports.map((report) => Number(report.seq) || 0));
    if (latest) next = markReportsRead(next, latest);
    writeReportState(user.id, id, next);
    if (currentId === id) reportState = next;
  }
  function noteIncomingReport(id, event) {
    if (!id || !user?.id) return;
    const report = parseReport(event.text);
    if (!report) return;
    const next = receiveReport(readReportState(user.id, id), { ...report, seq: event.seq }, currentId === id);
    writeReportState(user.id, id, next);
    if (currentId === id) reportState = next;
    sessions = sessions.map((session) => session.id === id ? { ...session, reportSeq: event.seq } : session);
  }
  function changeReportDismissal(seq, dismissed) {
    if (!currentId || !user?.id) return;
    reportState = setReportDismissed(reportState, seq, dismissed);
    writeReportState(user.id, currentId, reportState);
  }
  const usage = $derived(latestUsageCached(events, usageCache));
  const status = $derived(meta?.status ?? 'idle');
  // Height of the floating composer stack: the transcript pads by it so the last block never hides.
  let overlayH = $state(176);

  // Routes: / is the Dashboard; /p/<id> scopes Sessions and Runs, while /s/<id>
  // stays canonical and derives its scope from the Session metadata.
  const TAIL_TURNS = 6;
  const LAST_KEY = 'eb.last';
  let hasMore = $state(false);
  let firstSeq = $state(0);
  let loadingEarlier = $state(false);
  let sessionMissing = $state(false);
  let runId = $state(initialRoute.runId ?? null);
  let ticketId = $state(initialRoute.ticketId ?? null);
  let atSeq = $state(0);
  let openToken = 0;

  const goSession = (id) => navigate(buildPath({ name: 'session', id }));
  const openRow = (row) => {
    if (row.kind !== 'run') {
      if (currentId === row.id) syncReportState(row.id, [], reportState.latestSeq);
      return goSession(row.id);
    }
    const path = scopedProjectRouteId
      ? buildPath({ name: 'project-run', projectId: scopedProjectRouteId, runId: row.id })
      : buildPath({ name: 'run', runId: row.id });
    return navigate(path);
  };

  function navigate(path, { replace = false } = {}) {
    // Any navigation closes the phone drawer (backdrop taps close it
    // through the dialog itself).
    const fromDrawer = drawerEntry && history.state?.drawer;
    menuOpen = false;
    if (fromDrawer) {
      // The drawer's own entry becomes the destination: no dead Back step.
      drawerEntry = false;
      history.replaceState(null, '', path);
    } else if (replace) history.replaceState(null, '', path);
    else if (window.location.pathname + window.location.search !== path) history.pushState(null, '', path);
    return applyRoute(path);
  }

  function clearSessionView() {
    closeStream();
    stopSessionWatch();
    openToken++;
    draftPickGeneration++;
    draftPick = null;
    failedDraftPrompt = null;
    batch = [];
    pendingSends = [];
    queue = [];
    currentId = null;
    meta = null;
    events = [];
    lastSeq = 0;
    draftOpen = false;
    draftProjectId = null;
    sessionMissing = false;
    hasMore = false;
    firstSeq = 0;
    loadingEarlier = false;
    atSeq = 0;
    resumable = true;
    resumeNote = '';
    reportState = { reports: [], dismissed: [], readSeq: 0, latestSeq: 0 };
  }

  function resolveProjectScope(projectId) {
    if (route.name !== 'project' || route.projectId !== projectId || draftOpen) return;
    const wanted = projectId === 'none' ? '' : projectId;
    const belongs = (session) => (session.projectId ?? '') === wanted;
    const currentMatches = currentId && sessionsById[currentId] && belongs(sessionsById[currentId]) && meta?.id === currentId && belongs(meta);
    let id = currentMatches ? currentId : null;
    if (!id) {
      const scopedSessions = sessions.filter(belongs);
      let last = null;
      try {
        last = localStorage.getItem(LAST_KEY);
      } catch {
        // Private mode: use the newest Session in this scope.
      }
      id = scopedSessions.some((session) => session.id === last) ? last : scopedSessions[0]?.id ?? null;
    }
    if (id) {
      if (id !== currentId) void openSession(id);
      else view = 'chat';
      return;
    }
    if (projectId === 'none' || projects.some((project) => project.id === projectId)) {
      startDraft(projectId === 'none' ? null : projectId);
    } else {
      clearSessionView();
      view = 'chat';
    }
  }

  function showDashboard() {
    clearSessionView();
    view = 'dashboard';
    menuOpen = false;
    gitOpen = false;
  }

  async function applyRoute(path) {
    const r = parsePath(path);
    if (r.name === 'unknown') {
      history.replaceState(null, '', '/');
      return applyRoute('/');
    }
    route = r;
    if (r.name === 'settings') {
      if (!r.section && !isPhone()) {
        const destination = '/settings/agents';
        history.replaceState(null, '', destination);
        return applyRoute(destination);
      }
      view = 'settings';
      menuOpen = false;
      return;
    }
    if (r.name === 'quota') {
      view = 'quota';
      menuOpen = false;
      return;
    }
    if (r.name === 'providers') {
      view = 'providers';
      menuOpen = false;
      return;
    }
    if (r.name === 'memory') {
      view = 'memory';
      menuOpen = false;
      return;
    }
    if (r.name === 'project-settings') {
      view = 'project-settings';
      menuOpen = false;
      return;
    }
    if (['runs', 'run', 'ticket', 'project-runs', 'project-run', 'project-ticket'].includes(r.name)) {
      view = 'runs';
      menuOpen = false;
      runId = r.runId ?? null;
      ticketId = r.ticketId ?? null;
      return;
    }
    if (r.name === 'session') {
      draftOpen = false;
      draftProjectId = null;
      void refresh();
      if (r.id !== currentId) return openSession(r.id, { at: r.at ?? 0 });
      if (r.at) atSeq = r.at;
      view = 'chat';
      menuOpen = false;
      return;
    }
    if (r.name === 'project') {
      view = 'chat';
      menuOpen = false;
      runId = null;
      ticketId = null;
      const matchingDraft = draftOpen && (r.projectId === 'none' ? draftProjectId == null : draftProjectId === r.projectId);
      if (!matchingDraft) {
        draftOpen = false;
        draftProjectId = null;
      } else {
        if (!sessionsLoaded || sessionsDirty) void refresh();
        return;
      }
      if (!sessionsLoaded || sessionsDirty) {
        void refresh().then(() => {
          if (window.location.pathname + window.location.search === path) resolveProjectScope(r.projectId);
        });
        return;
      }
      resolveProjectScope(r.projectId);
      return;
    }
    if (r.name === 'home') {
      showDashboard();
      return;
    }
  }

  async function loadProjects() {
    try {
      projects = await listProjects();
    } catch {
      // The Session list reports an unreachable Daemon.
    } finally {
      projectsLoaded = true;
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
      beginScopedDraft(p.id);
    } catch (err) {
      addError = err.message || 'Could not add the project.';
    } finally {
      adding = false;
    }
  }
  function confirmProjectRemoval(p) {
    return window.confirm(`Remove "${p.name}" from the list? Its chats and files stay.`);
  }

  async function renameProjectRecord(p, name) {
    const updated = await renameProject(p.id, name);
    await loadProjects();
    return updated;
  }

  async function rename(p) {
    const name = window.prompt('Rename project', p.name);
    if (name && name.trim() && name !== p.name) await renameProjectRecord(p, name.trim());
  }

  async function removeProjectRecord(p) {
    await removeProject(p.id);
    if (draftProjectId === p.id) draftProjectId = null;
    await loadProjects();
  }

  async function remove(p) {
    if (confirmProjectRemoval(p)) await removeProjectRecord(p);
  }

  async function saveProjectGist(p, gist) {
    const updated = await updateProject(p.id, { gist });
    await loadProjects();
    return updated;
  }

  async function removeFromSettings(p) {
    if (!confirmProjectRemoval(p)) return false;
    await removeProjectRecord(p);
    await navigate('/');
    return true;
  }

  // A draft creates its Session only when the first prompt is sent.
  function startDraft(projectId) {
    clearSessionView();
    view = 'chat';
    menuOpen = false;
    draftProjectId = projectId;
    draftOpen = true;
  }

  function beginScopedDraft(projectId) {
    const id = projectId ?? null;
    const path = buildPath({ name: 'project', projectId: id ?? 'none' });
    void navigate(path);
    startDraft(id);
  }

  function newChat() {
    beginScopedDraft(isProjectScoped && scopeProjectId !== null ? scopeProjectId || null : null);
  }

  async function refresh() {
    const revision = sessionsRevision;
    try {
      sessions = await listSessions();
      sessionsDirty = sessionsRevision !== revision;
      sessionsLoaded = true;
      // A deleted Session never opens something random: flag it instead.
      if (currentId && meta?.id === currentId && !sessionMissing && !sessions.some((s) => s.id === currentId)) sessionMissing = true;
    } catch {
      sessionsDirty = false;
      sessionsLoaded = true;
      loadError = 'Daemon unreachable. Start it and reload.';
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
    reportState = readReportState(user?.id, id);
    draftOpen = false;
    draftPickGeneration++;
    draftPick = null;
    failedDraftPrompt = null;
    draftProjectId = null;
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
      syncReportState(id, body.reports ?? events, body.session?.reportSeq ?? 0);
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
      syncReportState(id, body.events ?? []);
      hasMore = body.hasMore ?? false;
      firstSeq = body.firstSeq ?? firstSeq;
    } catch {
      // The sentinel stays: retry on the next scroll up.
    } finally {
      if (id === currentId) loadingEarlier = false;
    }
  }

  // Events arrive per delta; regrouping per delta is O(L^2) over a burst, so
  // chunks coalesce into one flush per window while structural events flush
  // at once and never wait on a burst.
  const FLUSH_MS = 80;
  let batch = [];
  let flushTimer = 0;
  let lastFlush = 0;
  function flushBatch() {
    if (flushTimer) {
      clearTimeout(flushTimer);
      flushTimer = 0;
    }
    if (!batch.length) return;
    lastFlush = performance.now();
    const fresh = batch;
    batch = [];
    events = [...events, ...fresh];
  }
  function scheduleChunk() {
    if (flushTimer) return;
    const wait = FLUSH_MS - (performance.now() - lastFlush);
    if (wait <= 0) {
      if (typeof requestAnimationFrame === 'undefined') flushBatch();
      else requestAnimationFrame(flushBatch);
    } else {
      flushTimer = setTimeout(flushBatch, wait);
    }
  }
  const isChunkEvent = (e) => e.kind === 'assistant_chunk' || e.kind === 'thinking_chunk' || e.kind === 'tool_call_updated';

  function subscribe(id) {
    source = openStream(id, lastSeq, (e) => {
      if (e.seq <= lastSeq) return;
      lastSeq = e.seq;
      batch.push(e);
      if (e.kind === 'prompt' && parseReport(e.text)) noteIncomingReport(id, e);
      if (isChunkEvent(e)) scheduleChunk();
      else flushBatch();
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
      if (sending || draftPick) return;
      sending = true;
      const generation = draftPickGeneration;
      const settings = { projectId: draftProjectId, harness: draftHarness, model: draftModel, effort: draftEffort };
      const message = { text, images, imageIds, audio };
      try {
        if (settings.projectId == null && text.trim() && !audio) {
          let suggestedProject = null;
          try {
            suggestedProject = (await suggestProject(text))?.suggestedProject ?? null;
          } catch {
            // Jev is advisory: an unavailable suggestion keeps today's start path.
          }
          if (generation !== draftPickGeneration || !drafting) {
            // The Composer has already handed off this message. If navigation
            // closed its draft while Jev was running, honor the send on the
            // original settings rather than silently losing the prompt.
            await createDraftAndSend(settings.projectId, message, settings);
            return;
          }
          if (suggestedProject?.id) {
            draftPick = { project: suggestedProject, message };
            return;
          }
        }
        await createDraftAndSend(settings.projectId, message);
      } catch (err) {
        loadError = 'Could not start the chat.';
        throw err;
      } finally {
        sending = false;
      }
      return;
    }
    await deliverToSession(currentId, text, mode, images, imageIds, audio);
  }

  async function createDraftAndSend(projectId, message, settings = null) {
    const selected = settings ?? { harness: draftHarness, model: draftModel, effort: draftEffort };
    const session = await createSession({
      harness: selected.harness,
      model: selected.model,
      effort: (selected.harness === 'claude' || selected.harness === 'pig') && selected.effort !== 'default' ? selected.effort : '',
      projectId,
    });
    await refresh();
    await navigate(buildPath({ name: 'session', id: session.id }));
    await deliverToSession(session.id, message.text, 'prompt', message.images, message.imageIds, message.audio);
  }

  async function resolveDraftPick(useSuggestedProject) {
    const pick = draftPick;
    if (!pick || sending) return;
    sending = true;
    try {
      await createDraftAndSend(useSuggestedProject ? pick.project.id : null, pick.message);
    } catch {
      if (currentId && !drafting) {
        failedDraftPrompt = { sessionId: currentId, message: pick.message };
        loadError = 'The Session started, but the first prompt was not sent.';
      } else {
        loadError = 'Could not start the chat.';
      }
    } finally {
      sending = false;
    }
  }

  async function retryFailedDraftPrompt() {
    const failed = failedDraftPrompt;
    if (!failed || sending || failed.sessionId !== currentId) return;
    sending = true;
    try {
      await deliverToSession(failed.sessionId, failed.message.text, 'prompt', failed.message.images, failed.message.imageIds, failed.message.audio);
      if (failedDraftPrompt === failed) failedDraftPrompt = null;
      loadError = '';
    } catch {
      loadError = 'Could not send the first prompt. Retry below.';
    } finally {
      sending = false;
    }
  }

  async function deliverToSession(id, text, mode, images = [], imageIds = [], audio = null) {
    if (!id) return;
    // A voice recording uploads once the Session exists; its id rides the
    // prompt like an image id, and the model still sees only the text.
    if (audio) {
      const meta = await uploadAudio(id, audio);
      imageIds = [...imageIds, meta.id];
    }
    const key = ++pendingKey;
    pendingSends = [...pendingSends, { key, text, mode, images: images.length, imageIds, state: 'sending' }];
    try {
      let res;
      // Only the prompt route carries attachments; it takes the mode too.
      if (images.length || imageIds.length) res = await postPrompt(id, text, { mode, images, imageIds });
      else if (mode === 'steer') res = await postSteer(id, text);
      else if (mode === 'followup') res = await postFollowup(id, text);
      else res = await postPrompt(id, text);
      // A queued message shows in the queue panel until it is delivered;
      // an accepted one waits here for its prompt Event.
      if (res?.queued && !res?.seq) pendingSends = pendingSends.filter((p) => p.key !== key);
      else pendingSends = pendingSends.map((p) => (p.key === key ? { ...p, state: 'sent' } : p));
      // Best-effort and off the send path: a slow queue fetch must not keep
      // the Send button disabled.
      void refreshQueue(id);
    } catch (err) {
      pendingSends = pendingSends.filter((p) => p.key !== key);
      throw err;
    }
  }

  // Session changes while another area is open mark the sidebar data stale;
  // home refreshes before resolving the last/newest Session.
  watch('sessions', (topic) => {
    if (topic === 'hello' && !sessionsLoaded) return;
    sessionsRevision += 1;
    if (view !== 'chat') {
      sessionsDirty = true;
      return;
    }
    void refresh();
  });
  watch('users', () => refreshPeople());
  watch('disk', () => refreshDisk());
  refreshDisk();
  loadProjects();
  // Sign-out unmounts App: close the live stream and drop its watchers.
  onDestroy(stopLive);
  onDestroy(() => {
    people.list = [];
  });

  // Back/forward follows the URL without pushing a new entry.
  const onPop = (e) => {
    if (ignorePop) {
      ignorePop = false;
      return;
    }
    // Back with the drawer open only closes it.
    if (menuOpen) {
      drawerEntry = false;
      menuOpen = false;
      return;
    }
    // Forward onto a stale drawer entry: stay on the page.
    if (e?.state?.drawer) return;
    applyRoute(window.location.pathname + window.location.search);
  };
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
  // taps route to the exact canonical Session or scoped/unscoped Run URL.
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

  function navigateArea(id) {
    if (id === 'chat') {
      const path = scopedProjectRouteId ? buildPath({ name: 'project', projectId: scopedProjectRouteId }) : '/';
      return navigate(path);
    }
    if (id === 'runs') {
      const path = scopedProjectRouteId ? buildPath({ name: 'project-runs', projectId: scopedProjectRouteId }) : '/runs';
      return navigate(path);
    }
    return navigate(id === 'quota' ? '/quota' : id === 'memory' ? '/memory' : '/settings');
  }
  void refreshPeople();
  // The requested area is available immediately. Home resolves its last/newest
  // Session in the background; Runs and Quota load their own data on mount.
  void applyRoute(initialPath);
</script>

{#snippet projectSettingsLink()}
  {#if scopedProject}
    <Button variant="ghost" size="sm" class="justify-start text-fg-2" onclick={() => navigate(buildPath({ name: 'project-settings', projectId: scopedProject.id }))}><Settings />Project settings</Button>
  {/if}
{/snippet}

{#snippet sideTitle(text)}
  <h2 class="px-2 pt-1 text-title text-fg-1">{text}</h2>
{/snippet}

{#snippet rail2(inDrawer = false)}
  {#if view !== 'memory'}
  <div class="animate-rise flex min-h-0 flex-1 flex-col gap-3">
    {#if view === 'runs'}
      <ProjectSwitcher projects={projects} selectedId={switcherProjectId} onNavigate={navigate} />
      {@render projectSettingsLink()}
      {@render sideTitle('Specs and Runs')}
      <RunsSidebar
        workDir={isProjectScoped ? (scopedProject?.path ?? '') : (meta?.workDir ?? '')}
        projectId={scopedProjectRouteId}
        projectsLoaded={projectsLoaded}
        {runId}
        {ticketId}
        onNavigate={navigate}
      />
    {:else if view === 'project-settings'}
      <ProjectSwitcher projects={projects} selectedId={switcherProjectId} onNavigate={navigate} />
      {@render sideTitle('Project settings')}
    {:else if view === 'settings'}
      {@render sideTitle('Settings')}
      <SettingsNav current={route.section ?? null} onNavigate={navigate} />
    {:else if view === 'quota' || view === 'providers'}
      <QuotaSidebar onNavigate={navigate} inDrawer={inDrawer} current={view} />
    {:else if view === 'dashboard'}
      <ProjectSwitcher projects={projects} selectedId={null} onNavigate={navigate} />
      <Button variant="secondary" class="justify-start" onclick={() => { if (inDrawer) menuOpen = false; addOpen = true; }}><FolderPlus />Add project</Button>
    {:else}
      <ProjectSwitcher projects={projects} selectedId={switcherProjectId} onNavigate={navigate} />
      {@render projectSettingsLink()}
      {#if !inDrawer}
        {@render sideTitle(scopedProject?.name ?? (scopeProjectId === '' && isProjectScoped ? 'No project' : 'Sessions'))}
      {/if}
      <Button variant="secondary" class="justify-start" onclick={newChat}><Plus />New chat</Button>
      <div class="relative">
        <Search class="pointer-events-none absolute top-1/2 left-3.5 size-4 -translate-y-1/2 text-fg-3" aria-hidden="true" />
        <Input type="search" placeholder="Filter chats" aria-label="Filter chats" bind:value={chatQuery} class="h-9 rounded-full bg-muted pr-3 pl-10 max-md:h-11" />
      </div>
      <div class="flex items-center justify-between px-2">
        <h2 class="text-caption font-medium text-fg-3">Sessions</h2>
        <IconButton icon={FolderPlus} label="Add project" class="size-7 max-md:size-11" onclick={() => { if (inDrawer) menuOpen = false; addOpen = true; }} />
      </div>
      <div class="min-h-0 flex-1 overflow-auto">
        {#if sessionsLoaded && (route.name !== 'session' || scopeProjectId !== null)}
          <OwnerGroups
            users={people.list}
            me={user}
            sessions={sidebarSessions}
            {runs}
            {projects}
            {currentId}
            {activeProjectId}
            query={chatQuery}
            scoped={isProjectScoped}
            scopeProjectId={scopeProjectId ?? ''}
            onSelect={openRow}
            onNew={beginScopedDraft}
            onRename={rename}
            onRemove={remove}
          />
        {:else}
          <div role="status" class="flex flex-col gap-1 px-1" data-testid="sessions-loading">
            <span class="sr-only">{sessionsLoaded ? 'Loading Project scope…' : 'Loading Sessions…'}</span>
            {#each [0, 1, 2, 3] as i (i)}<Skeleton class="h-12 rounded-xl" />{/each}
          </div>
        {/if}
      </div>
    {/if}
  </div>
  {/if}
{/snippet}

{#snippet accountFooter()}
  {#if user}
    <div class="animate-rise flex items-center gap-2 border-t pt-3" data-testid="account">
      <span class="grid size-8 shrink-0 place-items-center rounded-full bg-muted text-caption font-semibold uppercase" aria-hidden="true">{user.username.slice(0, 1)}</span>
      <div class="min-w-0 flex-1 leading-tight">
        <p class="truncate text-body font-medium text-fg-1">{user.username}</p>
        {#if user.admin}<p class="text-caption text-fg-3">Admin</p>{/if}
      </div>
      <IconButton icon={LogOut} label="Sign out" onclick={onSignOut} />
    </div>
  {/if}
{/snippet}

{#snippet chatStatus()}
  <ChatStatus {connected} {status} />
{/snippet}

<div class="flex h-dvh">
  <Rail view={areaView} collapsed={sideCollapsed} {attention} onToggleSidebar={toggleSidebar} onSelect={navigateArea} />
  <aside data-testid="sidebar" inert={sideCollapsed || view === 'memory'} aria-hidden={sideCollapsed || view === 'memory'} class="motion-emphasis flex shrink-0 flex-col overflow-hidden border-r bg-sidebar transition-[width] max-md:hidden {sideCollapsed || view === 'memory' ? 'w-0 border-transparent' : 'w-64'}">
    <div class="flex min-h-0 w-64 flex-1 flex-col gap-3 p-3" aria-hidden={sideCollapsed}>
      {#key view}
        {@render rail2(false)}
      {/key}
      {@render accountFooter()}
    </div>
  </aside>
  <main class="flex min-w-0 flex-1 flex-col">
    {#if view === 'settings'}
      {#if route.section}
        {#key route.section}
          <SettingsPage section={route.section} onBack={() => navigate('/settings')} admin={!!user?.admin} />
        {/key}
      {:else}
        <SettingsHub onNavigate={navigate} />
      {/if}
    {:else if view === 'quota'}
      <QuotaPage onBack={() => navigate('/')} onNavigate={navigate} />
    {:else if view === 'providers'}
      <ProvidersPage admin={!!user?.admin} />
    {:else if view === 'memory'}
      <MemoryPage userId={user?.id ?? 'default'} onBack={() => navigate('/')} />
    {:else if view === 'dashboard'}
      <DashboardPage {projects} onNavigate={navigate} />
    {:else if view === 'project-settings'}
      <ProjectSettingsPage
        project={scopedProject}
        loading={!projectsLoaded}
        onBack={() => navigate(buildPath({ name: 'project', projectId: scopedProjectRouteId }))}
        onRename={renameProjectRecord}
        onSaveGist={saveProjectGist}
        onRemove={removeFromSettings}
      />
    {:else if view === 'runs'}
      <RunsView
        workDir={isProjectScoped ? (scopedProject?.path ?? '') : (meta?.workDir ?? '')}
        projectId={scopedProjectRouteId}
        {runId}
        {ticketId}
        onOpenSession={goSession}
        onNavigate={navigate}
        onOpenMenu={() => (menuOpen = true)}
        scopeLabel={isProjectScoped ? (scopedProject?.name ?? (scopeProjectId === '' ? 'No project' : 'Project not found')) : 'All projects'}
      />
    {:else}
    <div class="px-3 py-1.5 sm:px-4">
      <PageHeader
        size="compact"
        title={meta?.title || activeProject?.name || (drafting ? 'New chat' : 'No Session')}
        scope={meta?.title ? (activeProject?.name ?? '') : ''}
        scopeIcon={icons.project}
        subtitle={[creatorName ? `created by ${creatorName}` : '', meta ? `${meta.harness} · ${modelLabel(meta.harness, meta.model)}${meta.effort ? ` · ${meta.effort}` : ''}` : drafting ? `${draftHarness} · ${modelLabel(draftHarness, draftModel)}${(draftHarness === 'claude' || draftHarness === 'pig') && draftEffort !== 'default' ? ` · ${draftEffort}` : ''}` : ''].filter(Boolean).join(' · ')}
        titleAttrs={{ 'data-testid': 'header-title' }}
        scopeAttrs={{ class: 'shrink-[2] max-sm:hidden' }}
        meta={chatStatus}
      >
        {#snippet actions()}
          {#if currentId && !drafting && !sessionMissing}
            <ContextMeter model={modelLabel(meta?.harness, meta?.model ?? '')} {usage} onCompress={canCompress ? () => { compressError = ''; compressOpen = true; } : null} busy={status === 'working'} {compressing} {compressNote} />
          {/if}
          <IconButton icon={Plus} label="New Session" class="md:hidden" onclick={newChat} />
          {#if activeProject}
            <span class="relative inline-flex">
              <IconButton
                icon={icons.git}
                label={git?.isRepo ? `Git: ${git.branch}${git.files.length ? `, ${git.files.length} changed` : ''}` : 'Git status'}
                id="git-toggle"
                variant={gitOpen ? 'secondary' : 'ghost'}
                aria-pressed={gitOpen}
                onclick={() => (gitOpen = !gitOpen)}
              />
              {#if git?.isRepo && git.files.length}
                <span class="pointer-events-none absolute right-0.5 top-0.5 size-2 rounded-full bg-status-needs-you ring-2 ring-background" data-testid="git-dirty" aria-hidden="true"></span>
              {/if}
            </span>
          {/if}
        {/snippet}
      </PageHeader>
    </div>

    <DiskBanner status={disk} />

    {#if loadError}
      <ErrorState compact title={loadError} class="mx-3 animate-rise sm:mx-4" />
    {/if}

    <div class="flex min-h-0 flex-1">
      <div class="relative flex min-w-0 flex-1 flex-col">
        {#if sessionMissing}
          <EmptyState icon={icons.session} title="Session not found" description="It was deleted or the link is wrong. Pick another chat." class="mx-auto w-full max-w-3xl py-16" data-testid="session-missing">
            {#snippet action()}
              <Button variant="secondary" onclick={() => navigate('/')}>Back to chats</Button>
            {/snippet}
          </EmptyState>
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
          reports={reportState.reports}
          dismissedReports={reportState.dismissed}
          onDismissReport={(seq) => changeReportDismissal(seq, true)}
          onRestoreReport={(seq) => changeReportDismissal(seq, false)}
        />
        {/if}
        <div bind:clientHeight={overlayH} class="pointer-events-none absolute inset-x-0 bottom-0 bg-gradient-to-t from-background via-background/90 to-transparent px-3 pb-[max(1.25rem,env(safe-area-inset-bottom))] pt-10 sm:px-4">
          <div class="pointer-events-auto mx-auto flex w-full max-w-3xl flex-col gap-2">
            {#if currentId && meta?.compressBlock}
              {@const bannerFor = currentId}
              {#key bannerFor}
                <CompressBanner sessionId={bannerFor} onChangeModel={() => navigate('/settings/agents')} onDismissed={() => clearCompressBlock(bannerFor)} />
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
            {#if failedDraftPrompt?.sessionId === currentId}
              <div role="alert" data-testid="failed-draft-prompt" class="animate-rise flex flex-wrap items-center gap-2 rounded-2xl border bg-card p-3 text-sm">
                <span class="min-w-0 flex-1">The first prompt was not sent.</span>
                <Button variant="outline" disabled={sending} onclick={retryFailedDraftPrompt}>Retry first prompt</Button>
              </div>
            {/if}
            {#if drafting}
              <DraftBar bind:harness={draftHarness} bind:model={draftModel} bind:effort={draftEffort} />
            {#if draftPick}
              <div role="group" aria-label="Project suggestion" data-testid="project-pick-banner" class="animate-rise flex flex-wrap items-center gap-2 rounded-2xl border bg-card p-3">
                <p class="min-w-0 flex-1 text-sm">Run this in <strong>{draftPick.project.name}</strong>?</p>
                <Button variant="outline" disabled={sending} onclick={() => resolveDraftPick(false)}>No</Button>
                <Button disabled={sending} onclick={() => resolveDraftPick(true)}>Yes</Button>
              </div>
            {/if}
              <Composer status="idle" busy={sending || !!draftPick} onSend={send} />
            {:else if currentId && resumable && !meta?.scout}
              <Composer {status} busy={sending} sessionId={currentId} activity={activeTool(blocks)} onSend={send} onStop={() => interruptSession(currentId)} />
            {/if}
            {/if}
          </div>
        </div>
      </div>

      <div class="flex min-h-0 p-3 pl-0 max-lg:contents">
        {#if gitOpen && activeProject}
          <GitPanel projectId={activeProject.id} {git} onClose={() => (gitOpen = false)} onRefresh={loadGit} />
        {/if}
      </div>
    </div>
    {/if}
  </main>
</div>

<Sheet bind:open={menuOpen} title={view === 'runs' ? 'Specs and Runs' : view === 'settings' ? 'Settings' : view === 'project-settings' ? 'Project settings' : view === 'dashboard' ? 'All projects' : view === 'quota' || view === 'providers' ? 'Quota and Providers' : view === 'memory' ? 'Memory' : 'Sessions'}>
  <div class="flex shrink-0 items-center gap-2" data-testid="drawer-head">
    {#if user}
      <span class="grid size-8 shrink-0 place-items-center rounded-full bg-muted text-caption font-semibold uppercase" aria-hidden="true">{user.username.slice(0, 1)}</span>
      <div class="min-w-0 flex-1 leading-tight" data-testid="account">
        <p class="truncate text-body font-medium text-fg-1">{user.username}</p>
        {#if user.admin}<p class="text-caption text-fg-3">Admin</p>{/if}
      </div>
      <IconButton icon={LogOut} label="Sign out" onclick={onSignOut} />
    {:else}
      <span class="flex-1"></span>
    {/if}
    <IconButton icon={icons.dismiss} label="Close menu" onclick={() => (menuOpen = false)} />
  </div>
  <!-- One scroll region for the contextual lists: nothing nests a second scroller. -->
  <div class="-mx-1 min-h-0 flex-1 overflow-y-auto overscroll-contain px-1" data-testid="drawer-scroll">
    {#key view}
      {@render rail2(true)}
    {/key}
  </div>
  <DrawerNav view={areaView} {attention} onSelect={navigateArea} />
  <VersionBadge inline />
</Sheet>
<CompressDialog open={compressOpen} busy={compressBusy} error={compressError} {usage} onClose={() => (compressOpen = false)} onCompress={runCompress} />
<AddProjectDialog open={addOpen} busy={adding} error={addError} onClose={() => (addOpen = false)} onAdd={add} />
<VersionBadge panelOpen={gitOpen} connected={connected} />
