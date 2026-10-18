<!-- Runs dependency graph: dagre positions, SVG edges, HTML cards, pan/zoom.
     Inline, the graph never captures the page: vertical touch drags scroll the
     page (touch-action pan-y), wheel scrolls the page and zooms only with
     ctrl or cmd, and the zoom buttons stay one tap away. `fullscreen` is the
     phone opt-in: it fills a dialog that owns every gesture. -->
<script>
  // The ES build directly: the package main is a UMD file that Node and
  // vitest read as an empty module.
  import Panzoom from '@panzoom/panzoom/dist/panzoom.es.js';
  import { ZoomIn, ZoomOut, Maximize } from '@lucide/svelte';
  import IconButton from './ui/icon-button.svelte';
  import EmptyState from './ui/empty-state.svelte';
  import { icons } from './icons.js';
  import { layoutRunGraph, edgePath, countByStatus } from './runsGraph.js';

  let { tickets = [], selectedId = '', onSelect = null, fullscreen = false } = $props();

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
    const scale = Math.min(1, Math.max(0.2, Math.min((vw - FIT_PAD * 2) / graph.width, availH / graph.height)));
    const x = (vw - graph.width * scale) / 2;
    const y = FIT_TOP + Math.max(0, (availH - graph.height * scale) / 2);
    pz.zoom(scale, { animate, force: true });
    pz.pan(x / scale, y / scale, { animate, force: true });
  }

  const LEGEND = [
    { status: 'merged', dot: 'bg-status-done' },
    { status: 'running', dot: 'bg-status-working' },
    { status: 'paused', dot: 'bg-status-paused' },
    { status: 'needs-you', dot: 'bg-status-needs-you' },
    { status: 'blocked', dot: 'border border-fg-3' },
    { status: 'parked', dot: 'bg-fg-4' },
    { status: 'queued', dot: 'bg-fg-4/40' },
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
      minScale: 0.2,
      step: 0.2,
      cursor: 'grab',
      animate: true,
      duration: 220,
      easing: 'cubic-bezier(0.22, 1, 0.36, 1)',
      excludeClass: 'panzoom-exclude',
      // Inline: leave vertical touch drags to the page.
      touchAction: fullscreen ? 'none' : 'pan-y',
    });
    pz = instance;
    touched = false;
    const wheel = (e) => {
      // A plain wheel scrolls the page; a pinch on a trackpad arrives as ctrl+wheel.
      if (!fullscreen && !(e.ctrlKey || e.metaKey)) return;
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
    let c = 'panzoom-exclude motion-standard animate-rise absolute rounded-2xl border bg-card p-3 text-left shadow-elev-1 outline-none hover:border-fg-3 focus-visible:ring-[3px] focus-visible:ring-ring/40';
    if (n.status === 'needs-you') c += ' border-status-needs-you/40';
    if (n.status === 'blocked' || n.status === 'parked') c += ' bg-card/60 opacity-70';
    if (n.status === 'paused') c += ' border-status-paused/40';
    if (n.id === selectedId) c += ' ring-[3px] ring-ring/40';
    return c;
  }

  function labelClass(n) {
    if (n.status === 'merged') return 'text-status-done';
    if (n.status === 'running') return 'text-status-working';
    if (n.status === 'needs-you') return 'text-status-needs-you';
    if (n.status === 'paused') return 'text-status-paused';
    return 'text-fg-3';
  }
</script>

<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<div
  bind:this={viewport}
  data-testid="runs-graph"
  data-fullscreen={fullscreen ? '' : undefined}
  class="dots relative w-full min-w-0 cursor-grab overflow-hidden border active:cursor-grabbing {fullscreen ? 'min-h-0 flex-1 rounded-none border-x-0 border-b-0' : 'max-h-[70dvh] shrink-0 rounded-2xl'}"
  style={fullscreen ? undefined : `height:${Math.max(360, Math.min(graph.height + 120, 680))}px;`}
  onkeydown={onKey}
  tabindex="0"
  role="application"
  aria-label="Run dependency graph. Arrow keys pan, plus and minus zoom, zero resets."
>
  {#if !graph.nodes.length}
    <div class="absolute inset-0 grid place-items-center p-4">
      <EmptyState icon={icons.ticket} title="No tickets yet" description="Start a Run to lay out its Tickets." />
    </div>
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
          <div class="truncate text-caption font-medium {labelClass(n)}">
            {#if n.status === 'running'}<span class="mr-1 inline-block size-1.5 rounded-full bg-status-working motion-safe:animate-pulse"></span>{/if}{n.label}
          </div>
          <div class="mt-0.5 line-clamp-2 text-body leading-snug">{n.title ?? n.id}</div>
          {#if n.subtitle}
            <div class="mt-1 truncate text-caption {n.status === 'needs-you' ? 'text-status-needs-you' : 'text-fg-3'}">{n.subtitle}</div>
          {/if}
          {#if n.chain?.length}
            <div class="mt-1 space-y-0.5 text-caption text-fg-3" title={n.chain.map((entry) => `${entry.label} ${entry.position}/${entry.total}: ${entry.backend}/${entry.model}`).join('\n')} data-testid="graph-chain">
              {#each n.chain as entry (entry.role)}
                <div class="truncate">{entry.label} {entry.position}/{entry.total}: {entry.backend}/{entry.model}</div>
              {/each}
            </div>
          {/if}
        </button>
      {/each}
    </div>
  {/if}

  {#if graph.nodes.length}
    <div class="absolute left-3 top-3 z-10 flex max-w-[calc(100%-1.5rem)] flex-wrap gap-x-3 gap-y-1 rounded-2xl border bg-popover/80 px-3 py-2 text-caption backdrop-blur" data-testid="runs-legend">
      {#each LEGEND.filter((l) => counts[l.status]) as l (l.status)}
        <span class="tabular-nums"><i class="mr-1 inline-block size-2 rounded-full {l.dot}"></i>{counts[l.status]} {l.status}</span>
      {/each}
    </div>
  {/if}

  <div class="absolute bottom-3 right-3 flex gap-1 {fullscreen ? 'bottom-[max(0.75rem,env(safe-area-inset-bottom))]' : ''}">
    <IconButton variant="outline" icon={ZoomIn} label="Zoom in" class="bg-popover/80 backdrop-blur" onclick={() => zoomStep(1.2)} />
    <IconButton variant="outline" icon={ZoomOut} label="Zoom out" class="bg-popover/80 backdrop-blur" onclick={() => zoomStep(0.8)} />
    <IconButton variant="outline" icon={Maximize} label="Reset view" class="bg-popover/80 backdrop-blur" onclick={resetView} />
  </div>
</div>
