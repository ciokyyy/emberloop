import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, within } from '@testing-library/svelte';
import ProjectList from './ProjectList.svelte';

const projects = [
  { id: 'p1', name: 'emberloop', path: '/x/emberloop' },
  { id: 'p2', name: 'wick', path: '/y/wick' },
];
const sessions = [
  { id: 's1', harness: 'claude', title: 'Fix sidebar', status: 'idle', projectId: 'p1' },
  { id: 's2', harness: 'pig', title: 'Audio plan', status: 'working', projectId: 'p1' },
  { id: 's3', harness: 'claude', title: 'Rename pass', status: 'idle', projectId: 'p2' },
];

const props = {
  projects,
  sessions,
  currentId: null,
  activeProjectId: null,
  onSelect: vi.fn(),
  onNew: vi.fn(),
  onRename: vi.fn(),
  onRemove: vi.fn(),
};

beforeEach(() => localStorage.clear());

describe('chat filter', () => {
  it('shows everything without a query', () => {
    const { container } = render(ProjectList, { props: { ...props, query: '' } });
    expect(within(container).getByText('Fix sidebar')).toBeTruthy();
    expect(within(container).getByText('Rename pass')).toBeTruthy();
  });

  it('filters sessions by title across projects', () => {
    const { container } = render(ProjectList, { props: { ...props, query: 'sidebar' } });
    expect(within(container).getByText('Fix sidebar')).toBeTruthy();
    expect(within(container).queryByText('Audio plan')).toBeNull();
    expect(within(container).queryByText('Rename pass')).toBeNull();
  });

  it('matches project names with all their sessions', () => {
    const { container } = render(ProjectList, { props: { ...props, query: 'wick' } });
    expect(within(container).getByText('Rename pass')).toBeTruthy();
    expect(within(container).queryByText('Fix sidebar')).toBeNull();
  });

  it('force-opens collapsed projects with matches', () => {
    localStorage.setItem('eb.collapsed', JSON.stringify(['p1']));
    const { container } = render(ProjectList, { props: { ...props, query: 'sidebar' } });
    expect(within(container).getByText('Fix sidebar')).toBeTruthy();
  });

  it('reports no matches', () => {
    const { container } = render(ProjectList, { props: { ...props, query: 'zzz' } });
    expect(within(container).getByText(/no chats match/i)).toBeTruthy();
  });
});
