<!-- Floating-label input (shadcn style, ported from scintilla-chat-v2): the
     label sits inside the field and floats to the top-left on focus or when
     the field has a value. The placeholder only shows while focused. -->
<script>
  import { cn } from './cn.js';

  let { class: className = '', inputClass = '', label, id, value = $bindable(''), ...rest } = $props();
  const uid = $props.id();
  const inputId = $derived(id ?? `fi-${uid}`);
</script>

<div class={cn('relative w-full', className)}>
  <input
    id={inputId}
    aria-label={label}
    bind:value
    class={cn(
      'peer flex h-14 w-full rounded-xl border border-input bg-transparent px-3.5 pt-5 pb-1.5 text-sm shadow-xs outline-none transition-[color,box-shadow] placeholder:text-transparent focus:placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/30 disabled:opacity-50',
      inputClass,
    )}
    placeholder={rest.placeholder ?? ' '}
    {...rest}
  />
  <label
    for={inputId}
    class="pointer-events-none absolute top-1/2 left-3.5 max-w-[calc(100%-1.75rem)] origin-left -translate-y-1/2 truncate text-sm text-muted-foreground transition-all duration-200 ease-[var(--ease-smooth)] peer-focus:top-2 peer-focus:translate-y-0 peer-focus:text-[11px] peer-focus:font-semibold peer-focus:text-ring peer-not-placeholder-shown:top-2 peer-not-placeholder-shown:translate-y-0 peer-not-placeholder-shown:text-[11px] peer-not-placeholder-shown:font-semibold peer-not-placeholder-shown:text-foreground/80 peer-disabled:opacity-50"
  >{label}</label>
</div>
