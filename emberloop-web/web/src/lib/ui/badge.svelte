<script module>
  import { tv } from 'tailwind-variants';

  export const badgeVariants = tv({
    base: 'inline-flex items-center gap-1 whitespace-nowrap rounded-full px-2.5 py-0.5 text-xs font-medium',
    variants: {
      variant: {
        default: 'bg-primary text-primary-foreground',
        secondary: 'bg-muted text-muted-foreground',
        outline: 'border border-border text-muted-foreground',
        success: 'bg-green-500/10 text-green-300',
        warning: 'bg-amber-500/10 text-amber-300',
        destructive: 'bg-destructive/10 text-destructive',
      },
    },
    defaultVariants: { variant: 'secondary' },
  });
</script>

<script>
  import { cn } from './cn.js';

  let { class: className = '', variant = 'secondary', children, ...rest } = $props();
</script>

<span class={cn(badgeVariants({ variant }), className)} {...rest}>
  {@render children?.()}
</span>
