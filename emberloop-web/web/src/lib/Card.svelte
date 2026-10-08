<!-- Transcript card: one visual block per grouped Event run. -->
<script>
  let { block } = $props();
  const diffLines = $derived(block.card === 'diff' ? String(block.body ?? '').split('\n') : []);
</script>

{#if block.card === 'user'}
  <div class="flex justify-end" data-card="user">
    <div class="max-w-[80%] rounded-2xl rounded-br-md bg-primary px-4 py-2 text-sm text-primary-foreground shadow-sm">
      {block.body}
    </div>
  </div>
{:else if block.card === 'status'}
  <div class="flex justify-center" data-card="status">
    <span class="rounded-full border bg-muted px-3 py-0.5 text-xs text-muted-foreground">Session {block.title}</span>
  </div>
{:else if block.card === 'tool' || block.card === 'thinking'}
  <details class="rounded-xl border bg-card text-card-foreground shadow-sm" data-card={block.card} open={!block.collapsed}>
    <summary class="cursor-pointer list-none px-4 py-2.5 text-sm font-medium">
      <span class="mr-2 inline-block rounded-md bg-muted px-1.5 py-0.5 font-mono text-xs">{block.card === 'tool' ? 'tool' : 'thinking'}</span>{block.title}
    </summary>
    <pre class="max-h-64 overflow-auto border-t bg-muted/50 px-4 py-2 font-mono text-xs whitespace-pre-wrap">{block.body}</pre>
  </details>
{:else if block.card === 'diff'}
  <div class="overflow-hidden rounded-xl border bg-card text-card-foreground shadow-sm" data-card="diff">
    <div class="border-b px-4 py-2 text-sm font-medium">Diff</div>
    <pre class="max-h-80 overflow-auto px-4 py-2 font-mono text-xs leading-relaxed">{#each diffLines as line}<span
        class:diff-add={line.startsWith('+') && !line.startsWith('+++')}
        class:diff-del={line.startsWith('-') && !line.startsWith('---')}>{line}
</span>{/each}</pre>
  </div>
{:else if block.card === 'permission'}
  <div class="rounded-xl border border-green-500/30 bg-card px-4 py-2.5 text-sm shadow-sm" data-card="permission">
    <span class="mr-2 font-medium">✓ {block.title}</span><span class="text-muted-foreground">{block.body}</span>
  </div>
{:else if block.card === 'error'}
  <div class="rounded-xl border border-destructive/50 bg-destructive/10 px-4 py-2.5 text-sm shadow-sm" role="alert" data-card="error">
    <span class="font-medium">Error: </span>{block.body}
  </div>
{:else if block.card === 'subagent'}
  <div class="ml-6 rounded-xl border border-dashed bg-card px-4 py-2.5 text-sm shadow-sm" data-card="subagent">
    <div class="mb-1 text-xs font-medium text-muted-foreground">{block.title}</div>
    <div class="whitespace-pre-wrap">{block.body}</div>
  </div>
{:else}
  <div class="rounded-xl border bg-card px-4 py-2.5 text-sm shadow-sm" data-card={block.card}>
    <div class="mb-1 text-xs font-medium text-muted-foreground">{block.title}</div>
    <div class="whitespace-pre-wrap">{block.body}</div>
  </div>
{/if}
