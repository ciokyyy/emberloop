// The five app areas, shared by the desktop Rail and the phone drawer nav.
import { Settings } from '@lucide/svelte';
import { icons } from './icons.js';

export const areas = [
  { id: 'chat', label: 'Chat', short: 'Chat', path: '/', icon: icons.session },
  { id: 'runs', label: 'Specs and Runs', short: 'Runs', path: '/runs', icon: icons.run },
  { id: 'memory', label: 'Memory', short: 'Memory', path: '/memory', icon: icons.memory },
  { id: 'quota', label: 'Quota and Providers', short: 'Quota', path: '/quota', icon: icons.quota },
  { id: 'settings', label: 'Settings', short: 'Settings', path: '/settings', icon: Settings },
];
