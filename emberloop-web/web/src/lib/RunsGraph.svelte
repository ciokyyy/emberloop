<!-- Runs dependency graph: dagre positions, SVG edges, HTML cards, pan/zoom. -->
<script>
  // The ES build directly: the package main is a UMD file that Node and
  // vitest read as an empty module.
  import Panzoom from '@panzoom/panzoom/dist/panzoom.es.js';
  import { ZoomIn, ZoomOut, Maximize } from '@lucide/svelte';
  import Button from './ui/button.svelte';
  import { layoutRunGraph, edgePath, countByStatus } from './runsGraph.js';

  let { tickets = [], selectedId = '', onSelect = null } = $props();

  let viewport = $state(null);
  let canvas = $state(null);
  let pz = null;
  // Positions derive from ids plus blocker lists only (see structureKey),
  // so an SSE status flip repaints cards in place without moving nodes.
  const graph = $derived(layoutRunGraph(tickets));
  const counts = $derived(countByStatus(graph.nodes));

  // Fit-to-view: the whole graph scales to fill the viewport (never above
  // 1:1) and centers below the legend strip. Fits again on a structure
  // change and on resize until the user pans or zooms by hand.
  const FIT_TOP = 52;
  const FIT_PAD = 16;
  let touched = false;
  function fit(animate = false) {
    const host = viewport;
    if (!pz || !host || !graph.width || !graph.height) return;
    const vw = host.clientWidth;
    const vh = host.clientHeight;
    if (!vw || !vh) {
      // No layout yet (hidden tab, test DOM): the plain 1:1 origin view.
      pz.reset({ animate: false });
      return;
    }
    const availH = Math.max(1, vh - FIT_TOP - FIT_PAD);
    const scale = Math.min(1, Math.max(0.4, Math.min((vw - FIT_PAD * 2) / graph.width, availH / graph.height)));
    const x = (vw - graph.width * scale) / 2;
    const y = FIT_TOP + Math.max(0, (availH - graph.height * scale) / 2);
    pz.zoom(scale, { animate, force: true });
    pz.pan(x / scale, y / scale, { animate, force: true });
  }

  const LEGEND = [
    { status: 'merged', dot: 'bg-green-500' },
    { status: 'running', dot: 'bg-blue-500' },
    { status: 'needs-you', dot: 'bg-amber-400' },
    { status: 'blocked', dot: 'border border-muted-foreground' },
    { status: 'parked', dot: 'bg-muted-foreground/50' },
    { status: 'queued', dot: 'bg-muted-foreground/30' },
  ];

  // Pan, wheel and pinch zoom come from @panzoom/panzoom; animate eases
  // every programmatic move (buttons, keys, reset).
  $effect(() => {
    const el = canvas;
    const host = viewport;
    if (!el || !host) return;
    const instance = Panzoom(el, {
      origin: '0 0',
      maxScale: 2.5,
      minScale: 0.4,
      step: 0.2,
      cursor: 'grab',
      animate: true,
      duration: 220,
      easing: 'cubic-bezier(0.22, 1, 0.36, 1)',
      excludeClass: 'panzoom-exclude',
    });
    pz = instance;
    touched = false;
    const wheel = (e) => {
      touched = true;
      instance.zoomWithWheel(e);
    };
    const drag = () => (touched = true);
    host.addEventListener('wheel', wheel, { passive: false });
    el.addEventListener('panzoomstart', drag);
    fit(false);
    const ro = typeof ResizeObserver === 'undefined' ? null : new ResizeObserver(() => !touched && fit(false));
    ro?.observe(host);
    return () => {
      ro?.disconnect();
      el.removeEventListener('panzoomstart', drag);
      host.removeEventListener('wheel', wheel);
      instance.destroy();
      pz = null;
    };
  });

  const zoomStep = (factor) => {
    touched = true;
    return factor > 1 ? pz?.zoomIn() : pz?.zoomOut();
  };
  const resetView = () => {
    touched = false;
    fit(true);
  };

  // A new structure (ticket added or blockers changed) refits; a status flip
  // keeps the view where the user left it.
  let fitKey = '';
  $effect(() => {
    const key = `${graph.width}x${graph.height}:${graph.nodes.length}`;
    if (key === fitKey) return;
    fitKey = key;
    touched = false;
    queueMicrotask(() => fit(false));
  });

  // Keyboard pan/zoom keeps the canvas reachable without a pointer: the
  // zoom buttons stay tabbable and arrows move the canvas when focused.
  function onKey(e) {
    const step = 40;
    const moves = { ArrowLeft: [step, 0], ArrowRight: [-step, 0], ArrowUp: [0, step], ArrowDown: [0, -step] };
    if (moves[e.key]) {
      touched = true;
      pz?.pan(...moves[e.key], { relative: true });
      e.preventDefault();
    } else if (e.key === '+' || e.key === '=') {
      pz?.zoomIn();
      e.preventDefault();
    } else if (e.key === '-') {
      pz?.zoomOut();
      e.preventDefault();
    } else if (e.key === '0') {
      resetView();
      e.preventDefault();
    }
  }

  function cardClass(n) {
    let c = 'panzoom-exclude animate-rise absolute rounded-2xl border bg-card p-3 text-left hover:border-muted-foreground';
    if (n.status === 'needs-you') c += ' border-amber-400/40';
    if (n.status === 'blocked' || n.status === 'parked') c += ' bg-card/60 opacity-70';
    if (n.id === selectedId) c += ' ring-[3px] ring-ring/40';
    return c;
  }

  function labelClass(n) {
    if (n.status === 'merged') return 'text-green-400';
    if (n.status === 'running') return 'text-blue-300';
    if (n.status === 'needs-you') return 'text-amber-300';
    return 'text-muted-foreground';
  }
</script>

<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<div
  bind:this={viewport}
  data-testid="runs-graph"
  class="dots relative w-full min-w-0 shrink-0 cursor-grab overflow-hidden rounded-2xl border active:cursor-grabbing"
  style="height:{Math.max(360, Math.min(graph.height + 120, 680))}px;"
  onkeydown={onKey}
  tabindex="0"
  role="application"
  aria-label="Run dependency graph. Arrow keys pan, plus and minus zoom, zero resets."
>
  {#if !graph.nodes.length}
    <p class="absolute inset-0 grid place-items-center text-sm text-muted-foreground">No tickets yet — start a Run.</p>
  {:else}
    <div
      bind:this={canvas}
      data-testid="runs-canvas"
      class="absolute left-0 top-0"
      style="width:{graph.width}px;height:{graph.height}px;"
    >
      <svg data-testid="runs-edges" class="absolute left-0 top-0" width={graph.width} height={graph.height} fill="none" stroke-width="1.5" aria-hidden="true">
        <g stroke="var(--input)">
          {#each graph.edges as e (e.from + '→' + e.to)}
            <path d={edgePath(e.points)} stroke-dasharray={graph.nodes.find((n) => n.id === e.to)?.status === 'blocked' ? '4 4' : undefined} />
          {/each}
        </g>
      </svg>
      {#each graph.nodes as n (n.id)}
        <button
          type="button"
          class={cardClass(n)}
          style="left:{n.x}px;top:{n.y}px;width:{n.width}px;height:{n.height}px;"
          aria-current={n.id === selectedId ? 'true' : undefined}
          aria-label={`Ticket ${n.title ?? n.id}, ${n.label}`}
          onclick={() => onSelect?.(n.ticket)}
        >
          <div class="truncate text-[10px] font-medium uppercase tracking-wide {labelClass(n)}">
            {#if n.status === 'running'}<span class="mr-1 inline-block size-1.5 animate-pulse rounded-full bg-blue-500"></span>{/if}{n.label}
          </div>
          <div class="mt-0.5 line-clamp-2 text-sm font-medium leading-snug">{n.title ?? n.id}</div>
          {#if n.subtitle}
            <div class="mt-1 truncate text-xs {n.status === 'needs-you' ? 'text-amber-200/70' : 'text-muted-foreground'}">{n.subtitle}</div>
          {/if}
        </button>
      {/each}
    </div>
  {/if}

  {#if graph.nodes.length}
    <div class="absolute left-4 top-3 z-10 flex gap-3 rounded-2xl border bg-popover/80 px-3.5 py-2 text-xs backdrop-blur" data-testid="runs-legend">
      {#each LEGEND.filter((l) => counts[l.status]) as l (l.status)}
        <span><i class="mr-1 inline-block size-2 rounded-full {l.dot}"></i>{counts[l.status]} {l.status}</span>
      {/each}
    </div>
  {/if}

  <div class="absolute bottom-3 right-4 flex gap-1">
    <Button variant="outline" size="icon-sm" class="bg-popover/80 backdrop-blur" aria-label="Zoom in" onclick={() => zoomStep(1.2)}><ZoomIn /></Button>
    <Button variant="outline" size="icon-sm" class="bg-popover/80 backdrop-blur" aria-label="Zoom out" onclick={() => zoomStep(0.8)}><ZoomOut /></Button>
    <Button variant="outline" size="icon-sm" class="bg-popover/80 backdrop-blur" aria-label="Reset view" onclick={resetView}><Maximize /></Button>
  </div>
</div>
