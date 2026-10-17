import { describe, it, expect, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup, waitFor } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import { Gauge } from '@lucide/svelte';
import Chip from './chip.svelte';
import EmptyState from './empty-state.svelte';
import ErrorState from './error-state.svelte';
import IconButton from './icon-button.svelte';
import InfoHint from './info-hint.svelte';
import Row from './row.svelte';
import Section from './section.svelte';
import Skeleton from './skeleton.svelte';
import StatCard from './stat-card.svelte';
import { cn } from './cn.js';
import { icons, statusIcons, statusLabels, providerGlyph } from '../icons.js';
import { areas } from '../areas.js';
import { settingsIcons } from '../settingsIcons.js';

afterEach(cleanup);

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
    for (const key of ['session', 'run', 'ticket', 'subagent', 'steer', 'followUp', 'report', 'memory', 'quota', 'provider', 'project', 'git', 'compress', 'copy', 'share', 'edit', 'retry', 'dismiss']) {
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
  it('Skeleton is decorative and motion-safe', () => {
    const { container } = render(Skeleton);
    const el = container.firstElementChild;
    expect(el.getAttribute('aria-hidden')).toBe('true');
    expect(el.className).toContain('motion-safe:animate-pulse');
  });
});
