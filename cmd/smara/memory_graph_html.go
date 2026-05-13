package main

// memoryGraphHTML is a self-contained, dependency-free (CDN-only) page that
// renders the memory graph using vis-network. Works fully offline only when
// CDN is reachable; for fully offline use, run with --export to dump JSON.
const memoryGraphHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8" />
<title>Smara · Memory Graph</title>
<meta name="viewport" content="width=device-width,initial-scale=1" />
<script src="https://unpkg.com/vis-network/standalone/umd/vis-network.min.js"></script>
<style>
  :root {
    --bg: #0b0d0a;
    --panel: #11140f;
    --border: #1f2419;
    --text: #e9efe2;
    --muted: #8a9078;
    --accent: #bef264;
    --accent-soft: rgba(190,242,100,0.12);
    --auto: #6a8a3a;
    --manual: #bef264;
  }
  * { box-sizing: border-box; }
  html, body {
    margin: 0; height: 100%; background: var(--bg); color: var(--text);
    font-family: ui-sans-serif, -apple-system, "Segoe UI", system-ui, sans-serif;
  }
  #app { display: grid; grid-template-columns: 1fr 360px; height: 100vh; }
  #graph { background: radial-gradient(ellipse at center, #0f1410 0%, #0b0d0a 70%); }
  #side {
    background: var(--panel); border-left: 1px solid var(--border);
    padding: 18px; overflow-y: auto;
  }
  header { padding: 14px 18px; border-bottom: 1px solid var(--border);
           display: flex; align-items: center; gap: 12px;
           background: var(--panel); position: sticky; top: 0; z-index: 5; }
  header .dot { width: 8px; height: 8px; border-radius: 50%; background: var(--accent); }
  header h1 { margin: 0; font-size: 14px; font-weight: 600; letter-spacing: 0.3px; }
  header .stats { margin-left: auto; color: var(--muted); font-size: 12px; }
  header input { background: #0b0d0a; border: 1px solid var(--border);
                 color: var(--text); padding: 6px 10px; border-radius: 6px;
                 font-size: 12px; width: 220px; outline: none; }
  header input:focus { border-color: var(--accent); }
  #side h2 { font-size: 12px; text-transform: uppercase; letter-spacing: 1px;
             color: var(--muted); margin: 0 0 10px; }
  .empty { color: var(--muted); font-size: 13px; padding: 24px 0; }
  .pill { display: inline-block; padding: 2px 8px; border-radius: 999px;
          font-size: 11px; background: var(--accent-soft); color: var(--accent);
          margin-right: 6px; margin-bottom: 6px; }
  .pill.gray { background: #1a1f15; color: var(--muted); }
  .meta { color: var(--muted); font-size: 12px; margin-top: 10px; line-height: 1.6; }
  .content { background: #0b0d0a; border: 1px solid var(--border);
             padding: 12px; border-radius: 8px; white-space: pre-wrap;
             font-size: 13px; line-height: 1.5; max-height: 320px; overflow-y: auto; }
  .links { margin-top: 14px; }
  .link-row { display: flex; align-items: center; gap: 8px; padding: 6px 0;
              border-bottom: 1px dashed var(--border); font-size: 12px; }
  .link-row:last-child { border-bottom: 0; }
  .link-row .rel { color: var(--accent); font-weight: 600; }
  .link-row .auto { color: var(--auto); font-size: 10px; text-transform: uppercase; }
  .legend { display: flex; gap: 12px; align-items: center; padding: 10px 18px;
            font-size: 11px; color: var(--muted); border-top: 1px solid var(--border);
            background: var(--panel); }
  .legend span { display: inline-flex; align-items: center; gap: 6px; }
  .legend .swatch { width: 10px; height: 2px; }
  button.icon { background: transparent; border: 1px solid var(--border); color: var(--text);
                padding: 5px 10px; border-radius: 6px; font-size: 12px; cursor: pointer; }
  button.icon:hover { border-color: var(--accent); color: var(--accent); }
</style>
</head>
<body>
<div id="app">
  <div style="display:flex; flex-direction:column;">
    <header>
      <span class="dot"></span>
      <h1>Smara · Memory Graph</h1>
      <input id="search" placeholder="Cari node by content/tags…" />
      <span class="stats" id="stats">loading…</span>
      <button class="icon" id="reload">↻ reload</button>
    </header>
    <div id="graph" style="flex:1;"></div>
    <div class="legend">
      <span><span class="swatch" style="background:var(--manual);"></span> manual link</span>
      <span><span class="swatch" style="background:var(--auto); border-top:1px dashed var(--auto);"></span> auto link (similarity)</span>
      <span style="margin-left:auto;">click node untuk detail · scroll untuk zoom</span>
    </div>
  </div>
  <aside id="side">
    <h2>Detail Memori</h2>
    <div id="detail" class="empty">Pilih node dari graph untuk melihat detail.</div>
  </aside>
</div>

<script>
const elGraph  = document.getElementById('graph');
const elDetail = document.getElementById('detail');
const elStats  = document.getElementById('stats');
const elSearch = document.getElementById('search');

let network = null;
let allNodes = [];
let allEdges = [];
let nodesDS = null;
let edgesDS = null;

async function fetchGraph() {
  const res = await fetch('/api/graph');
  const data = await res.json();
  return data;
}

function classifyNode(n) {
  // Color scale by degree.
  const deg = n.degree || 0;
  let bg = '#1a1f15', border = '#3a4a25', font = '#cfd8c0';
  if (deg >= 1)  { bg = '#243018'; border = '#5a7a2a'; font = '#e6f0d4'; }
  if (deg >= 3)  { bg = '#3a5018'; border = '#8aae3a'; font = '#f4ffe0'; }
  if (deg >= 6)  { bg = '#557a25'; border = '#bef264'; font = '#0b0d0a'; }
  if (deg >= 10) { bg = '#bef264'; border = '#e7ffb0'; font = '#0b0d0a'; }
  return { background: bg, border, font };
}

function toVisNodes(nodes) {
  return nodes.map(n => {
    const c = classifyNode(n);
    return {
      id: n.id,
      label: n.label || ('#' + n.id),
      title: (n.tags && n.tags.length ? '[' + n.tags.join(', ') + '] ' : '') + (n.content || '').slice(0, 200),
      color: { background: c.background, border: c.border, highlight: { background: '#bef264', border: '#e7ffb0' } },
      font: { color: c.font, size: 12, face: 'ui-sans-serif' },
      shape: 'box',
      margin: 8,
      borderWidth: 1,
      _raw: n,
    };
  });
}

function toVisEdges(edges) {
  return edges.map((e, i) => ({
    id: 'e' + i,
    from: e.from,
    to: e.to,
    label: e.relation === 'similar' ? '' : e.relation,
    color: { color: e.auto ? 'rgba(106,138,58,0.55)' : 'rgba(190,242,100,0.85)',
             highlight: '#bef264' },
    dashes: !!e.auto,
    width: Math.max(0.6, Math.min(3.5, (e.weight || 0.4) * 3.5)),
    arrows: { to: { enabled: !e.auto, scaleFactor: 0.5 } },
    smooth: { type: 'continuous' },
    font: { color: '#8a9078', size: 10, strokeWidth: 0, align: 'top' },
  }));
}

function renderDetail(node) {
  if (!node) { elDetail.innerHTML = '<div class="empty">Pilih node dari graph untuk melihat detail.</div>'; return; }
  const r = node._raw || node;
  const tagsHTML = (r.tags || []).map(t => '<span class="pill">' + escapeHtml(t) + '</span>').join('');
  const neighbors = allEdges
    .filter(e => e.from === r.id || e.to === r.id)
    .map(e => {
      const otherId = e.from === r.id ? e.to : e.from;
      const other = allNodes.find(x => x.id === otherId);
      return { other, edge: e };
    });
  const linksHTML = neighbors.length === 0
    ? '<div class="empty">Belum ada link untuk memori ini.</div>'
    : neighbors.map(({other, edge}) => {
        if (!other) return '';
        return '<div class="link-row">' +
          '<span class="rel">' + escapeHtml(edge.relation) + '</span>' +
          '<span style="color:#8a9078">w=' + (edge.weight || 0).toFixed(2) + '</span>' +
          (edge.auto ? '<span class="auto">auto</span>' : '') +
          '<span style="margin-left:auto; cursor:pointer; color:#bef264;" data-id="' + other.id + '">[' + other.id + '] ' +
          escapeHtml(other.label || '') + '</span>' +
          '</div>';
      }).join('');
  elDetail.innerHTML =
    '<div style="font-size:11px; color:#8a9078;">MEMORY #' + r.id + (r.source ? ' · ' + escapeHtml(r.source) : '') + '</div>' +
    '<div style="margin:8px 0;">' + tagsHTML + (r.tags && r.tags.length ? '' : '<span class="pill gray">no tags</span>') + '</div>' +
    '<div class="content">' + escapeHtml(r.content || '') + '</div>' +
    '<div class="meta">degree: ' + (r.degree || 0) + ' · neighbors: ' + neighbors.length + '</div>' +
    '<div class="links"><h2 style="margin-top:18px;">Links</h2>' + linksHTML + '</div>';

  // Wire up neighbor click navigation.
  elDetail.querySelectorAll('[data-id]').forEach(el => {
    el.addEventListener('click', () => {
      const id = parseInt(el.getAttribute('data-id'));
      network.selectNodes([id]);
      network.focus(id, { scale: 1.2, animation: { duration: 400, easingFunction: 'easeInOutCubic' } });
      const target = allNodes.find(x => x.id === id);
      renderDetail(target);
    });
  });
}

function escapeHtml(s) {
  return String(s).replace(/[&<>"']/g, c =>
    ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
}

async function build() {
  const data = await fetchGraph();
  allNodes = data.nodes || [];
  allEdges = data.edges || [];
  elStats.textContent = allNodes.length + ' nodes · ' + allEdges.length + ' edges';

  if (allNodes.length === 0) {
    elGraph.innerHTML = '<div style="display:flex; align-items:center; justify-content:center; height:100%; color:#8a9078; font-size:14px;">' +
      'Belum ada memori di workspace ini.<br>Tambahkan dengan <code style=\"color:#bef264\">smara</code> atau jalankan <code style=\"color:#bef264\">smara memory autolink</code>.' +
      '</div>';
    return;
  }

  nodesDS = new vis.DataSet(toVisNodes(allNodes));
  edgesDS = new vis.DataSet(toVisEdges(allEdges));

  network = new vis.Network(elGraph, { nodes: nodesDS, edges: edgesDS }, {
    physics: {
      enabled: true,
      solver: 'forceAtlas2Based',
      forceAtlas2Based: { gravitationalConstant: -45, centralGravity: 0.008, springLength: 90, damping: 0.5 },
      stabilization: { iterations: 200 },
    },
    interaction: { hover: true, tooltipDelay: 150, navigationButtons: false },
    nodes: { borderWidthSelected: 2 },
    edges: { selectionWidth: 2 },
  });

  network.on('click', (params) => {
    if (params.nodes.length === 0) { renderDetail(null); return; }
    const id = params.nodes[0];
    const node = allNodes.find(n => n.id === id);
    if (!node) return;
    renderDetail(node);
  });

  network.once('stabilizationIterationsDone', () => {
    network.setOptions({ physics: { enabled: false } });
  });
}

elSearch.addEventListener('input', () => {
  if (!nodesDS) return;
  const q = elSearch.value.trim().toLowerCase();
  if (!q) {
    nodesDS.update(allNodes.map(n => ({ id: n.id, hidden: false })));
    edgesDS.update(allEdges.map((_, i) => ({ id: 'e' + i, hidden: false })));
    return;
  }
  const matchedIds = new Set(
    allNodes
      .filter(n =>
        (n.label || '').toLowerCase().includes(q) ||
        (n.content || '').toLowerCase().includes(q) ||
        (n.tags || []).some(t => t.toLowerCase().includes(q))
      )
      .map(n => n.id)
  );
  nodesDS.update(allNodes.map(n => ({ id: n.id, hidden: !matchedIds.has(n.id) })));
  edgesDS.update(allEdges.map((e, i) => ({
    id: 'e' + i,
    hidden: !(matchedIds.has(e.from) && matchedIds.has(e.to)),
  })));
});

document.getElementById('reload').addEventListener('click', () => {
  if (network) network.destroy();
  build();
});

build();
</script>
</body>
</html>`
