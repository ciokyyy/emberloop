<!-- Transcript card: one visual block per grouped Event run. -->
<script>
  import Markdown from './Markdown.svelte';
  import { CheckCircle2, XCircle, Loader, ChevronRight, Brain, Bot } from '@lucide/svelte';
  let { block } = $props();
  // One-line hint of what the tool was asked: first line of the input.
  const firstReply = $derived(String(block.members?.[0]?.reply ?? block.body ?? '').replace(/\s+/g, ' ').slice(0, 110));
  const preview = $derived(String(block.input ?? '').replace(/\s+/g, ' ').trim().slice(0, 120));
  const diffLines = $derived(block.card === 'diff' ? String(block.body ?? '').split('\n') : []);
</script>

{#if block.card === 'user'}
  <div class="flex animate-rise justify-end" data-card="user">
    <div class="max-w-[80%] rounded-3xl bg-muted px-4 py-2.5 text-[15px] leading-relaxed text-foreground">
      {#if block.title && block.title !== 'You'}
        <div class="mb-0.5 text-[11px] font-medium uppercase tracking-wide opacity-80">{block.title}</div>
      {/if}
      {block.body}
    </div>
  </div>
{:else if block.card === 'status'}
  <div class="flex animate-rise justify-center" data-card="status">
    <span class="rounded-full bg-muted px-3 py-0.5 text-xs text-muted-foreground">Session {block.title}</span>
  </div>
{:else if block.card === 'tool'}
  <details class="group animate-rise overflow-hidden rounded-2xl border bg-card text-card-foreground" data-card="tool">
    <summary class="flex cursor-pointer list-none items-center gap-2.5 px-3.5 py-2.5 text-sm transition-colors hover:bg-muted/50 [&::-webkit-details-marker]:hidden">
      <span class="flex size-5 shrink-0 items-center justify-center">
        {#if block.status === 'failed'}
          <XCircle class="size-4 text-destructive" />
        {:else if block.status === 'completed'}
          <CheckCircle2 class="size-4 text-green-400" />
        {:else}
          <Loader class="size-4 animate-spin text-muted-foreground" />
        {/if}
      </span>
      <span class="min-w-0 truncate font-medium">{block.title}</span>
      {#if block.server}
        <span class="shrink-0 rounded-md bg-muted px-1.5 py-0.5 text-[11px] text-muted-foreground">{block.server}</span>
      {/if}
      {#if preview}
        <span class="min-w-0 flex-1 truncate font-mono text-xs text-muted-foreground">{preview}</span>
      {:else}
        <span class="flex-1"></span>
      {/if}
      <ChevronRight class="size-4 shrink-0 text-muted-foreground transition-transform duration-200 group-open:rotate-90" />
    </summary>
    <div class="flex flex-col gap-3 border-t bg-background/40 p-3.5">
      {#if block.input}
        <div>
          <div class="mb-1 text-[11px] font-medium tracking-wide text-muted-foreground uppercase">Input</div>
          <pre class="max-h-56 overflow-auto rounded-xl bg-muted/60 p-3 font-mono text-xs leading-relaxed whitespace-pre-wrap">{block.input}</pre>
        </div>
      {/if}
      {#if block.output}
        <div>
          <div class="mb-1 text-[11px] font-medium tracking-wide text-muted-foreground uppercase">Output</div>
          <pre class="max-h-72 overflow-auto rounded-xl bg-muted/60 p-3 font-mono text-xs leading-relaxed whitespace-pre-wrap">{block.output}</pre>
        </div>
      {/if}
      {#if !block.input && !block.output}
        <pre class="max-h-72 overflow-auto rounded-xl bg-muted/60 p-3 font-mono text-xs whitespace-pre-wrap">{block.body || 'No content.'}</pre>
      {/if}
    </div>
  </details>
{:else if block.card === 'report'}
  <details class="group animate-rise overflow-hidden rounded-2xl border border-dashed bg-card text-card-foreground" data-card="report">
    <summary class="flex cursor-pointer list-none items-center gap-2.5 px-3.5 py-2.5 text-sm transition-colors hover:bg-muted/50 [&::-webkit-details-marker]:hidden">
      <Bot class="size-4 shrink-0 text-muted-foreground" />
      <span class="font-medium">{block.title}</span>
      <span class="min-w-0 flex-1 truncate text-xs text-muted-foreground">{firstReply}</span>
      <ChevronRight class="size-4 shrink-0 text-muted-foreground transition-transform duration-200 group-open:rotate-90" />
    </summary>
    <div class="flex flex-col gap-3 border-t bg-background/40 p-3.5">
      {#each block.members?.length ? block.members : [{ id: '', kind: 'agent', reply: block.body }] as m}
        <div>
          <div class="mb-1 flex items-center gap-2 text-[11px] font-medium tracking-wide text-muted-foreground uppercase">
            {m.kind}{#if m.id}<span class="font-mono normal-case">{m.id}</span>{/if}
          </div>
          <div class="max-h-96 overflow-auto rounded-xl bg-muted/60 p-3"><Markdown text={m.reply} /></div>
        </div>
      {/each}
    </div>
  </details>
{:else if block.card === 'thinking'}
  <details class="group animate-rise rounded-2xl border bg-card text-card-foreground" data-card="thinking" open={!block.collapsed}>
    <summary class="flex cursor-pointer list-none items-center gap-2.5 px-3.5 py-2.5 text-sm text-muted-foreground transition-colors hover:bg-muted/50 [&::-webkit-details-marker]:hidden">
      <Brain class="size-4" /><span class="flex-1">{block.title}</span>
      <ChevronRight class="size-4 transition-transform duration-200 group-open:rotate-90" />
    </summary>
    <pre class="max-h-64 overflow-auto border-t bg-background/40 p-3.5 font-mono text-xs whitespace-pre-wrap">{block.body}</pre>
  </details>
{:else if block.card === 'diff'}
  <div class="animate-rise overflow-hidden rounded-2xl border bg-card text-card-foreground" data-card="diff">
    <div class="border-b px-4 py-2 text-sm font-medium">Diff</div>
    <pre class="max-h-80 overflow-auto px-4 py-2 font-mono text-xs leading-relaxed">{#each diffLines as line}<span
        class:diff-add={line.startsWith('+') && !line.startsWith('+++')}
        class:diff-del={line.startsWith('-') && !line.startsWith('---')}>{line}
</span>{/each}</pre>
  </div>
{:else if block.card === 'permission'}
  <div class="animate-rise rounded-2xl border border-green-500/30 bg-card px-4 py-2.5 text-sm" data-card="permission">
    <span class="mr-2 font-medium">✓ {block.title}</span><span class="text-muted-foreground">{block.body}</span>
  </div>
{:else if block.card === 'error'}
  <div class="animate-rise rounded-2xl border border-destructive/40 bg-destructive/10 px-4 py-2.5 text-sm" role="alert" data-card="error">
    <span class="font-medium">Error: </span>{block.body}
  </div>
{:else if block.card === 'subagent'}
  <div class="animate-rise ml-6 rounded-2xl border border-dashed bg-card px-4 py-2.5 text-sm" data-card="subagent">
    <div class="mb-1 text-xs font-medium text-muted-foreground">{block.title}</div>
    <Markdown text={block.body} />
  </div>
{:else}
  <div class="animate-rise px-1 py-1 text-[15px] leading-7" data-card={block.card}>
    <div class="mb-1 text-xs font-medium text-muted-foreground">{block.title}</div>
    <Markdown text={block.body} />
  </div>
{/if}
