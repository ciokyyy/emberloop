import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/svelte';
import Markdown from './Markdown.svelte';

describe('Markdown tables', () => {
  it('wraps a table in the rounded scroll container', () => {
    const { container } = render(Markdown, { props: { text: '| a | b |\n|---|---|\n| 1 | 2 |' } });
    const wrap = container.querySelector('.md-table');
    expect(wrap).toBeTruthy();
    expect(wrap.querySelector('table')).toBeTruthy();
  });
});
