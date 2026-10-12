import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, fireEvent, cleanup } from '@testing-library/svelte';
import Markdown from './Markdown.svelte';

afterEach(() => cleanup());

describe('Markdown tables', () => {
  it('wraps a table in the rounded scroll container', () => {
    const { container } = render(Markdown, { props: { text: '| a | b |\n|---|---|\n| 1 | 2 |' } });
    const wrap = container.querySelector('.md-table');
    expect(wrap).toBeTruthy();
    expect(wrap.querySelector('table')).toBeTruthy();
  });
});

describe('Markdown code copy', () => {
  it('adds one copy button per fenced block, none for inline code', async () => {
    const { container } = render(Markdown, { props: { text: 'a `chip` and:\n```js\nconst a = 1;\n```\n```\nplain\n```' } });
    await vi.waitFor(() => expect(container.querySelectorAll('.md-copy')).toHaveLength(2));
    const btn = container.querySelector('.md-copy');
    expect(btn.getAttribute('aria-label')).toBe('Copy code');
    expect(btn.className).toContain('h-11');
  });
  it('copies over http via execCommand and shows Copied feedback', async () => {
    const exec = vi.fn(() => true);
    document.execCommand = exec;
    const { container } = render(Markdown, { props: { text: '```js\nconst a = 1;\n```' } });
    await vi.waitFor(() => expect(container.querySelector('.md-copy')).toBeTruthy());
    const btn = container.querySelector('.md-copy');
    await fireEvent.click(btn);
    expect(exec).toHaveBeenCalledWith('copy');
    expect(btn.textContent).toContain('Copied');
    expect(btn.getAttribute('aria-label')).toBe('Copied');
  });
});
