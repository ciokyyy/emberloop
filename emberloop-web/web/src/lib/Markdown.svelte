<!-- Agent output as sanitized Markdown. -->
<script>
  import { marked } from 'marked';
  import DOMPurify from 'dompurify';

  let { text = '', class: className = '' } = $props();

  marked.setOptions({ gfm: true, breaks: true });
  const html = $derived(DOMPurify.sanitize(marked.parse(String(text ?? ''), { async: false })));
</script>

<div class="md {className}">{@html html}</div>
