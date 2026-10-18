// Project scope values shared by the switcher popover and the phone Project
// sheet: All projects and No project are stable routes, never Project ids.
import { buildPath } from './router.js';

export const ALL_PROJECTS = '__all_projects__';
export const NO_PROJECT = '__no_project__';

export function scopeItems(projects = [], selectedId = null) {
  return [
    { value: ALL_PROJECTS, label: 'All projects' },
    { value: NO_PROJECT, label: 'No project' },
    ...projects.map((project) => ({ value: project.id, label: project.name })),
    ...(selectedId && selectedId !== 'none' && !projects.some((project) => project.id === selectedId)
      ? [{ value: selectedId, label: 'Project not found' }]
      : []),
  ];
}

export function scopeValue(selectedId) {
  return selectedId == null ? ALL_PROJECTS : selectedId === '' || selectedId === 'none' ? NO_PROJECT : selectedId;
}

export function scopePath(value) {
  if (value === ALL_PROJECTS) return buildPath({ name: 'home' });
  return buildPath({ name: 'project', projectId: value === NO_PROJECT ? 'none' : value });
}
