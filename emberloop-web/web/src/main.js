import { mount } from 'svelte';
import './app.css';
import Root from './Root.svelte';
import { registerSW } from './lib/pwa.js';

// The offline shell registers at app start, not only when push is enabled,
// so the app is installable from the first visit.
void registerSW();

const app = mount(Root, { target: document.body });

export default app;
