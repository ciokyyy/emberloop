<!-- Session reports tray: every report, dismissed ones included, so dismissing a
     notice is never destructive. The list scrolls inside a capped panel, so it
     stays readable with one report or ten at 390 px and 1280 px. -->
<script>
  import { ChevronRight } from '@lucide/svelte';
  import Button from './ui/button.svelte';
  import Chip from './ui/chip.svelte';
  import Markdown from './Markdown.svelte';
  import { icons } from './icons.js';
  import { reportCountsText } from './subagents.js';

  let { reports = [], dismissed = [], onRestore = () => {} } = $props();
  const ReportIcon = icons.report;
  const isDismissed = (report) => dismissed.includes(Number(report.seq));
  const preview = (report) => String(report.members?.[0]?.reply ?? report.body ?? '').replace(/\s+/g, ' ').trim().slice(0, 110);
  const name = (report) => report.groupName || report.groupId || 'Subagent group';
  const remaining = $derived(reports.filter((report) => !isDismissed(report)).length);
</script>

{#if reports.length}
  <details class="group w-full overflow-hidden rounded-xl border border-border/60 bg-surface-1 text-card-foreground shadow-elev-1" data-testid="reports-tray">
    <summary class="flex min-h-11 cursor-pointer list-none items-center gap-2 px-2 py-1.5 transition-colors hover:bg-surface-3/60 [&::-webkit-details-marker]:hidden">
      <span class="flex size-7 shrink-0 items-center justify-center rounded-lg bg-muted text-fg-2" aria-hidden="true">
        <ReportIcon class="size-3.5" />
      </span>
      <span class="shrink-0 text-body font-medium">Reports</span>
      <Chip tone="outline">{reports.length}</Chip>
      {#if remaining}<span class="min-w-0 flex-1 truncate text-caption text-fg-3">{remaining} in transcript</span>{:else}<span class="flex-1"></span>{/if}
      <ChevronRight class="size-4 shrink-0 text-fg-3 transition-transform duration-200 group-open:rotate-90" aria-hidden="true" />
    </summary>
    <div class="flex max-h-[min(60dvh,24rem)] flex-col gap-1 overflow-y-auto overscroll-contain border-t border-border/60 p-1.5">
      {#each reports as report (report.seq)}
        <details class="group/report rounded-lg border border-transparent bg-surface-0/60" data-report-seq={report.seq}>
          <summary class="flex min-h-11 cursor-pointer list-none items-center gap-2 rounded-lg px-2 py-1.5 transition-colors hover:bg-surface-3/60 [&::-webkit-details-marker]:hidden">
            <span class="min-w-0 flex-1">
              <span class="block truncate text-body text-fg-1">{name(report)}</span>
              <span class="block truncate text-caption text-fg-3">{reportCountsText(report)}{#if preview(report)} · {preview(report)}{/if}</span>
            </span>
            {#if isDismissed(report)}<Chip tone="neutral" class="shrink-0">Dismissed</Chip>{/if}
            <ChevronRight class="size-4 shrink-0 text-fg-3 transition-transform duration-200 group-open/report:rotate-90" aria-hidden="true" />
          </summary>
          <div class="flex flex-col gap-2 border-t border-border/60 p-2.5">
            <p class="text-caption text-fg-3">{reportCountsText(report)}</p>
            <div class="max-h-80 overflow-auto rounded-lg bg-surface-0 p-3"><Markdown text={report.body ?? ''} /></div>
            {#if isDismissed(report)}
              <div class="flex justify-end">
                <Button variant="outline" size="sm" aria-label={`Restore report ${name(report)}`} onclick={(event) => { event.preventDefault(); event.stopPropagation(); onRestore(report.seq); }}>Restore</Button>
              </div>
            {:else}
              <span class="text-right text-caption text-fg-3">In transcript</span>
            {/if}
          </div>
        </details>
      {/each}
    </div>
  </details>
{/if}
