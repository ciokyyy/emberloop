<script module>
  import { tv } from 'tailwind-variants';

  export const buttonVariants = tv({
    base: 'inline-flex items-center justify-center gap-2 rounded-xl text-sm font-medium transition-colors disabled:pointer-events-none disabled:opacity-40 [&_svg]:size-4 [&_svg]:shrink-0',
    variants: {
      variant: {
        default: 'bg-primary text-primary-foreground shadow-sm hover:bg-primary/90',
        secondary: 'bg-muted text-foreground shadow-sm hover:bg-muted/70',
        outline: 'border border-border bg-background shadow-sm hover:bg-muted',
        ghost: 'hover:bg-muted',
      },
      size: {
        default: 'h-9 px-4 py-2',
        sm: 'h-8 rounded-lg px-3 text-xs',
        lg: 'h-10 rounded-xl px-6',
      },
    },
    defaultVariants: { variant: 'default', size: 'default' },
  });
</script>

<script>
  import { cn } from './cn.js';

  let { class: className = '', variant = 'default', size = 'default', type = 'button', children, ...rest } = $props();
</script>

<button {type} class={cn(buttonVariants({ variant, size }), className)} {...rest}>
  {@render children?.()}
</button>
