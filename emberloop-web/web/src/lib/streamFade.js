// Streaming text fade. The Markdown view replaces its whole DOM on every
// update, so a plain CSS animation on new text would restart each time.
// Instead each arrival is recorded with a timestamp, and every apply() wraps
// the characters that arrived within the last FADE_MS in a span whose
// negative animation-delay equals its age: the animation resumes where it
// was. The first apply is the baseline and never animates. A fast stream
// buries text faster than any fade can play (and the pace jump means the
// fade would never complete), so apply() skips wrapping once the caller's
// backlog passes SKIP_FADE_BACKLOG; slow streams keep the fade.
// FADE_MS is the emphasis duration (--dur-emphasis in app.css): the span's
// negative delay and the CSS animation must agree.
export const FADE_MS = 320;
export const SKIP_FADE_BACKLOG = 1500;

export function fadeNewText(now = () => performance.now()) {
  let seen = -1; // chars at the previous apply; -1 before the baseline
  const marks = []; // { from, to, t }: char range and arrival time

  function textNodes(root) {
    const out = [];
    const w = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
    for (let n = w.nextNode(); n; n = w.nextNode()) out.push(n);
    return out;
  }

  function apply(root, opts = {}) {
    const nodes = textNodes(root);
    const total = nodes.reduce((a, n) => a + n.data.length, 0);
    if (seen < 0) {
      seen = total;
      return;
    }
    const t = now();
    const grew = total > seen;
    if (grew) marks.push({ from: seen, to: total, t });
    seen = total;
    while (marks.length && t - marks[0].t > FADE_MS) marks.shift();
    if (!marks.length || !grew) return;
    // No growth since the last apply means the live spans (if any) are still
    // playing on their own delays: re-walking would only nest new spans
    // inside them. A huge backlog means the text arrives faster than the
    // fade: skip the splitText churn and let it appear at once.
    if ((opts.backlog ?? 0) > SKIP_FADE_BACKLOG) {
      marks.length = 0;
      return;
    }
    // Newest range first: splitting a node leaves its head in place, so the
    // earlier offsets stay valid.
    const young = [...marks].reverse();
    let offset = 0;
    for (const node of nodes) {
      const a = offset;
      offset += node.data.length;
      for (const y of young) {
        const from = Math.max(a, y.from) - a;
        const to = Math.min(offset, y.to) - a;
        if (to <= from) continue;
        wrap(node, from, to, t - y.t);
      }
    }
  }

  function wrap(node, from, to, age) {
    if (to < node.data.length) node.splitText(to);
    const mid = from > 0 ? node.splitText(from) : node;
    const span = document.createElement('span');
    span.className = 'stream-in';
    span.style.animationDelay = `-${Math.round(age)}ms`;
    mid.parentNode.insertBefore(span, mid);
    span.appendChild(mid);
  }

  return { apply };
}
