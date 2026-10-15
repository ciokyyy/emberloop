import { render } from 'svelte/server';
import DashboardPage from './lib/DashboardPage.svelte';

export function renderDashboard() {
  return render(DashboardPage, { props: { projects: [] } }).body;
}
globalThis.__elRenderDashboard = renderDashboard;
