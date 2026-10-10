import { mount } from 'svelte';
import './app.css';
import App from './App.svelte';
import { registerSW } from './lib/pwa.js';

// The offline shell registers at app start, not only when push is enabled,
// so the app is installable from the first visit.
void registerSW();

const app = mount(App, { target: document.body });

export default app;
