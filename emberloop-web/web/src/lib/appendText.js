// Append-only text for live blocks (thinking). Svelte's {body} interpolation
// replaces the whole string per delta; appending only the new tail avoids
// re-setting kilobytes per flush and never touches existing nodes, so the
// mount-only rise animation is never disturbed.
export function appendText(node, text = '') {
  let shown = String(text ?? '');
  node.textContent = shown;
  return {
    update(next = '') {
      next = String(next ?? '');
      if (next === shown) return;
      if (next.startsWith(shown) && node.firstChild && !node.firstChild.nextSibling) {
        node.firstChild.appendData(next.slice(shown.length));
      } else {
        node.textContent = next;
      }
      shown = next;
    },
  };
}
