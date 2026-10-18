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
    await fireEvent.click(trigger);
    await fireEvent.click(await screen.findByRole('option', { name: 'All projects' }));
    expect(onNavigate).toHaveBeenLastCalledWith('/');

    await fireEvent.click(trigger);
    await fireEvent.click(await screen.findByRole('option', { name: 'No project' }));
    expect(onNavigate).toHaveBeenLastCalledWith('/p/none');

    await fireEvent.click(trigger);
    await fireEvent.click(await screen.findByRole('option', { name: 'Renamed Project' }));
    expect(onNavigate).toHaveBeenLastCalledWith('/p/stable-id');
  });

  it('searches the scope list from the popover', async () => {
    render(ProjectSwitcher, {
      props: {
        projects: [
          { id: 'a', name: 'Alpha', path: '/a' },
          { id: 'b', name: 'Bravo', path: '/b' },
        ],
        selectedId: null,
        onNavigate: vi.fn(),
      },
    });
    await fireEvent.click(screen.getByRole('button', { name: 'Project scope' }));
    await fireEvent.input(await screen.findByLabelText('Search projects'), { target: { value: 'brav' } });
    const names = (await screen.findAllByRole('option')).map((option) => option.textContent.trim());
    expect(names).toEqual(['Bravo']);
  });
});
