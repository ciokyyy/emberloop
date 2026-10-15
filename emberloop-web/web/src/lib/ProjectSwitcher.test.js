import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen } from '@testing-library/svelte';
import ProjectSwitcher from './ProjectSwitcher.svelte';

afterEach(() => {
  cleanup();
});

describe('Project switcher', () => {
  it('offers All projects and No project as stable routes in addition to renamed Projects', async () => {
    const onNavigate = vi.fn();
    render(ProjectSwitcher, {
      props: {
        projects: [{ id: 'stable-id', name: 'Renamed Project', path: '/repo' }],
        selectedId: 'stable-id',
        onNavigate,
      },
    });

    const trigger = screen.getByRole('button', { name: 'Project scope' });
    expect(trigger.textContent).toContain('Renamed Project');
    await fireEvent.keyDown(trigger, { key: 'ArrowDown' });
    await fireEvent.pointerUp(await screen.findByRole('option', { name: 'All projects' }), { button: 0, pointerId: 1, pointerType: 'mouse' });
    expect(onNavigate).toHaveBeenLastCalledWith('/');

    await fireEvent.keyDown(trigger, { key: 'ArrowDown' });
    await fireEvent.pointerUp(await screen.findByRole('option', { name: 'No project' }), { button: 0, pointerId: 1, pointerType: 'mouse' });
    expect(onNavigate).toHaveBeenLastCalledWith('/p/none');

    await fireEvent.keyDown(trigger, { key: 'ArrowDown' });
    await fireEvent.pointerUp(await screen.findByRole('option', { name: 'Renamed Project' }), { button: 0, pointerId: 1, pointerType: 'mouse' });
    expect(onNavigate).toHaveBeenLastCalledWith('/p/stable-id');
  });
});
