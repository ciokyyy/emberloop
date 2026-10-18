import { describe, it, expect, afterEach, vi } from 'vitest';
import { render, screen, fireEvent, cleanup, waitFor } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import { Gauge } from '@lucide/svelte';
import Chip from './chip.svelte';
import EmptyState from './empty-state.svelte';
import ErrorState from './error-state.svelte';
import IconButton from './icon-button.svelte';
import InfoHint from './info-hint.svelte';
import PageHeader from './page-header.svelte';
import Panel from './panel.svelte';
import Row from './row.svelte';
import BottomSheet from './bottom-sheet.svelte';
import Section from './section.svelte';
import SettingRow from './setting-row.svelte';
import Skeleton from './skeleton.svelte';
import StatCard from './stat-card.svelte';
import Segmented from './segmented.svelte';
import { cn } from './cn.js';
import { SHELL_KEY } from '../shell.js';
import { icons, statusIcons, statusLabels, providerGlyph } from '../icons.js';
import { areas } from '../areas.js';
import { settingsIcons } from '../settingsIcons.js';
import { resetBackStack } from '../overlayBack.js';

afterEach(() => {
  cleanup();
  resetBackStack();
});

const text = (value) => createRawSnippet(() => ({ render: () => `<span>${value}</span>` }));

describe('cn token merging', () => {
  it('keeps colour and type-scale classes apart', () => {
    expect(cn('text-caption', 'text-fg-3')).toBe('text-caption text-fg-3');
    expect(cn('text-body', 'text-title')).toBe('text-title');
    expect(cn('shadow-elev-1', 'shadow-elev-2')).toBe('shadow-elev-2');
  });
});

describe('icon map', () => {
  it('names every domain concept from the spec once', () => {
    for (const key of ['session', 'run', 'ticket', 'subagent', 'steer', 'followUp', 'report', 'memory', 'quota', 'provider', 'project', 'git', 'compress', 'copy', 'share', 'edit', 'retry', 'dismiss', 'cancel']) {
      expect(icons[key], key).toBeTruthy();
    }
  });
  it('never gives two concepts one icon', () => {
    const all = Object.values(icons);
    expect(new Set(all).size).toBe(all.length);
    const statuses = Object.values(statusIcons);
    expect(new Set(statuses).size).toBe(statuses.length);
    expect(Object.keys(statusLabels)).toEqual(Object.keys(statusIcons));
  });
  it('feeds the rail and settings icons', () => {
    const byId = Object.fromEntries(areas.map((a) => [a.id, a.icon]));
    expect(byId.chat).toBe(icons.session);
    expect(byId.runs).toBe(icons.run);
    expect(byId.memory).toBe(icons.memory);
    expect(byId.quota).toBe(icons.quota);
    expect(settingsIcons.ticket).toBe(icons.ticket);
    expect(settingsIcons.brain).toBe(icons.memory);
    expect(providerGlyph('claude')).not.toBe(providerGlyph('openrouter'));
    expect(providerGlyph('unknown')).toBeTruthy();
  });
});

describe('IconButton', () => {
  it('has an accessible name, a 44px phone target and a tooltip on focus', async () => {
    render(IconButton, { props: { icon: Gauge, label: 'Compress' } });
    const button = screen.getByRole('button', { name: 'Compress' });
    expect(button.classList.contains('max-md:size-11')).toBe(true);
    expect(button.querySelector('svg').getAttribute('aria-hidden')).toBe('true');
    button.focus();
    await waitFor(() => expect(screen.getAllByText('Compress').length).toBeGreaterThan(0));
  });
  it('forwards clicks and disabled state', async () => {
    let clicks = 0;
    render(IconButton, { props: { icon: Gauge, label: 'Retry', onclick: () => clicks++ } });
    await fireEvent.click(screen.getByRole('button', { name: 'Retry' }));
    expect(clicks).toBe(1);
  });
  it('can drop the tooltip and still keeps the name', () => {
    render(IconButton, { props: { icon: Gauge, label: 'Dismiss', tooltip: false, disabled: true } });
    expect(screen.getByRole('button', { name: 'Dismiss' }).disabled).toBe(true);
  });
});

describe('InfoHint', () => {
  it('hides help until tapped', async () => {
    render(InfoHint, { props: { label: 'About X', children: text('Why this exists') } });
    expect(screen.queryByText('Why this exists')).toBeNull();
    const trigger = screen.getByRole('button', { name: 'About X' });
    expect(trigger.classList.contains('max-md:size-11')).toBe(true);
    await fireEvent.click(trigger);
    expect(await screen.findByText('Why this exists')).toBeTruthy();
  });
});

describe('numeric style', () => {
  it('keeps text-numeric figures tabular in app.css', async () => {
    const css = (await import('node:fs')).readFileSync(`${process.cwd()}/src/app.css`, 'utf8');
    expect(css).toMatch(/\.text-numeric\s*\{\s*font-variant-numeric:\s*tabular-nums/);
  });
});

describe('Chip', () => {
  it('renders the word with a status dot and tone', () => {
    const { container } = render(Chip, { props: { tone: 'needs-you', dot: true, children: text('Needs you') } });
    const chip = container.querySelector('[data-tone="needs-you"]');
    expect(chip.textContent).toBe('Needs you');
    expect(chip.className).toContain('text-status-needs-you');
    expect(chip.className).toContain('text-caption');
    expect(chip.querySelector('.bg-status-needs-you')).toBeTruthy();
  });
  it('truncates instead of overflowing', () => {
    const { container } = render(Chip, { props: { children: text('x') } });
    expect(container.firstElementChild.className).toContain('max-w-full');
    expect(container.querySelector('.truncate')).toBeTruthy();
  });
});

describe('Section', () => {
  it('renders title, one-line description and actions', () => {
    render(Section, { props: { title: 'Providers', description: 'Accounts', actions: text('act'), children: text('body') } });
    expect(screen.getByRole('heading', { level: 2, name: 'Providers' })).toBeTruthy();
    expect(screen.getByText('Accounts')).toBeTruthy();
    expect(screen.getByText('act')).toBeTruthy();
    expect(screen.getByText('body')).toBeTruthy();
  });
  it('moves help behind an info control', async () => {
    render(Section, { props: { title: 'Memory', hint: text('Long help') } });
    expect(screen.queryByText('Long help')).toBeNull();
    await fireEvent.click(screen.getByRole('button', { name: 'About Memory' }));
    expect(await screen.findByText('Long help')).toBeTruthy();
  });
  it('opens the hint inside a collapsible Section without toggling it', async () => {
    const { container } = render(Section, { props: { title: 'Ledger', collapsible: true, open: false, hint: 'Why', children: text('rows') } });
    const details = container.querySelector('details');
    await fireEvent.click(screen.getByRole('button', { name: 'About Ledger' }));
    expect(await screen.findByText('Why')).toBeTruthy();
    expect(details.open).toBe(false);
  });
  it('collapses secondary detail in a details element', () => {
    const { container } = render(Section, { props: { title: 'Ledger', collapsible: true, open: false, children: text('rows') } });
    const details = container.querySelector('details');
    expect(details.open).toBe(false);
    expect(details.querySelector('summary').textContent).toContain('Ledger');
  });
});

describe('Row', () => {
  it('is a link with title and one line of meta', () => {
    render(Row, { props: { title: 'Fix login', meta: '2 min ago', href: '/s/1', status: 'working' } });
    const link = screen.getByRole('link', { name: /Fix login/ });
    expect(link.getAttribute('href')).toBe('/s/1');
    expect(link.className).toContain('max-md:min-h-14');
    expect(link.querySelector('[data-status="working"]')).toBeTruthy();
  });
  it('is a button when it has a handler', async () => {
    let n = 0;
    render(Row, { props: { title: 'Open', onclick: () => n++ } });
    await fireEvent.click(screen.getByRole('button', { name: 'Open' }));
    expect(n).toBe(1);
  });
  it('is static otherwise and truncates long text', () => {
    const { container } = render(Row, { props: { title: 'Static', meta: 'meta' } });
    expect(container.querySelector('a, button')).toBeNull();
    expect(container.querySelectorAll('.truncate')).toHaveLength(2);
  });
});

describe('Row structure', () => {
  it('shows a status glyph with an sr-only word, an unread dot and the needs-you tint', () => {
    render(Row, { props: { title: 'Waits', meta: 'Session · pig', onclick: () => {}, status: 'needs-you', unread: true, attention: true, current: true } });
    const row = screen.getByRole('button', { name: /Needs you\. Waits/ });
    expect(row.querySelector('[data-status="needs-you"]')).toBeTruthy();
    expect(row.querySelector('[data-unread]')).toBeTruthy();
    expect(row.getAttribute('aria-current')).toBe('true');
    expect(row.className).toContain('border-status-needs-you/30');
  });
  it('drops the sr-only status word when the meta already names it', () => {
    render(Row, { props: { title: 'Failed one', meta: 'Failed', onclick: () => {}, status: 'failed', statusWord: false } });
    expect(screen.getByRole('button').textContent).not.toContain('Failed.');
  });
});

describe('PageHeader', () => {
  it('renders one h1 with a scope chip, hint and actions', async () => {
    const { container } = render(PageHeader, {
      props: { title: 'Runs', scope: 'All projects', hint: 'Why', actions: text('Act') },
    });
    expect(screen.getAllByRole('heading', { level: 1, name: 'Runs' })).toHaveLength(1);
    expect(container.querySelector('[data-scope-chip]').textContent).toContain('All projects');
    expect(screen.getByRole('button', { name: 'About runs' })).toBeTruthy();
    expect(screen.getByText('Act')).toBeTruthy();
  });
  it('shows the phone menu from the shell context and swaps it for Back on drill-down pages', async () => {
    let opened = 0;
    const context = new Map([[SHELL_KEY, { openMenu: () => opened++, menuLabel: 'Sessions' }]]);
    const first = render(PageHeader, { props: { title: 'Quota' }, context });
    await fireEvent.click(screen.getByRole('button', { name: 'Sessions' }));
    expect(opened).toBe(1);
    first.unmount();
    render(PageHeader, { props: { title: 'Agents', onBack: () => {}, backLabel: 'Settings', backOn: 'phone' }, context });
    expect(screen.queryByRole('button', { name: 'Sessions' })).toBeNull();
    expect(screen.getByRole('button', { name: 'Settings' }).className).toContain('max-md:size-11');
  });
  it('has no menu button without a shell', () => {
    render(PageHeader, { props: { title: 'Alone' } });
    expect(screen.queryByRole('button')).toBeNull();
  });
});

describe('StatCard', () => {
  it('shows a tabular value, unit and caption', () => {
    const { container } = render(StatCard, { props: { label: 'Peak use', value: 94, unit: '%', caption: 'OpenRouter', tone: 'failed' } });
    const value = container.querySelector('[data-stat-value]');
    expect(value.textContent).toBe('94');
    expect(value.className).toContain('tabular-nums');
    expect(container.querySelector('.text-status-failed')).toBeTruthy();
    expect(screen.getByText('OpenRouter')).toBeTruthy();
  });
});

describe('states', () => {
  it('EmptyState shows an icon, one line and one action', () => {
    const { container } = render(EmptyState, { props: { title: 'Nothing here', description: 'Start one.', action: text('Go') } });
    expect(screen.getByText('Nothing here')).toBeTruthy();
    expect(container.querySelector('svg')).toBeTruthy();
    expect(screen.getByText('Go')).toBeTruthy();
  });
  it('ErrorState alerts and retries', async () => {
    let n = 0;
    render(ErrorState, { props: { title: 'Could not load', detail: 'offline', onRetry: () => n++ } });
    expect(screen.getByRole('alert').textContent).toContain('offline');
    await fireEvent.click(screen.getByRole('button', { name: /Try again/ }));
    expect(n).toBe(1);
  });
  it('compact empty and error states are one row with the same meaning', async () => {
    const empty = render(EmptyState, { props: { title: 'No output', compact: true } });
    expect(empty.container.querySelector('[data-empty-state][data-compact]')).toBeTruthy();
    expect(screen.getByText('No output')).toBeTruthy();
    cleanup();
    let n = 0;
    render(ErrorState, { props: { title: 'Refresh failed', compact: true, onRetry: () => n++ } });
    // The retry button's own slot whitespace is all that joins the message.
    expect(screen.getByRole('alert').textContent.trim()).toBe('Refresh failed');
    await fireEvent.click(screen.getByRole('button', { name: 'Try again' }));
    expect(n).toBe(1);
  });
  it('Skeleton is decorative and motion-safe', () => {
    const { container } = render(Skeleton);
    const el = container.firstElementChild;
    expect(el.getAttribute('aria-hidden')).toBe('true');
    expect(el.className).toContain('motion-safe:animate-pulse');
  });

});

describe('Segmented', () => {
  const items = [
    { value: 'graph', label: 'Graph' },
    { value: 'list', label: 'List' },
  ];
  it('is a pressed-button group for a view switch', async () => {
    let picked = '';
    render(Segmented, { props: { label: 'Layout', value: 'graph', items, onchange: (v) => (picked = v) } });
    expect(screen.getByRole('group', { name: 'Layout' })).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Graph' }).getAttribute('aria-pressed')).toBe('true');
    await fireEvent.click(screen.getByRole('button', { name: 'List' }));
    expect(picked).toBe('list');
  });
  it('is a tablist whose arrow keys move the selection', async () => {
    let picked = '';
    const tabItems = [
      { value: 'status', label: 'Status' },
      { value: 'transcript', label: 'Transcript' },
      { value: 'facts', label: 'Facts' },
    ];
    render(Segmented, { props: { mode: 'tabs', label: 'Sections', panel: 'sections-panel', value: 'status', items: tabItems, onchange: (v) => (picked = v) } });
    // Every tab names the panel it drives, so the pair reads to assistive tech.
    expect(screen.getByRole('tab', { name: 'Facts' }).getAttribute('aria-controls')).toBe('sections-panel');
    expect(screen.getByRole('tablist', { name: 'Sections' })).toBeTruthy();
    expect(screen.getByRole('tab', { name: 'Status' }).getAttribute('aria-selected')).toBe('true');
    await fireEvent.keyDown(screen.getByRole('tab', { name: 'Status' }), { key: 'End' });
    expect(picked).toBe('facts');
  });
});

describe('BottomSheet', () => {
  it('opens from the bottom edge with a titled dialog and a close action', async () => {
    let open = true;
    const props = {
      get open() {
        return open;
      },
      set open(value) {
        open = value;
      },
      title: 'Ticket #3',
      children: text('Sheet body'),
    };
    render(BottomSheet, { props });
    // No description given: the sheet still has a name and a valid
    // description reference (an sr-only copy), so the dialog reads cleanly.
    const dialog = await screen.findByRole('dialog', { name: 'Ticket #3' });
    expect(dialog.textContent).toContain('Sheet body');
    // The close control is a named icon button; bits-ui owns Escape and the
    // backdrop, and RunsView covers the bind:open round trip.
    expect(screen.getByRole('button', { name: 'Close' })).toBeTruthy();
  });
  it('closes on system Back and drops the entry it held', async () => {
    const back = vi.spyOn(history, 'back').mockImplementation(() => {});
    let open = true;
    const props = {
      get open() {
        return open;
      },
      set open(value) {
        open = value;
      },
      title: 'Palette',
      children: text('Sheet body'),
    };
    render(BottomSheet, { props });
    await screen.findByRole('dialog', { name: 'Palette' });
    expect(history.state?.elSheet).toBe(1);

    window.dispatchEvent(new PopStateEvent('popstate'));
    await vi.waitFor(() => expect(open).toBe(false));
    resetBackStack();
    back.mockRestore();
  });
});

describe('Panel and SettingRow', () => {
  it('Panel is one divided card for rows', () => {
    const { container } = render(Panel, { props: { children: text('rows') } });
    const el = container.querySelector('[data-panel]');
    expect(el.className).toContain('rounded-2xl');
    expect(el.className).toContain('divide-y');
    expect(el.className).toContain('shadow-elev-1');
  });
  it('SettingRow keeps help behind a hint and the note visible', async () => {
    render(SettingRow, { props: { label: 'Cooldown', hint: 'Turns to wait', note: 'Set by the environment', children: text('control') } });
    expect(screen.getByText('Cooldown')).toBeTruthy();
    expect(screen.getByText('Set by the environment')).toBeTruthy();
    expect(screen.getByText('control')).toBeTruthy();
    expect(screen.queryByText('Turns to wait')).toBeNull();
    await fireEvent.click(screen.getByRole('button', { name: 'About Cooldown' }));
    expect(await screen.findByText('Turns to wait')).toBeTruthy();
  });
  it('SettingRow stacks the control under the label on phone unless inline', () => {
    const stacked = render(SettingRow, { props: { label: 'A', children: text('x') } });
    expect(stacked.container.querySelector('[data-setting-row]').className).toContain('flex-col');
    stacked.unmount();
    const inline = render(SettingRow, { props: { label: 'B', inline: true, children: text('x') } });
    expect(inline.container.querySelector('[data-setting-row]').className).not.toContain('flex-col');
  });
});
