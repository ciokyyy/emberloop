<script>
  import Button from './ui/button.svelte';
  import Markdown from './Markdown.svelte';
  import { icons } from './icons.js';

  let { reports = [], dismissed = [], onRestore = () => {} } = $props();
  const ReportIcon = icons.report;
  const isDismissed = (report) => dismissed.includes(Number(report.seq));
  const countLabel = (report) => {
    const counts = report.counts ?? {};
    const parts = [`${counts.total ?? report.members?.length ?? 0} agent${(counts.total ?? report.members?.length ?? 0) === 1 ? '' : 's'}`];
    if (counts.workers) parts.push(`${counts.workers} worker${counts.workers === 1 ? '' : 's'}`);
    if (counts.scouts) parts.push(`${counts.scouts} scout${counts.scouts === 1 ? '' : 's'}`);
    if (counts.failed) parts.push(`${counts.failed} failed`);
    return parts.join(' · ');
  };
  const preview = (report) => String(report.members?.[0]?.reply ?? report.body ?? '').replace(/\s+/g, ' ').trim().slice(0, 110);
</script>

{#if reports.length}
  <details class="group w-full rounded-xl border bg-card text-card-foreground shadow-elev-1" data-testid="reports-tray">
    <summary class="flex min-h-11 cursor-pointer list-none items-center gap-2 rounded-xl px-3 py-2 text-sm hover:bg-muted/50 [&::-webkit-details-marker]:hidden">
      <ReportIcon class="size-4 shrink-0 text-muted-foreground" />
      <span class="font-medium">Reports</span>
      <span class="text-caption text-fg-3">{reports.length}</span>
      <span class="min-w-0 flex-1"></span>
      <span class="text-caption text-fg-3 transition-transform group-open:rotate-90" aria-hidden="true">›</span>
    </summary>
    <div class="flex flex-col gap-2 border-t p-2.5">
      {#each reports as report (report.seq)}
        <details class="group rounded-lg border bg-background/40" data-report-seq={report.seq}>
          <summary class="flex min-h-11 cursor-pointer list-none items-center gap-2 px-3 py-2 text-sm [&::-webkit-details-marker]:hidden">
            <span class="min-w-0 flex-1 truncate font-medium">{report.groupName || report.groupId || 'Subagent group'}</span>
            <span class="hidden max-w-[45%] truncate text-caption text-fg-3 sm:block">{countLabel(report)}</span>
            {#if isDismissed(report)}<span class="rounded-full bg-muted px-2 py-0.5 text-caption text-fg-3">Dismissed</span>{/if}
          </summary>
          <div class="flex flex-col gap-2 border-t p-3">
            <p class="text-caption text-fg-3">{countLabel(report)}</p>
            {#if preview(report)}<p class="truncate text-body text-fg-2">{preview(report)}</p>{/if}
            <div class="max-h-80 overflow-auto rounded-lg bg-muted/50 p-3"><Markdown text={report.body ?? ''} /></div>
            {#if isDismissed(report)}
              <div class="flex justify-end">
                <Button variant="outline" size="sm" aria-label={`Restore report ${report.groupName || report.groupId || report.seq}`} onclick={(event) => { event.preventDefault(); event.stopPropagation(); onRestore(report.seq); }}>Restore</Button>
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
