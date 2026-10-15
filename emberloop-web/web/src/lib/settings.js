// Settings pages are global account routes, independent of Project scope.
export const SETTINGS_GROUPS = [
  {
    title: 'Workspace',
    items: [
      { id: 'agents', label: 'Agents', icon: 'bot', hint: 'Models, effort and fallbacks per role', scope: 'Global' },
      { id: 'runs', label: 'Runs', icon: 'ticket', hint: 'Ticket concurrency', scope: 'Global' },
      { id: 'context', label: 'Context & names', icon: 'layers', hint: 'Compression, Session names', scope: 'Global' },
      { id: 'memory', label: 'Memory', icon: 'brain', hint: 'Deciders and Jev', scope: 'Global' },
      { id: 'access', label: 'Users & access', icon: 'shield-check', hint: 'Sign-up and account access', scope: 'Admin' },
    ],
  },
  {
    title: 'You',
    items: [
      { id: 'profile', label: 'Profile', icon: 'user-round', hint: 'What agents know about you', scope: 'You' },
      { id: 'notifications', label: 'Notifications', icon: 'bell', hint: 'Push on this device', scope: 'You' },
      { id: 'sound-voice', label: 'Sound & voice', icon: 'volume-2', hint: 'Sounds, dictation and install', scope: 'You' },
    ],
  },
];

export const SETTINGS_PAGE_IDS = SETTINGS_GROUPS.flatMap((group) => group.items.map((item) => item.id));
export const SETTINGS_PAGES = Object.fromEntries(SETTINGS_GROUPS.flatMap((group) => group.items.map((item) => [item.id, item])));

export const settingsPath = (section) => `/settings/${encodeURIComponent(section)}`;
