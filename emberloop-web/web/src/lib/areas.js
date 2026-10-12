// The five app areas, shared by the desktop Rail and the phone drawer nav.
import { MessageSquare, Layers, Gauge, Brain, Settings } from '@lucide/svelte';

export const areas = [
  { id: 'chat', label: 'Chat', short: 'Chat', path: '/', icon: MessageSquare },
  { id: 'runs', label: 'Specs and Runs', short: 'Runs', path: '/runs', icon: Layers },
  { id: 'memory', label: 'Memory', short: 'Memory', path: '/memory', icon: Brain },
  { id: 'quota', label: 'Quota', short: 'Quota', path: '/quota', icon: Gauge },
  { id: 'settings', label: 'Settings', short: 'Settings', path: '/settings', icon: Settings },
];
