<!-- Compact status or fact chip: optional dot or icon plus one short word.
     `tone` is a status (working, needs-you, done, failed, paused) or neutral. -->
<script module>
  import { tv } from 'tailwind-variants';
  import { twMergeConfig } from './cn.js';

  export const chipVariants = tv({
    base: 'inline-flex max-w-full min-w-0 items-center gap-1.5 whitespace-nowrap rounded-full border border-transparent px-2.5 py-0.5 text-caption font-medium [&_svg]:size-3 [&_svg]:shrink-0',
    variants: {
      tone: {
        neutral: 'bg-muted text-fg-2',
        outline: 'border-border bg-transparent text-fg-3',
        working: 'bg-status-working/12 text-status-working',
        'needs-you': 'bg-status-needs-you/12 text-status-needs-you',
        done: 'bg-status-done/12 text-status-done',
        failed: 'bg-status-failed/12 text-status-failed',
        paused: 'bg-status-paused/12 text-status-paused',
      },
    },
    defaultVariants: { tone: 'neutral' },
  }, { twMergeConfig });

  export const DOT = {
    working: 'bg-status-working',
    'needs-you': 'bg-status-needs-you',
    done: 'bg-status-done',
    failed: 'bg-status-failed',
    paused: 'bg-status-paused',
  };
</script>

<script>
  import { cn } from './cn.js';

  let { tone = 'neutral', dot = false, icon: Icon = null, class: className = '', children, ...rest } = $props();
</script>

<span class={cn(chipVariants({ tone }), className)} data-tone={tone} {...rest}
  >{#if dot && DOT[tone]}<span class={cn('size-1.5 shrink-0 rounded-full', DOT[tone], tone === 'working' && 'motion-safe:animate-pulse')} aria-hidden="true"></span
  >{/if}{#if Icon}<Icon aria-hidden="true" />{/if}<span class="min-w-0 truncate">{@render children?.()}</span></span
>
