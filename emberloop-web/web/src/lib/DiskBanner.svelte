<!-- One-line low-disk warning in the header area, fed by GET /api/disk. -->
<script>
  let { status = null } = $props();
  const line = $derived.by(() => {
    if (!status || !status.low) return '';
    const free = status.freeBytes ?? 0;
    const gb = (free / 1073741824).toFixed(1);
    return status.critical
      ? `Disk critically low: ${gb} GiB free — new Runs and Sessions are paused until space is freed.`
      : `Disk low: ${gb} GiB free — the janitor is trimming caches and temp dirs.`;
  });
</script>

{#if line}
  <p role="status" data-testid="disk-banner" class="animate-rise mx-3 rounded-xl bg-amber-500/10 px-4 py-2 text-sm text-amber-300 sm:mx-4">{line}</p>
{/if}
