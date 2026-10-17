<!-- Activity glyph plus the current tool in a few characters. The only decorative
     loop in the app; reduced motion keeps the glyph still. -->
<script>
  import { shortTool } from './toolstrip.js';

  // tool is the name of the running tool, '' when none is known.
  let { status = 'idle', tool = '', class: cls = 'px-3 pt-1' } = $props();
  const short = $derived(shortTool(tool));
</script>

{#if status === 'working'}
  <div data-testid="working-indicator" role="status" class="flex min-w-0 items-center gap-1.5 text-caption text-status-working {cls}" title={tool ? `Working: ${tool}` : 'Working'}>
    <svg class="working-spinner size-3.5 shrink-0 animate-spin motion-reduce:animate-none" viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <circle cx="12" cy="12" r="9" stroke="currentColor" stroke-opacity="0.25" stroke-width="3" />
      <path d="M21 12a9 9 0 0 0-9-9" stroke="currentColor" stroke-width="3" stroke-linecap="round" />
    </svg>
    <span class="sr-only">Working</span>
    {#if short}<span class="truncate font-mono text-fg-2" data-testid="working-tool" aria-hidden="true">{short}</span>{/if}
  </div>
{/if}
