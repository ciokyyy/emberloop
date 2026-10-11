<!-- A User's initial on its owner color. A green dot marks a User who is online. -->
<script>
  let { name = '', tone = 'var(--muted-foreground)', online = false, size = 'md' } = $props();
  const initial = $derived(String(name ?? '').trim().slice(0, 1).toUpperCase() || '?');
</script>

<span
  class="relative grid shrink-0 place-items-center rounded-full font-semibold text-background {size === 'sm' ? 'size-4 text-[9px]' : 'size-6 text-xs'}"
  style="background: {tone}"
  aria-hidden="true"
  data-testid="avatar"
>
  {initial}
  {#if online}
    <span class="absolute -right-0.5 -bottom-0.5 size-2 rounded-full border-2 border-sidebar bg-green-500" title="Online" data-testid="online-dot"></span>
  {/if}
</span>
