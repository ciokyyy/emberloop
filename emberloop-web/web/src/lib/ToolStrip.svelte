<!-- Live tool strip: one card per burst of consecutive tool calls. The header
  always shows the current tool; the stage below swaps its output as tools
  start, and the history drawer keeps every previous tool newest-first. -->
<script>
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
  } from '@lucide/svelte';
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
    prefersReducedMotion,
  } from './toolstrip.js';

  let { block } = $props();

  const glyphs = { read: FileText, grep: Search, edit: Pencil, write: FilePlus, bash: Terminal, tool: Wrench };

  const runs = $derived(block.runs ?? []);
  const settled = $derived(!!block.settled);
  const current = $derived(runs.length ? runs[runs.length - 1] : null);
  const failed = $derived(String(current?.status ?? '').toLowerCase() === 'failed');
  const isDone = (s) => ['completed', 'complete', 'failed'].includes(String(s ?? '').toLowerCase());
  const settledDone = $derived(isDone(current?.status));
  const doneCount = $derived(runs.filter((r) => isDone(r.status)).length);
  const progress = $derived(runs.length ? Math.round((doneCount / runs.length) * 100) : 0);
  const summary = $derived(stripSummary(runs.length, block.elapsedMs ?? 0));
  const reduced = prefersReducedMotion();
  // Previous tools newest-first. A settled strip lists every run; a live one
  // keeps the current run on the stage.
  const historyList = $derived((settled ? [...runs] : runs.slice(0, -1)).reverse());

  let histOpen = $state(false);
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
  $effect(() => {
    const out = current?.output;
    const key = current?.key;
    void out;
    void key;
    if (reduced || !scrollEl) {
      stageH = 0;
      return;
    }
    const h = Math.min(scrollEl.scrollHeight || 0, STRIP_MAX_HEIGHT);
    stageH = h > 0 ? h : 0;
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
  {@const kind = outputKind(run?.name)}
  {@const lines = splitOutputLines(run?.output || meaningfulInput(run?.input))}
  {@const empty = lines.length === 1 && lines[0] === ''}
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
  {:else if looksJson(lines) && !run?.output}
    {#each lines as line, i (i)}
      <div class="ts-line font-mono text-xs leading-5 whitespace-pre-wrap">{@render tokens(highlightJson(line), line)}</div>
    {/each}
  {:else if kind === 'diff'}
    {#each lines as line, i (i)}
      {@const dk = diffLineKind(line)}
      <div class="ts-line rounded font-mono text-xs leading-5 whitespace-pre-wrap {dk === 'add' ? 'diff-add' : dk === 'del' ? 'diff-del' : dk === 'hunk' ? 'diff-hunk' : ''}">
        {@render tokens(highlightDiff(line), line)}
      </div>
    {/each}
  {:else if kind === 'stream'}
    {#if toolTarget(run)}
      <div class="ts-line font-mono text-xs leading-5 whitespace-pre-wrap"><span class="tk-ok">$</span> {@render tokens(highlightShell(toolTarget(run)), toolTarget(run))}</div>
    {/if}
    {#each lines as line, i (i)}
      <div class="ts-line font-mono text-xs leading-5 whitespace-pre-wrap">{@render tokens(highlightOutput(line), line)}</div>
    {/each}
  {:else if kind === 'file' && toolIcon(run?.name) === 'grep'}
    {#each lines as line, i (i)}
      <div class="ts-line font-mono text-xs leading-5 whitespace-pre-wrap">{@render tokens(highlightGrepLine(line), line)}</div>
    {/each}
  {:else if kind === 'file'}
    {#each lines as line, i (i)}
      <div class="ts-line font-mono text-xs leading-5 whitespace-pre-wrap">{@render tokens(highlightFileLine(line), line)}</div>
    {/each}
  {:else}
    {#each lines as line, i (i)}
      <div class="ts-line font-mono text-xs leading-5 whitespace-pre-wrap">{@render tokens(highlightOutput(line), line)}</div>
    {/each}
  {/if}
{/snippet}

<div
  class="animate-rise overflow-hidden rounded-2xl border bg-card/80 text-card-foreground {failed ? 'border-destructive/40' : ''}"
  data-card="toolstrip"
  data-status={failed ? 'failed' : settledDone ? 'done' : 'running'}
  data-motion={reduced ? 'off' : 'on'}
>
  <!-- header: always visible -->
  <div class="flex items-center gap-2.5 px-3.5 py-2.5">
    <span class="grid size-5 shrink-0 place-items-center" data-testid="toolstrip-status">
      {@render statusIcon(current?.status, failed ? 'size-4 text-destructive' : settledDone ? 'size-4 text-green-400' : 'size-4 text-ring')}
    </span>
    <span class="inline-flex min-w-0 items-center gap-1.5 font-medium">
      <HeadGlyph class="size-3.5 shrink-0 text-muted-foreground" />
      <span class="truncate">{current?.name ?? 'tools'}</span>
    </span>
    {#if current?.server}
      <span class="shrink-0 rounded-full bg-muted px-2 py-0.5 text-[10px] text-muted-foreground">{current.server}</span>
    {/if}
    {#if toolTarget(current)}
      <span class="ml-1 min-w-0 flex-1 truncate font-mono text-xs text-muted-foreground">{toolTarget(current)}</span>
    {:else}
      <span class="flex-1"></span>
    {/if}
    <div class="ml-auto flex shrink-0 items-center gap-2">
      <span class="font-mono text-[11px] text-muted-foreground" data-testid="toolstrip-elapsed">{elapsed}</span>
      <button
        class="flex items-center gap-1.5 rounded-full bg-muted px-2.5 py-1 text-xs text-muted-foreground transition hover:bg-accent hover:text-foreground"
        onclick={toggleHist}
        aria-expanded={histOpen}
        aria-label="Tool history"
        data-testid="toolstrip-history-toggle"
      >
        <History class="size-3.5" />
        <span data-testid="toolstrip-count">{historyList.length}</span>
        <ChevronDown class="size-3.5 transition-transform duration-300" style={histOpen ? 'transform: rotate(180deg);' : ''} />
      </button>
    </div>
  </div>
  <!-- progress hairline -->
  <div class="h-px bg-border"><div class="h-px bg-ring transition-[width] duration-500" style="width: {progress}%;" data-testid="toolstrip-progress"></div></div>

  {#if settled}
    <div class="border-t bg-background/40 px-3.5 py-2.5 text-sm text-muted-foreground" data-testid="toolstrip-summary">{summary}</div>
  {:else if current}
    <!-- the swapping stage -->
    <div class="ts-stage" style={wrapStyle}>
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
  {/if}

  <!-- history: every previous tool, newest first; each row opens its output -->
  <div class="ts-history {histOpen ? 'open' : ''}" data-testid="toolstrip-history" data-open={histOpen ? 'true' : 'false'}>
    <div>
      <div class="space-y-1 border-t bg-background/40 p-2">
        {#each historyList as run (run.key)}
          {@const RowGlyph = glyphs[toolIcon(run.name)] ?? Wrench}
          <div data-testid="toolstrip-history-row">
            <button
              class="flex w-full items-center gap-2.5 rounded-xl px-2.5 py-2 text-left text-sm hover:bg-accent/50"
              onclick={() => toggleRow(run.key)}
              aria-expanded={!!expanded[run.key]}
            >
              {@render statusIcon(run.status, isDone(run.status) && String(run.status).toLowerCase() !== 'failed' ? 'size-3.5 shrink-0 text-green-400' : String(run.status).toLowerCase() === 'failed' ? 'size-3.5 shrink-0 text-destructive' : 'size-3.5 shrink-0 text-muted-foreground')}
              <span class="inline-flex min-w-0 items-center gap-1.5 font-medium">
                <RowGlyph class="size-3.5 shrink-0 text-muted-foreground" />
                <span class="truncate">{run.name}</span>
              </span>
              {#if toolTarget(run)}
                <span class="min-w-0 flex-1 truncate font-mono text-xs text-muted-foreground">{toolTarget(run)}</span>
              {:else}
                <span class="flex-1"></span>
              {/if}
              <ChevronRight class="ml-auto size-3.5 shrink-0 text-muted-foreground transition-transform" style={expanded[run.key] ? 'transform: rotate(90deg);' : ''} />
            </button>
            {#if expanded[run.key]}
              <div class="px-2.5 pb-2">
                <div class="ts-saved overflow-auto rounded-xl bg-background/60 p-3" style="max-height: {STRIP_MAX_HEIGHT}px;">{@render runOutput(run)}</div>
              </div>
            {/if}
          </div>
        {:else}
          <div class="px-2.5 py-2 text-xs text-muted-foreground">No previous tools yet.</div>
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
  .ts-history {
    display: grid;
    grid-template-rows: 0fr;
    transition:
      grid-template-rows 0.45s cubic-bezier(0.22, 1, 0.36, 1),
      opacity 0.3s;
    opacity: 0;
  }
  .ts-history.open {
    grid-template-rows: 1fr;
    opacity: 1;
  }
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
    .ts-history {
      transition: none;
    }
  }
</style>
