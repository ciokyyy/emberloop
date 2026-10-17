<!-- Live tool strip: one card per burst of consecutive tool calls. Collapsed by
  default to one summary line: status, one icon per tool kind, the current tool
  (or "N tools · 4.3s" once settled) and the timer. Expanding opens the stage
  that swaps its output as tools start, and the history drawer that keeps every
  previous tool newest-first. A failed strip starts open so the error shows. -->
<script>
  import { untrack } from 'svelte';
  import {
    LoaderCircle,
    CircleCheck,
    CircleX,
    ChevronDown,
    ChevronRight,
    FileText,
    FilePlus,
    Search,
    Pencil,
    Terminal,
    History,
    Wrench,
    Brain,
  } from '@lucide/svelte';
  import Markdown from './Markdown.svelte';
  import {
    highlightCode,
    highlightJson,
    highlightDiff,
    highlightShell,
    highlightOutput,
    highlightGrepLine,
    highlightFileLine,
    diffLineKind,
    looksJson,
  } from './highlight.js';
  import {
    STRIP_MAX_HEIGHT,
    formatElapsed,
    stripSummary,
    toolTarget,
    meaningfulInput,
    splitOutputLines,
    outputKind,
    toolIcon,
    kindSummary,
    prefersReducedMotion,
    memoSegs,
  } from './toolstrip.js';

  let { block } = $props();

  const glyphs = { read: FileText, grep: Search, edit: Pencil, write: FilePlus, bash: Terminal, tool: Wrench, thinking: Brain };

  const runs = $derived(block.runs ?? []);
  const settled = $derived(!!block.settled);
  const current = $derived(runs.length ? runs[runs.length - 1] : null);
  const failed = $derived(String(current?.status ?? '').toLowerCase() === 'failed');
  const isDone = (s) => ['completed', 'complete', 'failed'].includes(String(s ?? '').toLowerCase());
  const settledDone = $derived(isDone(current?.status));
  const doneCount = $derived(runs.filter((r) => isDone(r.status)).length);
  const progress = $derived(runs.length ? Math.round((doneCount / runs.length) * 100) : 0);
  const summary = $derived(stripSummary(runs.filter((r) => !r.thinking).length || runs.length, block.elapsedMs ?? 0));
  const reduced = prefersReducedMotion();
  // Previous tools newest-first. A settled strip lists every run; a live one
  // keeps the current run on the stage.
  const historyList = $derived((settled ? [...runs] : runs.slice(0, -1)).reverse());
  // Live and still on its current run: output streams in per delta. While
  // live, lines render without their entrance animation (it would never
  // complete at stream rate) and only the tail window renders; settled and
  // history rows keep every line with the mount animation.
  const streamLive = $derived(!settled && !settledDone);
  const TAIL_LINES = 200;
  // Collapsed by default. A strip that is already failed opens so the error shows.
  let histOpen = $state(untrack(() => String(runs[runs.length - 1]?.status ?? '').toLowerCase() === 'failed'));
  const KIND_MAX = 4;
  const kinds = $derived(kindSummary(runs));
  const shownKinds = $derived(kinds.slice(0, KIND_MAX));
  const moreKinds = $derived(Math.max(0, kinds.length - KIND_MAX));
  let expanded = $state({});
  const toggleHist = () => (histOpen = !histOpen);
  const toggleRow = (key) => (expanded = { ...expanded, [key]: !expanded[key] });

  // Live elapsed timer: the settled summary stays frozen on its total.
  let tick = $state(0);
  $effect(() => {
    if (settled || settledDone) return;
    const t = setInterval(() => (tick += 1), 500);
    return () => clearInterval(t);
  });
  const elapsed = $derived(formatElapsed((block.elapsedMs ?? 0) + (settled || settledDone ? 0 : tick * 500)));

  const HeadGlyph = $derived(glyphs[toolIcon(current?.name)] ?? Wrench);
  let prevRun = $state(null);
  let curKey = $state(undefined);
  $effect(() => {
    const k = current?.key ?? null;
    if (curKey === undefined) {
      curKey = k;
      return;
    }
    if (k === curKey) return;
    const old = runs.find((r) => r.key === curKey) ?? null;
    curKey = k;
    if (reduced || !old) {
      prevRun = null;
      return;
    }
    prevRun = old;
    const t = setTimeout(() => (prevRun = null), 400);
    return () => clearTimeout(t);
  });

  // The stage height eases to the content height (capped, then the scroll
  // box takes over). Unmeasured (tests, first paint) it stays automatic so
  // content is never hidden behind a zero height.
  let scrollEl = $state(null);
  let stageH = $state(0);
  let stageT = 0;
  let stageKey = null;
  $effect(() => {
    const out = current?.output;
    const key = current?.key;
    const live = streamLive;
    void out;
    void key;
    if (reduced || !scrollEl) {
      stageH = 0;
      return;
    }
    // Height writes per delta re-layout the page per flush: while live, move
    // it only when the tool switched, the content moved enough, or enough
    // time passed; settled runs always snap.
    const h = Math.min(scrollEl.scrollHeight || 0, STRIP_MAX_HEIGHT);
    const now = typeof performance !== 'undefined' ? performance.now() : 0;
    const switched = key !== stageKey;
    stageKey = key;
    if (!live || switched || h === 0 || Math.abs(h - untrack(() => stageH)) > 48 || now - stageT > 300) {
      stageH = h > 0 ? h : 0;
      stageT = now;
    }
  });
  const wrapStyle = $derived(
    `${reduced ? 'transition: none;' : 'transition: height .5s cubic-bezier(.22,1,.36,1);'}${stageH > 0 ? `height: ${stageH}px;` : ''}`,
  );

  // Streaming output stays pinned to the newest line.
  $effect(() => {
    void current?.output;
    if (scrollEl) scrollEl.scrollTop = scrollEl.scrollHeight;
  });
</script>

{#snippet statusIcon(status, cls)}
  {@const s = String(status ?? '').toLowerCase()}
  {#if s === 'failed'}
    <CircleX class={cls} />
  {:else if s === 'completed' || s === 'complete'}
    <CircleCheck class={cls} />
  {:else}
    <LoaderCircle class="{cls} animate-spin" />
  {/if}
{/snippet}

{#snippet tokens(segs, line)}{#if line === ''}{' '}{:else}{#each segs as sg}{#if sg.cls}<span class={sg.cls}>{sg.text}</span>{:else}{sg.text}{/if}{/each}{/if}{/snippet}

{#snippet runOutput(run)}
  {#if run?.thinking}
    <Markdown text={run.output} class="text-sm text-muted-foreground" />
  {:else}
  {@const kind = outputKind(run?.name)}
  {@const full = splitOutputLines(run?.output || meaningfulInput(run?.input))}
  {@const empty = full.length === 1 && full[0] === ''}
  {@const isLive = streamLive && run?.key === current?.key}
  {@const start = isLive && full.length > TAIL_LINES ? full.length - TAIL_LINES : 0}
  {@const lines = start ? full.slice(start) : full}
  {#if empty}
    {#if run}
      {@const live = run.key === current?.key && !settled && !isDone(run.status)}
      {#if live}
        <div class="ts-shimmer" data-testid="toolstrip-shimmer"></div>
        <div class="ts-line mt-1 font-mono text-xs leading-5 text-muted-foreground">Writing tool call…</div>
      {:else}
        <div class="ts-line font-mono text-xs leading-5 text-muted-foreground">No output.</div>
      {/if}
    {/if}
  {:else if looksJson(full) && !run?.output}
    {#each lines as line, i (i + start)}
      <div class="ts-line font-mono text-xs leading-5 whitespace-pre-wrap">{@render tokens(memoSegs('json', line, highlightJson), line)}</div>
    {/each}
  {:else if kind === 'diff'}
    {#if start > 0}<div class="ts-tail font-mono text-xs leading-5 text-muted-foreground">+{start} earlier lines</div>{/if}
    {#each lines as line, i (i + start)}
      {@const dk = diffLineKind(line)}
      <div class="ts-line rounded font-mono text-xs leading-5 whitespace-pre-wrap {dk === 'add' ? 'diff-add' : dk === 'del' ? 'diff-del' : dk === 'hunk' ? 'diff-hunk' : ''}">
        {@render tokens(memoSegs('diff', line, highlightDiff), line)}
      </div>
    {/each}
  {:else if kind === 'stream'}
    {#if toolTarget(run)}
      <div class="ts-line font-mono text-xs leading-5 whitespace-pre-wrap"><span class="tk-ok">$</span> {@render tokens(memoSegs('shell', toolTarget(run), highlightShell), toolTarget(run))}</div>
    {/if}
    {#if start > 0}<div class="ts-tail font-mono text-xs leading-5 text-muted-foreground">+{start} earlier lines</div>{/if}
    {#each lines as line, i (i + start)}
      <div class="ts-line font-mono text-xs leading-5 whitespace-pre-wrap">{@render tokens(memoSegs('out', line, highlightOutput), line)}</div>
    {/each}
  {:else if kind === 'file' && toolIcon(run?.name) === 'grep'}
    {#if start > 0}<div class="ts-tail font-mono text-xs leading-5 text-muted-foreground">+{start} earlier lines</div>{/if}
    {#each lines as line, i (i + start)}
      <div class="ts-line font-mono text-xs leading-5 whitespace-pre-wrap">{@render tokens(memoSegs('grep', line, highlightGrepLine), line)}</div>
    {/each}
  {:else if kind === 'file'}
    {#if start > 0}<div class="ts-tail font-mono text-xs leading-5 text-muted-foreground">+{start} earlier lines</div>{/if}
    {#each lines as line, i (i + start)}
      <div class="ts-line font-mono text-xs leading-5 whitespace-pre-wrap">{@render tokens(memoSegs('file', line, highlightFileLine), line)}</div>
    {/each}
  {:else}
    {#if start > 0}<div class="ts-tail font-mono text-xs leading-5 text-muted-foreground">+{start} earlier lines</div>{/if}
    {#each lines as line, i (i + start)}
      <div class="ts-line font-mono text-xs leading-5 whitespace-pre-wrap">{@render tokens(memoSegs('out', line, highlightOutput), line)}</div>
    {/each}
  {/if}
  {/if}
{/snippet}

<div
  class="animate-rise overflow-hidden rounded-2xl border bg-card/80 text-card-foreground {failed ? 'border-destructive/40' : ''}"
  data-card="toolstrip"
  data-status={failed ? 'failed' : settledDone ? 'done' : 'running'}
  data-motion={reduced ? 'off' : 'on'}
>
  <!-- summary line: always visible, the whole line toggles the details -->
  <button
    type="button"
    class="motion-standard flex min-h-11 w-full min-w-0 items-center gap-2 px-3 py-2 text-left outline-none hover:bg-surface-3/50 focus-visible:ring-[3px] focus-visible:ring-ring/40 focus-visible:ring-inset md:min-h-10"
    onclick={toggleHist}
    aria-expanded={histOpen}
    aria-label={histOpen ? 'Hide tools' : 'Show tools'}
    data-testid="toolstrip-history-toggle"
  >
    <span class="grid size-5 shrink-0 place-items-center" data-testid="toolstrip-status">
      {@render statusIcon(current?.status, failed ? 'size-4 text-status-failed' : settledDone ? 'size-4 text-status-done' : 'size-4 text-status-working')}
    </span>
    <span class="flex shrink-0 items-center gap-1 text-fg-3" data-testid="toolstrip-kinds" aria-hidden="true">
      {#each shownKinds as k (k.kind)}
        {@const KindGlyph = glyphs[k.kind] ?? Wrench}
        <KindGlyph class="size-3.5" data-kind={k.kind} />
      {/each}
      {#if moreKinds}<span class="text-caption tabular-nums">+{moreKinds}</span>{/if}
    </span>
    <span class="sr-only">{kinds.map((k) => k.kind).join(', ')}</span>
    {#if settled}
      <span class="min-w-0 flex-1 truncate text-body text-fg-2" data-testid="toolstrip-summary">{summary}</span>
    {:else}
      <span class="min-w-0 truncate text-body font-medium">{current?.thinking ? 'Thinking' : (current?.name ?? 'tools')}</span>
      {#if current?.server}
        <span class="shrink-0 rounded-full bg-muted px-2 py-0.5 text-caption text-fg-3">{current.server}</span>
      {/if}
      {#if toolTarget(current)}
        <span class="min-w-0 flex-1 truncate font-mono text-caption text-fg-3 max-sm:hidden">{toolTarget(current)}</span>
      {:else}
        <span class="flex-1"></span>
      {/if}
    {/if}
    <span class="ml-auto flex shrink-0 items-center gap-2">
      <span class="font-mono text-caption tabular-nums text-fg-3" data-testid="toolstrip-elapsed">{elapsed}</span>
      <span class={settled ? 'sr-only' : 'flex items-center gap-1 text-caption tabular-nums text-fg-3'}>
        {#if !settled}<History class="size-3.5" aria-hidden="true" />{/if}<span data-testid="toolstrip-count">{historyList.length}</span>
      </span>
      <ChevronDown class="motion-standard size-4 shrink-0 text-fg-3 transition-transform" style={histOpen ? 'transform: rotate(180deg);' : ''} aria-hidden="true" />
    </span>
  </button>
  {#if !settled}
    <!-- progress hairline -->
    <div class="h-px bg-border"><div class="motion-emphasis h-px bg-status-working transition-[width]" style="width: {progress}%;" data-testid="toolstrip-progress"></div></div>
  {/if}

  {#if !settled && current}
    <!-- the swapping stage: collapsed with the strip, still mounted so streaming keeps its place -->
    <div class="ts-collapse {histOpen ? 'open' : ''}" inert={!histOpen} data-open={histOpen ? 'true' : 'false'}>
      <div>
        <div class="ts-stage" style={wrapStyle} data-live={streamLive ? 'true' : undefined}>
          <div
            bind:this={scrollEl}
            class="ts-scroll border-t bg-background/40 p-3.5"
            style="max-height: {STRIP_MAX_HEIGHT}px;"
            data-testid="toolstrip-stage"
            data-kind={outputKind(current.name)}
            role={failed ? 'alert' : undefined}
          >
            {#if prevRun && !reduced}
              <div class="ts-prev">{@render runOutput(prevRun)}</div>
            {/if}
            {#key current.key}
              <div class={reduced ? undefined : 'ts-cur'}>{@render runOutput(current)}</div>
            {/key}
          </div>
        </div>
      </div>
    </div>
  {/if}

  <!-- history: every previous tool, newest first; each row opens its output -->
  <div class="ts-history {histOpen ? 'open' : ''}" inert={!histOpen} data-testid="toolstrip-history" data-open={histOpen ? 'true' : 'false'}>
    <div>
      <div class="space-y-1 border-t bg-background/40 p-2">
        {#each historyList as run (run.key)}
          {@const RowGlyph = glyphs[toolIcon(run.name)] ?? Wrench}
          {@const rowStatus = String(run.status ?? '').toLowerCase()}
          <div data-testid="toolstrip-history-row">
            <button
              type="button"
              class="motion-standard flex min-h-11 w-full items-center gap-2.5 rounded-xl px-2.5 py-2 text-left text-body outline-none hover:bg-surface-3/50 focus-visible:ring-[3px] focus-visible:ring-ring/40 md:min-h-9"
              onclick={() => toggleRow(run.key)}
              aria-expanded={!!expanded[run.key]}
            >
              {@render statusIcon(run.status, rowStatus === 'failed' ? 'size-3.5 shrink-0 text-status-failed' : isDone(run.status) ? 'size-3.5 shrink-0 text-status-done' : 'size-3.5 shrink-0 text-status-working')}
              <span class="inline-flex min-w-0 items-center gap-1.5 font-medium">
                <RowGlyph class="size-3.5 shrink-0 text-fg-3" aria-hidden="true" />
                <span class="truncate">{run.thinking ? 'Thinking' : run.name}</span>
              </span>
              {#if toolTarget(run)}
                <span class="min-w-0 flex-1 truncate font-mono text-caption text-fg-3 max-sm:hidden">{toolTarget(run)}</span>
              {:else}
                <span class="flex-1"></span>
              {/if}
              <ChevronRight class="motion-standard ml-auto size-3.5 shrink-0 text-fg-3 transition-transform" style={expanded[run.key] ? 'transform: rotate(90deg);' : ''} aria-hidden="true" />
            </button>
            {#if expanded[run.key]}
              <div class="px-2.5 pb-2">
                <div class="ts-saved overflow-auto rounded-xl bg-background/60 p-3" style="max-height: {STRIP_MAX_HEIGHT}px;">{@render runOutput(run)}</div>
              </div>
            {/if}
          </div>
        {:else}
          <div class="px-2.5 py-2 text-caption text-fg-3">No previous tools yet.</div>
        {/each}
      </div>
    </div>
  </div>
</div>

<style>
  .ts-stage {
    position: relative;
    overflow: hidden;
  }
  .ts-scroll {
    overflow: auto;
  }
  .ts-prev {
    position: absolute;
    inset: 0 0 auto 0;
    pointer-events: none;
    animation: ts-out 0.35s cubic-bezier(0.22, 1, 0.36, 1) both;
  }
  .ts-cur {
    animation: ts-in 0.5s cubic-bezier(0.22, 1, 0.36, 1) both;
  }
  @keyframes ts-in {
    from {
      opacity: 0;
      transform: translateY(14px);
      filter: blur(4px);
    }
    to {
      opacity: 1;
      transform: none;
      filter: none;
    }
  }
  @keyframes ts-out {
    from {
      opacity: 1;
      transform: none;
    }
    to {
      opacity: 0;
      transform: translateY(-14px);
      filter: blur(4px);
    }
  }
  .ts-line {
    animation: ts-line-in 0.25s cubic-bezier(0.22, 1, 0.36, 1) both;
  }
  /* While live, per-line entrances would restart every delta and never
     complete: lines appear at once, and the tool-switch swap keeps the
     mount-only stage animation. */
  .ts-stage[data-live='true'] .ts-line,
  .ts-stage[data-live='true'] .ts-tail {
    animation: none;
  }
  @keyframes ts-line-in {
    from {
      opacity: 0;
      transform: translateY(4px);
    }
    to {
      opacity: 1;
      transform: none;
    }
  }
  .ts-shimmer {
    height: 1rem;
    border-radius: 0.5rem;
    background: linear-gradient(90deg, transparent, rgba(255, 255, 255, 0.07), transparent);
    background-size: 200% 100%;
    animation: ts-shimmer 1.6s linear infinite;
  }
  @keyframes ts-shimmer {
    from {
      background-position: -200% 0;
    }
    to {
      background-position: 200% 0;
    }
  }
  .ts-collapse,
  .ts-history {
    display: grid;
    grid-template-rows: 0fr;
    transition:
      grid-template-rows var(--dur-emphasis) var(--ease-smooth),
      opacity var(--dur-standard) var(--ease-smooth);
    opacity: 0;
  }
  .ts-collapse.open,
  .ts-history.open {
    grid-template-rows: 1fr;
    opacity: 1;
  }
  .ts-collapse > div,
  .ts-history > div {
    overflow: hidden;
  }
  @media (prefers-reduced-motion: reduce) {
    .ts-prev,
    .ts-cur,
    .ts-line,
    .ts-shimmer {
      animation: none;
    }
    .ts-stage,
    .ts-collapse,
    .ts-history {
      transition: none;
    }
  }
</style>
