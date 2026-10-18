<!-- One setting: a label with an optional tap-to-reveal hint, an optional
     visible `note` (state such as "Set by an environment variable" or a
     warning), and the control. The control sits beside the label on desktop
     and under it on phone; `inline` keeps it beside the label on phone too
     (switches, small buttons); `stack` keeps it under the label on desktop too
     (textareas). `width` sizes the control on desktop. -->
<script>
  import InfoHint from './info-hint.svelte';
  import { cn } from './cn.js';

  const WIDTH = {
    auto: '',
    narrow: 'md:w-36',
    field: 'md:w-64',
    full: 'md:w-full',
    wide: 'md:w-80',
  };
  const NOTE = { default: 'text-fg-3', warn: 'text-status-needs-you', error: 'text-status-failed' };

  let {
    label,
    hint = null,
    note = '',
    noteTone = 'default',
    inline = false,
    stack = false,
    width = 'auto',
    class: className = '',
    controlClass = '',
    children,
    ...rest
  } = $props();
</script>

<div
  class={cn(
    'flex gap-3 p-4',
    inline ? 'items-center justify-between' : stack ? 'flex-col' : 'flex-col md:flex-row md:items-center md:justify-between md:gap-6',
    className,
  )}
  data-setting-row
  {...rest}
>
  <div class="flex min-w-0 flex-1 flex-col gap-0.5">
    <div class="flex min-h-6 min-w-0 items-center gap-1">
      <span class="min-w-0 text-body text-fg-1">{label}</span>
      {#if hint}
        <InfoHint label={`About ${label}`} class="max-md:-my-2.5">{#if typeof hint === 'string'}{hint}{:else}{@render hint()}{/if}</InfoHint>
      {/if}
    </div>
    {#if note}
      <p class={cn('text-caption', NOTE[noteTone])}>{#if typeof note === 'string'}{note}{:else}{@render note()}{/if}</p>
    {/if}
  </div>
  {#if children}
    <div class={cn('min-w-0', inline ? 'shrink-0' : 'w-full md:w-auto md:shrink-0', WIDTH[width], controlClass)}>{@render children()}</div>
  {/if}
</div>
