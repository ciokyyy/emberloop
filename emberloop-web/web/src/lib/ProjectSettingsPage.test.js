import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, fireEvent, waitFor, cleanup } from '@testing-library/svelte';
import ProjectSettingsPage from './ProjectSettingsPage.svelte';
afterEach(() => cleanup());

const project = {
  id: 'stable-id',
  name: 'Emberloop',
  path: '/work/emberloop',
  gist: 'A browser UI and local Daemon.',
};

describe('ProjectSettingsPage', () => {
  it('edits the Project name and gist without exposing override fields', async () => {
    const onRename = vi.fn(async (_project, name) => ({ ...project, name }));
    const onSaveGist = vi.fn(async (_project, gist) => ({ ...project, gist }));
    const { getByRole, getByText, queryByText } = render(ProjectSettingsPage, {
      props: { project, onRename, onSaveGist },
    });

    expect(getByRole('heading', { name: 'Project settings' })).toBeTruthy();
    expect(getByText('Emberloop', { selector: '[data-scope-chip] *' })).toBeTruthy();
    expect(getByRole('textbox', { name: 'Project name' }).value).toBe('Emberloop');
    expect(getByRole('textbox', { name: 'Project gist' }).value).toBe('A browser UI and local Daemon.');
    expect(queryByText(/override/i)).toBeNull();

    await fireEvent.input(getByRole('textbox', { name: 'Project name' }), { target: { value: 'Renamed Emberloop' } });
    await fireEvent.click(getByRole('button', { name: 'Save name' }));
    await waitFor(() => expect(onRename).toHaveBeenCalledWith(project, 'Renamed Emberloop'));
    expect(getByRole('textbox', { name: 'Project name' }).value).toBe('Renamed Emberloop');

    await fireEvent.input(getByRole('textbox', { name: 'Project gist' }), { target: { value: 'An edited one-line summary.' } });
    await fireEvent.click(getByRole('button', { name: 'Save gist' }));
    await waitFor(() => expect(onSaveGist).toHaveBeenCalledWith(project, 'An edited one-line summary.'));
    expect(getByRole('textbox', { name: 'Project gist' }).value).toBe('An edited one-line summary.');
  });

  it('refreshes clean fields when the same Project is updated', async () => {
    const view = render(ProjectSettingsPage, { props: { project } });
    await view.rerender({
      project: { ...project, name: 'Renamed elsewhere', gist: 'Updated summary.' },
    });

    await waitFor(() => {
      expect(view.getByRole('textbox', { name: 'Project name' }).value).toBe('Renamed elsewhere');
      expect(view.getByRole('textbox', { name: 'Project gist' }).value).toBe('Updated summary.');
    });
  });

  it('offers removal and returns to the Project Sessions route', async () => {
    const onBack = vi.fn();
    const onRemove = vi.fn();
    const { getByRole } = render(ProjectSettingsPage, { props: { project, onBack, onRemove } });

    await fireEvent.click(getByRole('button', { name: 'Back to Sessions' }));
    await fireEvent.click(getByRole('button', { name: 'Remove project' }));
    expect(onBack).toHaveBeenCalledOnce();
    await waitFor(() => expect(onRemove).toHaveBeenCalledWith(project));
  });

  it('distinguishes loading from a removed Project', () => {
    const loading = render(ProjectSettingsPage, { props: { loading: true } });
    expect(loading.getByRole('status', { name: /loading project settings/i })).toBeTruthy();
    loading.unmount();

    const missing = render(ProjectSettingsPage, { props: { loading: false } });
    expect(missing.getByTestId('project-settings-missing').textContent).toMatch(/project not found/i);
  });
});
