<script>
let sessions = [], current = null, lastSeq = 0, source = null;

async function refresh() {
  const res = await fetch('/api/sessions');
  sessions = await res.json();
  if (!current && sessions.length) openSession(sessions[0].id);
}

async function openSession(id) {
  current = id; lastSeq = 0;
  document.getElementById('transcript').innerHTML = '';
  if (source) source.close();
  const res = await fetch('/api/sessions/' + id);
  const body = await res.json();
  (body.events || []).forEach(show);
  lastSeq = (body.events || []).reduce((m, e) => Math.max(m, e.seq), 0);
  source = new EventSource('/api/sessions/' + id + '/events?since=' + lastSeq);
  source.onmessage = (ev) => {
    const e = JSON.parse(ev.data);
    lastSeq = Math.max(lastSeq, e.seq);
    show(e);
  };
}

function show(e) {
  const div = document.createElement('div');
  div.dataset.seq = e.seq;
  div.textContent = '#' + e.seq + ' ' + e.kind + ' ' + (e.text || e.status || '');
  document.getElementById('transcript').appendChild(div);
}

async function create(ev) {
  ev.preventDefault();
  const body = {
    harness: document.getElementById('harness').value,
    model: document.getElementById('model').value,
    workDir: document.getElementById('workDir').value
  };
  const res = await fetch('/api/sessions', {
    method: 'POST', headers: {'Content-Type': 'application/json'},
    body: JSON.stringify(body)
  });
  const s = await res.json();
  await refresh();
  openSession(s.id);
}

async function send(ev) {
  ev.preventDefault();
  if (!current) return;
  const text = document.getElementById('prompt').value;
  document.getElementById('prompt').value = '';
  await fetch('/api/sessions/' + current + '/prompt', {
    method: 'POST', headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({text})
  });
}
</script>

<main>
<h1>Sessions</h1>
<form onsubmit={create}>
<input id="harness" value="fake" aria-label="harness" />
<input id="model" value="fake-model" aria-label="model" />
<input id="workDir" value="." aria-label="working directory" />
<button>Create Session</button>
</form>
<ul>
{#each sessions as s}
<li><a href="#" onclick={(e) => { e.preventDefault(); openSession(s.id); }}>{s.id} {s.status}</a></li>
{/each}
</ul>
<h2>Transcript</h2>
<div id="transcript"></div>
<form onsubmit={send}>
<input id="prompt" aria-label="prompt" placeholder="Send a prompt" />
<button>Send</button>
</form>
</main>
