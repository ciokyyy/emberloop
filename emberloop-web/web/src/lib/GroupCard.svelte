<!-- Group header card: one spawn call's Subagents as live rows. The header
  counts done/running live; each row is the named Subagent card (name,
  harness badge, type chip, latest-tool line, status dot). Click opens the
  child transcript. -->
<script>
  import { onDestroy } from 'svelte';
  import { untrack } from 'svelte';
  import Badge from './ui/badge.svelte';
  import { harnessStyle } from './harnesses.js';
  import { childState, groupCounts, latestTool } from './subagents.js';
  import { getSession, openStream } from './api.js';

  let { block, sessions = [], onOpenSession = () => {} } = $props();

  const metaById = $derived(new Map((sessions ?? []).map((s) => [s.id, s])));
  let live = $state({});
  const streams = new Map();
  const fetched = new Set();

  onDestroy(() => {
    for (const s of streams.values()) s?.close?.();
    streams.clear();
  });

  function setKid(id, patch) {
    const cur = untrack(() => live[id]) ?? {};
    const next = { ...cur, ...patch };
    if (Object.keys(patch).every((k) => cur[k] === next[k])) return;
    live[id] = next;
  }

  async function loadChild(id) {
    try {
      const body = await getSession(id);
      const evs = body.events ?? [];
      const meta = body.session ?? {};
      setKid(id, {
        name: meta.name || untrack(() => live[id]?.name) || 'Subagent',
        harness: meta.harness || untrack(() => live[id]?.harness) || '',
        scout: meta.scout ?? untrack(() => live[id]?.scout) ?? false,
        status: meta.status || 'idle',
        tool: latestTool(evs),
        childEvents: evs.slice(-200),
      });
      if (!streams.has(id)) {
        const since = evs.reduce((m, e) => Math.max(m, e.seq ?? 0), 0);
        streams.set(id, openStream(id, since, (e) => onChildEvent(id, e)));
      }
    } catch {
      // A child that never loads keeps its spawn snapshot.
    }
  }

  function onChildEvent(id, e) {
    const cur = untrack(() => live[id]) ?? {};
    const evs = [...(cur.childEvents ?? []), e].slice(-200);
    const patch = { childEvents: evs, tool: latestTool(evs) };
    if (e.kind === 'status' && e.status) patch.status = e.status;
    setKid(id, patch);
  }

  $effect(() => {
    // Baseline from the spawn payload plus the Session list; live stream
    // state wins once it arrives (setKid keeps fresher fields).
    for (const m of block.members ?? []) {
      const meta = metaById.get(m.id) ?? {};
      setKid(m.id, {
        name: m.name || meta.name || untrack(() => live[m.id]?.name) || 'Subagent',
        harness: m.harness || meta.harness || untrack(() => live[m.id]?.harness) || '',
        scout: m.scout || meta.scout || untrack(() => live[m.id]?.scout) || false,
        status: untrack(() => live[m.id]?.status) ?? m.status ?? meta.status ?? 'idle',
        tool: untrack(() => live[m.id]?.tool) ?? '',
        childEvents: untrack(() => live[m.id]?.childEvents) ?? [],
      });
      if (!fetched.has(m.id)) {
        fetched.add(m.id);
        loadChild(m.id);
      }
    }
  });

  const kids = $derived(
    (block.members ?? []).map((m) => {
      const k = live[m.id] ?? {};
      const state = childState(k.status ?? 'idle', k.childEvents ?? []);
      return {
        id: m.id,
        name: k.name ?? m.name ?? 'Subagent',
        harness: k.harness ?? m.harness ?? '',
        scout: k.scout ?? m.scout ?? false,
        status: k.status ?? 'idle',
        state,
        tool: k.tool ?? '',
      };
    }),
  );
  const counts = $derived(groupCounts(kids.map((k) => k.state)));
  const short = (id) => String(id ?? '').slice(0, 8);

  const DOT = {
    done: 'bg-green-500',
    failed: 'bg-red-500',
    running: 'bg-blue-500 animate-pulse',
    idle: 'bg-zinc-400',
  };
  const STATE_LABEL = { done: 'done', failed: 'failed', running: 'running', idle: 'idle' };
  const STATE_TEXT = {
    done: 'text-green-500',
    failed: 'text-red-500',
    running: 'text-blue-500',
    idle: 'text-muted-foreground',
  };
</script>

<div
  class={`animate-rise overflow-hidden rounded-2xl border bg-card text-card-foreground transition-colors ${counts.allDone && !counts.anyFailed ? 'border-green-500/40' : ''} ${!counts.allDone && !counts.anyFailed ? 'border-dashed' : ''}`}
  data-card="group"
  data-group={block.groupId ?? ''}
>
  <div
    class={`flex items-center gap-2 px-3.5 py-2.5 text-sm ${counts.allDone && !counts.anyFailed ? 'bg-green-500/5' : ''} ${counts.anyFailed ? 'bg-red-500/5' : ''}`}
  >
    <span class="font-medium">{counts.total} subagent{counts.total === 1 ? '' : 's'}</span>
    <span class="min-w-0 flex-1 truncate text-xs text-muted-foreground">
      {counts.done} done · {counts.running} running{#if counts.failed} · {counts.failed} failed{/if}
    </span>
  </div>
  <div class="flex flex-col gap-1.5 border-t bg-background/40 p-2.5">
    {#each kids as k (k.id)}
      {@const style = harnessStyle(k.harness)}
      <button
        onclick={() => onOpenSession(k.id)}
        title={k.id}
        data-child={k.id}
        class="flex w-full items-center gap-2.5 rounded-xl border border-transparent bg-card px-3 py-2 text-left text-sm transition-colors hover:bg-muted/50"
        style={`border-left: 3px solid ${style.accent}`}
      >
        <span class="h-2 w-2 shrink-0 rounded-full {DOT[k.state] ?? DOT.idle}" title={k.state}></span>
        <span
          class="flex size-5 shrink-0 items-center justify-center rounded-md text-[11px] font-bold"
          style={`color: ${style.tint}; background: color-mix(in srgb, ${style.accent} 15%, transparent)`}
          title={style.label}
        >{style.mark}</span>
        <span class="min-w-0 flex-1 truncate font-medium">{k.name}</span>
        <Badge class="shrink-0 px-1.5 py-0 text-[10px]">{k.scout ? 'scout' : 'worker'}</Badge>
        {#if k.tool}
          <span class="hidden min-w-0 flex-1 truncate font-mono text-xs text-muted-foreground sm:block">{k.tool}</span>
        {/if}
        <span class="shrink-0 text-xs {STATE_TEXT[k.state] ?? STATE_TEXT.idle}">{STATE_LABEL[k.state] ?? k.state}</span>
        <span class="shrink-0 font-mono text-[10px] text-muted-foreground/60">{short(k.id)}</span>
      </button>
    {/each}
  </div>
</div>
