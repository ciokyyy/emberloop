<!-- Agent output as sanitized Markdown. -->
<script>
  import { marked } from 'marked';
  import DOMPurify from 'dompurify';

  let { text = '', class: className = '' } = $props();

  marked.setOptions({ gfm: true, breaks: true });
  // Tables sit in a clipped, rounded wrapper: a bare table cannot round its
  // corners with collapsed borders, and the wrapper scrolls wide ones.
  const html = $derived(
    DOMPurify.sanitize(marked.parse(String(text ?? ''), { async: false }))
      .replaceAll('<table', '<div class="md-table"><table')
      .replaceAll('</table>', '</table></div>'),
  );
</script>

<div class="md {className}">{@html html}</div>
