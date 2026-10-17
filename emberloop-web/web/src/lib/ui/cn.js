import { clsx } from 'clsx';
import { extendTailwindMerge } from 'tailwind-merge';

// The design-system type and elevation tokens (app.css) are custom utilities;
// teach the merger their groups so text-caption never eats text-fg-3.
export const twMergeConfig = {
  extend: {
    classGroups: {
      'font-size': [{ text: ['headline', 'title', 'body', 'caption', 'numeric'] }],
      shadow: [{ shadow: ['elev-1', 'elev-2', 'elev-3'] }],
    },
  },
};

const twMerge = extendTailwindMerge(twMergeConfig);

export function cn(...inputs) {
  return twMerge(clsx(...inputs));
}
