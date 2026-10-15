import { hydrate } from 'svelte';
import DashboardPage from './lib/DashboardPage.svelte';

hydrate(DashboardPage, {
  target: document.querySelector('#dashboard-root'),
  props: { projects: [] },
});
window.__EL_SSR_HYDRATED__ = true;
