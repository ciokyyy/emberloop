<script module>
  import { tv } from 'tailwind-variants';

  export const buttonVariants = tv({
    base: 'inline-flex shrink-0 items-center justify-center gap-2 whitespace-nowrap rounded-full text-sm font-medium transition-all outline-none focus-visible:ring-[3px] focus-visible:ring-ring/40 active:scale-[0.97] disabled:pointer-events-none disabled:opacity-40 [&_svg]:pointer-events-none [&_svg]:size-4 [&_svg]:shrink-0',
    variants: {
      variant: {
        default: 'bg-primary text-primary-foreground hover:bg-primary/85',
        secondary: 'bg-muted text-foreground hover:bg-accent',
        outline: 'border border-border bg-transparent hover:bg-muted',
        ghost: 'hover:bg-muted',
        destructive: 'bg-destructive text-white hover:bg-destructive/90',
      },
      size: {
        default: 'h-9 px-4',
        sm: 'h-8 px-3 text-xs',
        lg: 'h-11 px-6 text-base',
        icon: 'size-9',
        'icon-sm': 'size-8',
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
