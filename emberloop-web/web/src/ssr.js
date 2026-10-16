import { render } from 'svelte/server';
import DashboardPage from './lib/DashboardPage.svelte';

export function renderDashboard(projectsJSON = '[]', dashboardJSON = '{}') {
  const projects = typeof projectsJSON === 'string' ? JSON.parse(projectsJSON || '[]') : projectsJSON;
  const dashboard = typeof dashboardJSON === 'string' ? JSON.parse(dashboardJSON || '{}') : dashboardJSON;
  return render(DashboardPage, { props: { projects, initialDashboard: dashboard } }).body;
}
globalThis.__elRenderDashboard = renderDashboard;
