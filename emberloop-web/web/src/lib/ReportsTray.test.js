import { describe, it, expect, vi } from 'vitest';
import { fireEvent, render, within } from '@testing-library/svelte';
import ReportsTray from './ReportsTray.svelte';

const report = { seq: 4, groupName: 'group-a', counts: { total: 1, workers: 1 }, body: 'completed work', members: [{ kind: 'worker', reply: 'completed work' }] };

describe('ReportsTray', () => {
  it('lists dismissed reports and restores a selected report', async () => {
    const onRestore = vi.fn();
    const { container } = render(ReportsTray, { props: { reports: [report], dismissed: [4], onRestore } });
    const tray = container.querySelector('[data-testid="reports-tray"]');
    expect(tray.textContent).toContain('group-a');
    expect(tray.textContent).toContain('Dismissed');
    await fireEvent.click(within(tray).getByRole('button', { name: 'Restore report group-a' }));
    expect(onRestore).toHaveBeenCalledWith(4);
  });
});
