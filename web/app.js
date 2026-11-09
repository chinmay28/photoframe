// Photoframe companion app. Plain ES modules, no build step: the Pi serves
// these files straight from the binary.

const $ = (sel, root = document) => root.querySelector(sel);
const esc = (s) => String(s ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));

// ---------- API ----------

async function request(method, path, body) {
  const opts = { method, headers: {} };
  if (body !== undefined) {
    opts.headers['Content-Type'] = 'application/json';
    opts.body = JSON.stringify(body);
  }
  const res = await fetch(path, opts);
  if (!res.ok) {
    let msg = res.statusText || `HTTP ${res.status}`;
    try { msg = (await res.json()).error || msg; } catch { /* not JSON */ }
    throw new Error(msg);
  }
  if ((res.headers.get('content-type') || '').includes('json')) return res.json();
  return null;
}
const api = {
  get: (p) => request('GET', p),
  put: (p, b) => request('PUT', p, b),
  post: (p, b) => request('POST', p, b),
  del: (p) => request('DELETE', p),
};

// The API rejects unknown fields, so send only what a source accepts.
function sourceBody(s) {
  const { id, name, type, enabled, path, url, apiKey, albumId } = s;
  return { id, name, type, enabled, path, url, apiKey, albumId };
}
function settingsBody(s) {
  return { ...s, sources: [] }; // sources have their own endpoints
}

// ---------- helpers ----------

let toastTimer;
function toast(msg, isError = false) {
  const t = $('#toast');
  t.textContent = msg;
  t.className = 'toast' + (isError ? ' error' : '');
  t.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => { t.hidden = true; }, isError ? 5000 : 2500);
}

function debounce(fn, ms) {
  let t;
  return (...a) => { clearTimeout(t); t = setTimeout(() => fn(...a), ms); };
}

// Go duration string ("1h30m0s") to minutes.
function minutes(d) {
  let total = 0;
  for (const [, n, u] of String(d).matchAll(/([\d.]+)(h|m|s|ms|us|µs|ns)/g)) {
    total += parseFloat(n) * ({ h: 60, m: 1, s: 1 / 60 }[u] || 0);
  }
  return Math.round(total);
}
function fmtInterval(d) {
  const m = minutes(d);
  if (m % 1440 === 0) return m === 1440 ? 'Daily' : `Every ${m / 1440} days`;
  if (m % 60 === 0) return m === 60 ? 'Every hour' : `Every ${m / 60} hours`;
  return `Every ${m} min`;
}
function relTime(iso) {
  const ms = new Date(iso) - Date.now();
  const m = Math.round(ms / 60000);
  if (m <= 0) return 'now';
  if (m < 60) return `in ${m} min`;
  if (m < 12 * 60) return `in ${Math.floor(m / 60)} h ${m % 60} min`;
  return 'at ' + new Date(iso).toLocaleString([], { weekday: 'short', hour: '2-digit', minute: '2-digit' });
}
function ago(iso) {
  if (!iso) return 'never synced';
  const m = Math.round((Date.now() - new Date(iso)) / 60000);
  if (m < 1) return 'synced just now';
  if (m < 60) return `synced ${m} min ago`;
  if (m < 48 * 60) return `synced ${Math.round(m / 60)} h ago`;
  return `synced ${Math.round(m / 1440)} days ago`;
}
function monthYear(iso) {
  if (!iso || iso.startsWith('0001')) return '';
  return new Date(iso).toLocaleDateString([], { month: 'short', year: 'numeric' });
}
const plural = (n, word) => `${n} ${word}${n === 1 ? '' : 's'}`;

const icons = {
  prev: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M15 18l-6-6 6-6"/></svg>',
  next: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M9 18l6-6-6-6"/></svg>',
  pause: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M8 5v14M16 5v14"/></svg>',
  play: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M7 5l12 7-12 7z"/></svg>',
  plus: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 5v14M5 12h14"/></svg>',
  upload: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 16V4M7 9l5-5 5 5M5 20h14"/></svg>',
  sync: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M20 11a8 8 0 0 0-14.9-3M4 4v4h4M4 13a8 8 0 0 0 14.9 3M20 20v-4h-4"/></svg>',
  trash: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M4 7h16M9 7V4h6v3M6 7l1 13h10l1-13"/></svg>',
  adjust: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M6 2v14a2 2 0 0 0 2 2h14M18 22V8a2 2 0 0 0-2-2H2"/></svg>',
  rotl: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M3 4v5h5M3.5 9A9 9 0 1 1 5 17"/></svg>',
  rotr: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M21 4v5h-5M20.5 9A9 9 0 1 0 19 17"/></svg>',
};
const sourceTypes = {
  upload: { label: 'Stored on frame', icon: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M8 3h8a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2zM12 15V8M9 11l3-3 3 3"/></svg>' },
  immich: { label: 'Immich', icon: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M4 5h16v6H4zM4 13h16v6H4zM8 8h.01M8 16h.01"/></svg>' },
  folder: { label: 'Folder', icon: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/></svg>' },
  url: { label: 'Image URL', icon: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M5 19h.01M4 11a9 9 0 0 1 9 9M4 4a16 16 0 0 1 16 16"/></svg>' },
};
const orders = {
  shuffle: ['Shuffle', 'No repeats until all shown'],
  newest: ['Newest first', 'By date taken'],
  oldest: ['Oldest first', 'By date taken'],
  onthisday: ['On this day', 'Same date, past years'],
};

function switchHTML(on, label, attrs = '') {
  return `<button type="button" class="switch" role="switch" aria-checked="${on}" aria-label="${esc(label)}" ${attrs}><span></span></button>`;
}
function chips(items, current, attr, cls = '') {
  return items.map(([value, label, hint]) => `
    <button type="button" class="chip ${cls}" ${attr}="${esc(value)}" aria-pressed="${String(value) === String(current)}">
      ${hint ? `<span>${esc(label)}</span><small>${esc(hint)}</small>` : esc(label)}
    </button>`).join('');
}
function seg(items, current, attr) {
  return `<div class="seg" style="grid-template-columns:repeat(${items.length},minmax(0,1fr))">` +
    items.map(([v, l]) => `<button type="button" ${attr}="${esc(v)}" aria-pressed="${v === current}">${esc(l)}</button>`).join('') +
    '</div>';
}
function errorCard(err, retry) {
  return `<div class="card"><h2>Can’t reach the frame</h2><p class="hint err">${esc(err.message)}</p>
    <button class="btn" data-retry>${retry || 'Try again'}</button></div>`;
}

// ---------- Now ----------

function NowView(el) {
  let status, settings, timer, previewKey;
  const img = new Image();
  img.alt = 'What the frame is showing';

  async function load() {
    try {
      [status, settings] = await Promise.all([api.get('/api/status?upNext=4'), settings || api.get('/api/settings')]);
      render();
    } catch (err) {
      el.innerHTML = errorCard(err);
    }
  }

  function render() {
    const p = status.player;
    const cur = p.current;
    const src = cur && settings.sources.find((s) => s.id === cur.sourceId);
    const portrait = settings.display.orientation === 'portrait';

    let title = 'Add photos to start';
    if (p.refreshing) title = 'Updating the frame…';
    else if (p.screen === 'clear') title = 'Screen cleared · paused';
    else if (p.paused) title = 'Paused · frame holds this photo';
    else if (p.nextAt) title = `Next photo ${relTime(p.nextAt)}`;
    let progress = p.paused ? 100 : 0;
    if (!p.paused && p.nextAt && p.shownAt) {
      const span = new Date(p.nextAt) - new Date(p.shownAt);
      progress = Math.min(100, Math.max(0, ((Date.now() - new Date(p.shownAt)) / span) * 100));
    }
    const q = settings.quiet;

    el.innerHTML = `
      <header class="page">
        <div>
          <div class="sub"><span class="dot${p.error ? ' off' : ''}"></span>${p.error ? 'Problem' : 'Online'} · ${esc(location.host)}</div>
          <h1>${esc(status.name)}</h1>
        </div>
      </header>
      <section class="card tight">
        <div class="preview${portrait ? ' portrait' : ''}" id="pv">
          ${p.screen ? `<span class="badge">${p.refreshing ? 'Updating…' : p.panelMissing ? 'Preview · no panel' : 'E-ink preview'}</span>` : '<div class="empty">Nothing on the frame yet</div>'}
        </div>
        ${cur ? `<div class="row photo-meta">
          <span class="filename" title="${esc(cur.name || '')}">${esc(cur.name || '')}</span>
          <button type="button" class="link-btn" data-adjust="${esc(cur.id)}">${icons.adjust}Adjust</button>
        </div>` : ''}
        <div class="row" style="padding:0 4px">
          <div class="grow">
            <div style="font-weight:600">${esc(cur ? (cur.title || monthYear(cur.takenAt) || 'Untitled') : (p.screen === 'welcome' ? 'Welcome screen' : '—'))}</div>
            <div class="hint">${esc([cur && cur.title ? monthYear(cur.takenAt) : '', src ? src.name : ''].filter(Boolean).join(' · '))}</div>
          </div>
          <div class="hint">${plural(p.total, 'photo')}</div>
        </div>
        <div class="grid g3">
          <button class="btn" data-cmd="prev">${icons.prev}Back</button>
          <button class="btn" data-cmd="${p.paused ? 'resume' : 'pause'}" aria-pressed="${p.paused}">${p.paused ? icons.play + 'Resume' : icons.pause + 'Pause'}</button>
          <button class="btn primary" data-cmd="next">Next${icons.next}</button>
        </div>
      </section>
      <section class="card">
        <div class="row"><h2>${esc(title)}</h2><a href="#/slideshow" style="font-size:13px;text-decoration:none">Edit</a></div>
        <div class="bar"><div style="width:${progress}%"></div></div>
        <p class="hint">${esc(fmtInterval(settings.interval))} · ${esc(orders[settings.order][0])}${q.enabled ? ` · Quiet ${q.from}–${q.until}` : ''}</p>
        ${p.error ? `<p class="hint err">${esc(p.error)}</p>` : ''}
        ${p.panelMissing ? '<p class="hint">No panel is connected, so the slideshow runs in the app only. Attach the Inky Impression with the Pi powered off; the next photo appears on it after it boots.</p>' : ''}
      </section>
      <section style="display:flex;flex-direction:column;gap:10px">
        <div class="row"><h2 style="margin:0;font-size:15px">Up next</h2><a href="#/sources" style="font-size:13px;text-decoration:none">Manage sources</a></div>
        ${status.upNext.length ? `<div class="thumbs">${status.upNext.map((ph) =>
          `<button type="button" data-adjust="${esc(ph.id)}" title="${esc(ph.name || '')}" aria-label="Adjust ${esc(ph.name || ph.title || 'queued photo')}">
            <img loading="lazy" src="/api/photos/${encodeURIComponent(ph.id)}/thumb" alt=""></button>`).join('')}</div>`
          : '<p class="hint">Nothing queued. Add photos in Sources.</p>'}
      </section>`;

    const key = `${p.screen}|${p.shownAt}|${cur ? cur.id : ''}|${p.refreshing}`;
    if (p.screen && key !== previewKey && !p.refreshing) {
      previewKey = key;
      img.src = `/api/preview.png?k=${encodeURIComponent(key)}`;
    }
    if (p.screen) $('#pv', el).prepend(img);
  }

  el.onclick = async (e) => {
    if (e.target.closest('[data-retry]')) return load();
    const adj = e.target.closest('[data-adjust]');
    if (adj) return photoEditor(adj.dataset.adjust, () => setTimeout(load, 600));
    const b = e.target.closest('[data-cmd]');
    if (!b) return;
    b.disabled = true;
    try {
      await api.post('/api/control/' + b.dataset.cmd);
      toast({ next: 'Next photo coming up', prev: 'Going back', pause: 'Paused', resume: 'Resumed' }[b.dataset.cmd]);
      setTimeout(load, 600);
    } catch (err) {
      toast(err.message, true);
      b.disabled = false;
    }
  };

  load();
  timer = setInterval(() => { if (!document.hidden) load(); }, 5000);
  return { destroy() { clearInterval(timer); el.onclick = null; } };
}

// ---------- Sources ----------

function SourcesView(el) {
  let sources = [];
  let upload = null; // {done, total}

  async function load() {
    try {
      sources = await api.get('/api/sources');
      render();
    } catch (err) {
      el.innerHTML = errorCard(err);
    }
  }

  function meta(s) {
    const t = sourceTypes[s.type]?.label || s.type;
    if (!s.enabled) return `${t} · paused, not in rotation`;
    const st = s.status || {};
    if (st.syncing) return `${t} · syncing…`;
    if (st.error) return `${t} · ${st.error}`;
    return `${t} · ${plural(st.count || 0, 'photo')} · ${ago(st.lastSync)}`;
  }

  function render() {
    const active = sources.filter((s) => s.enabled);
    const total = active.reduce((n, s) => n + (s.status?.count || 0), 0);
    const uploadSrc = sources.find((s) => s.type === 'upload' && s.enabled) || sources.find((s) => s.type === 'upload');
    el.innerHTML = `
      <header class="page"><div><h1>Photo sources</h1>
        <div class="sub">${plural(total, 'photo')} in rotation from ${active.length} of ${plural(sources.length, 'source')}</div></div></header>
      ${sources.length ? `<ul class="list">${sources.map((s) => `
        <li>
          <span class="tile${s.enabled ? '' : ' off'}">${sourceTypes[s.type]?.icon || ''}</span>
          <button class="open" data-edit="${esc(s.id)}" aria-label="Edit ${esc(s.name)}">
            <span class="name">${esc(s.name)}</span>
            <span class="meta${s.status?.error && s.enabled ? ' err' : ''}">${esc(meta(s))}</span>
          </button>
          ${s.enabled && s.type !== 'upload' ? `<button class="icon-btn" data-sync="${esc(s.id)}" aria-label="Sync ${esc(s.name)} now">${icons.sync}</button>` : ''}
          ${switchHTML(s.enabled, `Include ${s.name}`, `data-toggle="${esc(s.id)}"`)}
        </li>`).join('')}</ul>` : '<div class="card"><p class="hint">No sources yet. Add one to start the slideshow.</p></div>'}
      <div class="grid g2">
        <button class="btn primary" data-add>${icons.plus}Add source</button>
        <label class="btn${uploadSrc ? '' : ' disabled'}" ${uploadSrc ? '' : 'aria-disabled="true"'}>
          ${icons.upload}Upload photos
          <input type="file" class="sr-only" multiple accept="image/jpeg,image/png,image/gif,image/webp" data-upload="${esc(uploadSrc?.id || '')}" ${uploadSrc ? '' : 'disabled'}>
        </label>
      </div>
      ${upload ? `<section class="card"><div class="row"><h2>Uploading</h2><span class="hint">${upload.done} of ${upload.total}</span></div>
        <div class="bar"><div style="width:${(upload.pct || 0).toFixed(0)}%"></div></div></section>` : ''}
      <p class="hint">Photos are resized and dithered on the frame, and remote sources are cached on its SD card, so the slideshow keeps going when a source is offline.</p>`;
  }

  async function toggle(id) {
    const s = sources.find((x) => x.id === id);
    try {
      await api.put('/api/sources/' + encodeURIComponent(id), sourceBody({ ...s, enabled: !s.enabled, apiKey: '' }));
      await load();
    } catch (err) { toast(err.message, true); }
  }

  async function sync(id) {
    try {
      await api.post(`/api/sources/${encodeURIComponent(id)}/sync`);
      toast('Syncing…');
      setTimeout(load, 1500);
    } catch (err) { toast(err.message, true); }
  }

  function uploadFiles(id, files) {
    const list = [...files];
    if (!list.length) return;
    upload = { done: 0, total: list.length, pct: 0 };
    render();
    const batches = [];
    for (let i = 0; i < list.length; i += 8) batches.push(list.slice(i, i + 8));
    let rejected = [];
    const sendBatch = (i) => new Promise((resolve, reject) => {
      const fd = new FormData();
      batches[i].forEach((f) => fd.append('photos', f, f.name));
      const xhr = new XMLHttpRequest();
      xhr.open('POST', `/api/sources/${encodeURIComponent(id)}/photos`);
      xhr.upload.onprogress = (e) => {
        if (e.lengthComputable) {
          upload.pct = ((upload.done + (e.loaded / e.total) * batches[i].length) / upload.total) * 100;
          render();
        }
      };
      xhr.onload = () => {
        let body = {};
        try { body = JSON.parse(xhr.responseText); } catch { /* ignore */ }
        if (xhr.status >= 500 || xhr.status === 0) return reject(new Error(body.error || 'Upload failed'));
        rejected = rejected.concat(body.rejected || []);
        resolve();
      };
      xhr.onerror = () => reject(new Error('Upload failed: connection lost'));
      xhr.send(fd);
    });
    (async () => {
      try {
        for (let i = 0; i < batches.length; i++) {
          await sendBatch(i);
          upload.done += batches[i].length;
        }
        toast(rejected.length ? `${plural(list.length - rejected.length, 'photo')} added; ${rejected.length} skipped` : `${plural(list.length, 'photo')} added`, rejected.length > 0);
      } catch (err) {
        toast(err.message, true);
      }
      upload = null;
      load();
    })();
  }

  el.onclick = (e) => {
    const t = e.target.closest('button');
    if (!t) return;
    if (t.matches('[data-retry]')) load();
    else if (t.dataset.toggle) toggle(t.dataset.toggle);
    else if (t.dataset.sync) sync(t.dataset.sync);
    else if (t.dataset.edit) sourceDialog(sources.find((s) => s.id === t.dataset.edit), load);
    else if (t.matches('[data-add]')) sourceDialog(null, load);
  };
  el.onchange = (e) => {
    if (e.target.dataset.upload) uploadFiles(e.target.dataset.upload, e.target.files);
  };

  load();
  const timer = setInterval(() => { if (!document.hidden && !upload) load(); }, 10000);
  return { destroy() { clearInterval(timer); el.onclick = el.onchange = null; } };
}

function sourceDialog(existing, onDone) {
  const dlg = $('#dialog');
  const editing = !!existing;
  let type = existing?.type || 'immich';

  function fields() {
    const v = existing || {};
    switch (type) {
      case 'immich': return `
        <label class="field">Server URL<input name="url" type="url" required placeholder="http://immich.local:2283" value="${esc(v.url)}"></label>
        <label class="field">API key<input name="apiKey" type="password" autocomplete="off" ${editing && v.apiKeySet ? 'placeholder="Stored — leave blank to keep"' : 'required'}></label>
        <label class="field">Album ID<input name="albumId" required placeholder="From the album link: …/albums/<id>" value="${esc(v.albumId)}"></label>
        <p class="hint">Create an API key in Immich under Account Settings → API Keys. Immich’s preview-size images are downloaded and cached on the frame.</p>`;
      case 'folder': return `
        <label class="field">Folder on the frame<input name="path" required placeholder="/mnt/photos" value="${esc(v.path)}"></label>
        <p class="hint">Any folder on the Pi, including a NAS share mounted there (SMB or NFS via /etc/fstab). Sub-folders are included and their names are used as captions.</p>`;
      case 'url': return `
        <label class="field">Image URL<input name="url" type="url" required placeholder="https://example.com/today.jpg" value="${esc(v.url)}"></label>
        <p class="hint">Fetched on every sync; when the image changes, the new one joins the rotation.</p>`;
      default: return '<p class="hint">Photos you upload from this app are stored on the frame’s SD card.</p>';
    }
  }

  function render() {
    dlg.innerHTML = `
      <form method="dialog">
        <h2>${editing ? 'Edit source' : 'Add source'}</h2>
        ${editing ? '' : `<label class="field">Type<select name="type">
          ${Object.entries({ immich: 'Immich album', folder: 'Folder or NAS share', url: 'Image URL', upload: 'Upload folder' })
            .map(([k, l]) => `<option value="${k}" ${k === type ? 'selected' : ''}>${l}</option>`).join('')}</select></label>`}
        <label class="field">Name<input name="name" required maxlength="64" value="${esc(existing?.name || '')}" placeholder="Family album"></label>
        ${fields()}
        <p class="hint err" data-error hidden></p>
        <div class="actions">
          ${editing ? `<button type="button" class="btn danger small" data-delete>${icons.trash}Remove</button>` : ''}
          <span class="spacer"></span>
          <button type="button" class="btn small" data-cancel>Cancel</button>
          <button type="submit" class="btn primary small">${editing ? 'Save' : 'Add'}</button>
        </div>
      </form>`;
  }

  render();
  dlg.onchange = (e) => {
    if (e.target.name === 'type') { type = e.target.value; render(); }
  };
  dlg.onclick = async (e) => {
    if (e.target.closest('[data-cancel]')) dlg.close();
    if (e.target.closest('[data-delete]')) {
      if (!confirm(`Remove “${existing.name}”? Its photos leave the rotation. Uploaded files stay on the SD card.`)) return;
      try { await api.del('/api/sources/' + encodeURIComponent(existing.id)); dlg.close(); onDone(); } catch (err) { toast(err.message, true); }
    }
  };
  dlg.onsubmit = async (e) => {
    e.preventDefault();
    const f = new FormData(e.target);
    const body = sourceBody({
      ...(existing || { enabled: true }),
      type,
      name: f.get('name').trim(),
      url: (f.get('url') || '').trim() || undefined,
      path: (f.get('path') || '').trim() || undefined,
      albumId: (f.get('albumId') || '').trim() || undefined,
      apiKey: (f.get('apiKey') || '').trim() || undefined,
    });
    try {
      if (editing) await api.put('/api/sources/' + encodeURIComponent(existing.id), body);
      else { delete body.id; await api.post('/api/sources', body); }
      dlg.close();
      toast(editing ? 'Saved' : 'Source added — syncing');
      onDone();
    } catch (err) {
      const p = $('[data-error]', dlg);
      p.textContent = err.message;
      p.hidden = false;
    }
  };
  dlg.showModal();
}

// ---------- photo editor ----------

// The panel's logical size; only its aspect ratio matters here.
const PANEL = { w: 800, h: 480 };
const fitModes = [['', 'Slideshow'], ['smart', 'Smart'], ['fill', 'Fill'], ['fit', 'Fit']];
const fitNames = { smart: 'smart crop', fill: 'fill', fit: 'fit + border' };

// viewWindow mirrors render.View on the frame: the region of a sw×sh photo,
// in photo pixels, that fills a w×h output. pos works like CSS
// object-position (0 = left/top edges aligned, 1 = right/bottom).
function viewWindow(sw, sh, w, h, mode, zoom, pos) {
  const wider = sw * h > sh * w;
  let vw, vh;
  if (mode === 'fit') [vw, vh] = wider ? [sw, (sw * h) / w] : [(sh * w) / h, sh];
  else [vw, vh] = wider ? [(sh * w) / h, sh] : [sw, (sw * h) / w];
  vw /= zoom; vh /= zoom;
  const p = pos || { x: 0.5, y: 0.5 };
  return { x: p.x * (sw - vw), y: p.y * (sh - vh), w: vw, h: vh };
}

function rotateImage(img, deg) {
  const c = document.createElement('canvas');
  const swap = deg % 180 !== 0;
  c.width = swap ? img.naturalHeight : img.naturalWidth;
  c.height = swap ? img.naturalWidth : img.naturalHeight;
  const ctx = c.getContext('2d');
  ctx.translate(c.width / 2, c.height / 2);
  ctx.rotate((deg * Math.PI) / 180);
  ctx.drawImage(img, -img.naturalWidth / 2, -img.naturalHeight / 2);
  return c;
}

// adjustmentBody sends only the fields that change something.
function adjustmentBody(a) {
  const out = {};
  if (a.rotate) out.rotate = a.rotate;
  if (a.fit) out.fit = a.fit;
  if (a.zoom && a.zoom !== 1) out.zoom = a.zoom;
  if (a.position) out.position = a.position;
  if (a.brightness) out.brightness = a.brightness;
  return out;
}

// photoEditor opens a sheet to rotate, frame, zoom, align and brighten one
// photo. The frame remembers the result for that photo.
async function photoEditor(id, onDone) {
  const dlg = $('#dialog');
  const path = `/api/photos/${encodeURIComponent(id)}`;
  let view, settings;
  try {
    [view, settings] = await Promise.all([api.get(path + '/adjustment'), api.get('/api/settings')]);
  } catch (err) { toast(err.message, true); return; }

  const ph = view.photo;
  const a = { rotate: 0, fit: '', zoom: 1, position: null, brightness: 0, ...view.adjustment };
  const portrait = settings.display.orientation === 'portrait';
  const [outW, outH] = portrait ? [PANEL.h, PANEL.w] : [PANEL.w, PANEL.h];
  const border = settings.border === 'black' ? '#000' : '#fff';
  const pct = (v) => `${v > 0 ? '+' : ''}${Math.round(v * 100)}%`;
  const mode = () => a.fit || settings.fit;
  let rotated = null;
  let frame = 0;

  const img = new Image();
  img.onload = () => { rotated = rotateImage(img, a.rotate); draw(); };
  img.onerror = () => { const s = $('[data-status]', dlg); if (s) s.textContent = 'Could not load the photo.'; };
  img.src = path + '/image';

  function alignGrid() {
    const cells = [];
    for (const y of [0, 0.5, 1]) {
      for (const x of [0, 0.5, 1]) {
        const on = a.position && a.position.x === x && a.position.y === y;
        const label = `${['Top', 'Middle', 'Bottom'][y * 2]} ${['left', 'centre', 'right'][x * 2]}`;
        cells.push(`<button type="button" data-align="${x},${y}" aria-pressed="${!!on}" aria-label="${label}" title="${label}"><span></span></button>`);
      }
    }
    return `<div class="align-grid">${cells.join('')}</div>`;
  }

  function hint() {
    const m = mode();
    if (m === 'smart' && !a.position) return 'Smart crop picks the position on the frame. Drag the photo or tap an alignment to choose it yourself.';
    if (m === 'fit') return 'The whole photo is shown with a border. Drag or tap an alignment to move it within the frame.';
    return 'Drag the photo to choose what the frame shows.';
  }

  function render() {
    dlg.innerHTML = `
      <form method="dialog" class="editor">
        <h2>Adjust photo</h2>
        <div class="editor-stage${portrait ? ' portrait' : ''}" style="background:${border}">
          <canvas data-canvas aria-label="Preview of the framed photo; drag to move it"></canvas>
        </div>
        <div class="filename" title="${esc(ph.name || '')}" data-status>${esc(ph.name || '')}</div>
        <p class="hint" data-hint>${esc(hint())}</p>
        <div class="row">
          <span class="field">Rotate</span>
          <div style="display:flex;gap:8px">
            <button type="button" class="icon-btn" data-rotate="270" aria-label="Rotate left">${icons.rotl}</button>
            <button type="button" class="icon-btn" data-rotate="90" aria-label="Rotate right">${icons.rotr}</button>
          </div>
        </div>
        <div class="field">Framing</div>
        ${seg(fitModes, a.fit, 'data-fit')}
        <p class="hint">${a.fit ? `This photo always uses ${fitNames[a.fit]}.` : `Follows the slideshow setting, now ${fitNames[settings.fit]}.`}</p>
        <div class="row align-row">
          <div class="grow">
            <label class="field"><span class="range-label">Zoom <b data-zoomv>${a.zoom.toFixed(1)}×</b></span>
              <input type="range" min="1" max="4" step="0.1" value="${a.zoom}" data-zoom></label>
            <label class="field"><span class="range-label">Brightness <b data-brightv>${pct(a.brightness)}</b></span>
              <input type="range" min="-50" max="50" step="5" value="${Math.round(a.brightness * 100)}" data-bright></label>
          </div>
          <div><div class="field" style="margin-bottom:4px">Align</div>${alignGrid()}</div>
        </div>
        <div class="actions">
          <button type="button" class="btn small" data-reset>Reset</button>
          <span class="spacer"></span>
          <button type="button" class="btn small" data-cancel>Cancel</button>
          <button type="submit" class="btn primary small">Save</button>
        </div>
      </form>`;
    bindDrag();
    draw();
  }

  // draw paints the preview on the next animation frame.
  function draw() {
    cancelAnimationFrame(frame);
    frame = requestAnimationFrame(paint);
  }
  function paint() {
    const c = $('[data-canvas]', dlg);
    if (!c || !rotated) return;
    const dpr = Math.min(window.devicePixelRatio || 1, 2);
    const cw = c.clientWidth;
    c.width = Math.max(1, Math.round(cw * dpr));
    c.height = Math.max(1, Math.round(((cw * outH) / outW) * dpr));
    const ctx = c.getContext('2d');
    ctx.fillStyle = border;
    ctx.fillRect(0, 0, c.width, c.height);
    const win = viewWindow(rotated.width, rotated.height, outW, outH, mode(), a.zoom, a.position);
    const k = c.width / win.w;
    ctx.imageSmoothingQuality = 'high';
    ctx.drawImage(rotated, -win.x * k, -win.y * k, rotated.width * k, rotated.height * k);
    if (a.brightness) {
      // Same as render.Brighten: shift every channel by brightness × 255.
      const d = ctx.getImageData(0, 0, c.width, c.height);
      const px = d.data;
      const shift = a.brightness * 255;
      for (let i = 0; i < px.length; i += 4) {
        px[i] += shift; px[i + 1] += shift; px[i + 2] += shift; // Uint8ClampedArray clamps
      }
      ctx.putImageData(d, 0, 0);
    }
  }

  function bindDrag() {
    const c = $('[data-canvas]', dlg);
    let start = null;
    c.onpointerdown = (e) => {
      if (!rotated) return;
      c.setPointerCapture(e.pointerId);
      const win = viewWindow(rotated.width, rotated.height, outW, outH, mode(), a.zoom, a.position);
      const p = a.position || { x: 0.5, y: 0.5 };
      start = { x: e.clientX, y: e.clientY, p, win, scale: win.w / c.clientWidth };
    };
    c.onpointermove = (e) => {
      if (!start) return;
      // Moving the photo right moves the window left over it.
      const travelX = rotated.width - start.win.w;
      const travelY = rotated.height - start.win.h;
      const clamp = (v) => Math.min(1, Math.max(0, v));
      const round = (v) => Math.round(v * 1000) / 1000;
      a.position = {
        x: Math.abs(travelX) < 0.5 ? 0.5 : round(clamp(start.p.x - ((e.clientX - start.x) * start.scale) / travelX)),
        y: Math.abs(travelY) < 0.5 ? 0.5 : round(clamp(start.p.y - ((e.clientY - start.y) * start.scale) / travelY)),
      };
      draw();
    };
    c.onpointerup = c.onpointercancel = () => {
      if (!start) return;
      start = null;
      $('[data-hint]', dlg).textContent = hint();
      dlg.querySelectorAll('[data-align]').forEach((b) => {
        const [x, y] = b.dataset.align.split(',').map(Number);
        b.setAttribute('aria-pressed', String(!!a.position && a.position.x === x && a.position.y === y));
      });
    };
  }

  function close() {
    cancelAnimationFrame(frame);
    window.removeEventListener('resize', draw);
    dlg.close();
  }

  dlg.onclick = async (e) => {
    const b = e.target.closest('button');
    if (!b) return;
    if (b.matches('[data-cancel]')) close();
    else if (b.dataset.rotate) {
      a.rotate = (a.rotate + Number(b.dataset.rotate)) % 360;
      if (img.complete && img.naturalWidth) rotated = rotateImage(img, a.rotate);
      render();
    } else if (b.dataset.fit !== undefined) {
      a.fit = b.dataset.fit;
      render();
    } else if (b.dataset.align) {
      const [x, y] = b.dataset.align.split(',').map(Number);
      a.position = { x, y };
      render();
    } else if (b.matches('[data-reset]')) {
      Object.assign(a, { rotate: 0, fit: '', zoom: 1, position: null, brightness: 0 });
      if (img.complete && img.naturalWidth) rotated = rotateImage(img, 0);
      render();
    }
  };
  dlg.oninput = (e) => {
    const t = e.target;
    if (t.matches('[data-zoom]')) {
      a.zoom = Number(t.value);
      $('[data-zoomv]', dlg).textContent = `${a.zoom.toFixed(1)}×`;
      draw();
    } else if (t.matches('[data-bright]')) {
      a.brightness = Number(t.value) / 100;
      $('[data-brightv]', dlg).textContent = pct(a.brightness);
      draw();
    }
  };
  dlg.onchange = null;
  dlg.onsubmit = async (e) => {
    e.preventDefault();
    try {
      const body = adjustmentBody(a);
      const out = Object.keys(body).length ? await api.put(path + '/adjustment', body) : await api.del(path + '/adjustment');
      close();
      toast(out?.onScreen ? 'Saved — the frame is updating' : 'Saved for the next time this photo is shown');
      onDone?.();
    } catch (err) { toast(err.message, true); }
  };
  dlg.onclose = () => { cancelAnimationFrame(frame); window.removeEventListener('resize', draw); };
  window.addEventListener('resize', draw);
  render();
  dlg.showModal();
  draw(); // the canvas has a size only once the dialog is open
}

// ---------- Settings-backed views ----------

// settingsView loads settings, renders with build(settings), and saves edits.
function settingsView(el, build, bind, extraLoad) {
  let settings, extra;
  const save = debounce(async () => {
    try {
      settings = await api.put('/api/settings', settingsBody(settings));
      const s = $('[data-saved]', el);
      if (s) s.textContent = 'Saved · applied to the frame';
    } catch (err) {
      toast(err.message, true);
      await load();
    }
  }, 350);
  const ctx = {
    get s() { return settings; },
    get extra() { return extra; },
    change(fn, rerender = true) { fn(settings); if (rerender) render(); save(); },
  };
  function render() { el.innerHTML = build(settings, extra); }
  async function load() {
    try {
      [settings, extra] = await Promise.all([api.get('/api/settings'), extraLoad ? extraLoad() : null]);
      render();
    } catch (err) { el.innerHTML = errorCard(err); }
  }
  el.onclick = (e) => {
    if (e.target.closest('[data-retry]')) return load();
    bind.click?.(e, ctx);
  };
  el.oninput = (e) => bind.input?.(e, ctx);
  el.onchange = (e) => bind.change?.(e, ctx);
  load();
  return { destroy() { el.onclick = el.oninput = el.onchange = null; } };
}

const intervals = [['15m0s', '15 min'], ['30m0s', '30 min'], ['1h0m0s', '1 hour'], ['3h0m0s', '3 hours'], ['6h0m0s', '6 hours'], ['24h0m0s', 'Daily']];
const intervalNotes = {
  15: 'About 96 refreshes a day. Each full refresh flashes for ~30 s.',
  30: 'About 48 refreshes a day — a calm pace for e-ink.',
  60: 'About 24 refreshes a day.',
  180: 'Eight photos a day.',
  360: 'Four photos a day.',
  1440: 'One photo a day.',
};

function SlideshowView(el) {
  return settingsView(el, (s) => {
    const cur = intervals.find(([v]) => minutes(v) === minutes(s.interval))?.[0] || s.interval;
    return `
      <header class="page"><div><h1>Slideshow</h1><div class="sub" data-saved>Changes apply to the frame instantly</div></div></header>
      <section class="card">
        <h2>Change photo every</h2>
        <div class="grid g3">${chips(intervals, cur, 'data-interval')}</div>
        <p class="hint">${esc(intervalNotes[minutes(s.interval)] || fmtInterval(s.interval))}</p>
      </section>
      <section class="card">
        <h2>Order</h2>
        <div class="grid g2">${chips(Object.entries(orders).map(([k, [l, h]]) => [k, l, h]), s.order, 'data-order', 'wide')}</div>
      </section>
      <section class="card">
        <h2>Framing</h2>
        <div class="grid g3">${chips([['smart', 'Smart crop'], ['fill', 'Fill'], ['fit', 'Fit + border']], s.fit, 'data-fit')}</div>
        ${s.fit === 'fit' ? `<div class="row"><span>Border</span>${seg([['white', 'White'], ['black', 'Black']], s.border, 'data-border')}</div>` : ''}
        <p class="hint">${{ smart: 'Crops to fill the panel, keeping the most detailed part of the photo.', fill: 'Crops to fill the panel around the centre.', fit: 'Shows the whole photo with a border.' }[s.fit]}</p>
        <div class="row">
          <div class="grow"><div>Show date &amp; place</div><div class="hint">Small caption, bottom left</div></div>
          ${switchHTML(s.caption, 'Show date and place', 'data-caption')}
        </div>
      </section>
      <section class="card">
        <div class="row">
          <div class="grow"><h2>Quiet hours</h2><div class="hint">No refreshes; the last photo stays up</div></div>
          ${switchHTML(s.quiet.enabled, 'Quiet hours', 'data-quiet')}
        </div>
        ${s.quiet.enabled ? `<div class="grid g2">
          <label class="field">From<input type="time" required value="${esc(s.quiet.from)}" data-qfrom></label>
          <label class="field">Until<input type="time" required value="${esc(s.quiet.until)}" data-quntil></label>
        </div>` : ''}
      </section>`;
  }, {
    click(e, c) {
      const b = e.target.closest('button');
      if (!b) return;
      if (b.dataset.interval) c.change((s) => { s.interval = b.dataset.interval; });
      else if (b.dataset.order) c.change((s) => { s.order = b.dataset.order; });
      else if (b.dataset.fit) c.change((s) => { s.fit = b.dataset.fit; });
      else if (b.dataset.border) c.change((s) => { s.border = b.dataset.border; });
      else if (b.hasAttribute('data-caption')) c.change((s) => { s.caption = !s.caption; });
      else if (b.hasAttribute('data-quiet')) c.change((s) => { s.quiet.enabled = !s.quiet.enabled; });
    },
    change(e, c) {
      if (e.target.matches('[data-qfrom]') && e.target.value) c.change((s) => { s.quiet.from = e.target.value; }, false);
      if (e.target.matches('[data-quntil]') && e.target.value) c.change((s) => { s.quiet.until = e.target.value; }, false);
    },
  });
}

const inks = {
  spectra6: [['Black', '#000000'], ['White', '#D0D1D2'], ['Yellow', '#E7DE23'], ['Red', '#CD2425'], ['Blue', '#1E1DAE'], ['Green', '#1DAD23']],
  acep7: [['Black', '#000000'], ['White', '#ECF8FF'], ['Green', '#01BD26'], ['Blue', '#0D17E2'], ['Red', '#FA2811'], ['Yellow', '#FFFF22'], ['Orange', '#F78216']],
};

function signalLabel(dbm) {
  if (dbm == null) return '';
  if (dbm >= -60) return 'good signal';
  if (dbm >= -70) return 'fair signal';
  return 'weak signal';
}
function uptime(sec) {
  if (!sec) return '';
  const d = Math.floor(sec / 86400);
  const h = Math.floor((sec % 86400) / 3600);
  return d ? `${plural(d, 'day')}${h ? `, ${h} h` : ''}` : `${h} h ${Math.floor((sec % 3600) / 60)} min`;
}

function DeviceView(el) {
  const pct = (v) => `${v > 0 ? '+' : ''}${Math.round(v * 100)}%`;
  return settingsView(el, (s, dev) => `
    <header class="page"><div><h1>Display &amp; device</h1><div class="sub" data-saved>Inky Impression 7.3″ · 800 × 480</div></div></header>
    <section class="card">
      <h2>Frame</h2>
      <label class="field">Name<input value="${esc(s.name)}" maxlength="64" data-name></label>
      <div class="field">Orientation</div>
      ${seg([['landscape', 'Landscape'], ['portrait', 'Portrait']], s.display.orientation, 'data-orientation')}
      <div class="field">Panel</div>
      ${seg([['spectra6', 'Spectra 6'], ['acep7', '7-colour ACeP']], s.display.panel, 'data-panel')}
      <div style="display:flex;gap:6px">${inks[s.display.panel].map(([n, c]) =>
        `<span title="${n}" style="width:28px;height:28px;border-radius:6px;background:${c};box-shadow:inset 0 0 0 1px rgba(0,0,0,.15)"></span>`).join('')}</div>
      <p class="hint">Pick the panel you have: the 2025 edition is Spectra 6, the original is 7-colour.</p>
    </section>
    <section class="card">
      <h2>Image tuning</h2>
      <div class="field">Dithering</div>
      ${seg([['fs', 'Floyd–Steinberg'], ['atkinson', 'Atkinson'], ['ordered', 'Ordered']], s.display.dither, 'data-dither')}
      <label class="field"><span class="range-label">Saturation <b data-satv>${pct(s.display.saturation)}</b></span>
        <input type="range" min="-100" max="100" step="5" value="${Math.round(s.display.saturation * 100)}" data-sat></label>
      <label class="field"><span class="range-label">Contrast <b data-conv>${pct(s.display.contrast)}</b></span>
        <input type="range" min="-100" max="100" step="5" value="${Math.round(s.display.contrast * 100)}" data-con></label>
      <button class="btn small" data-cmd="refresh">Re-render current photo</button>
      <p class="hint">Tuning applies from the next photo; re-render to see it now (takes about 30 s).</p>
    </section>
    <section class="card">
      <h2>Frame buttons</h2>
      <dl class="keys"><dt>A</dt><dd>Next photo</dd><dt>B</dt><dd>Previous photo</dd><dt>C</dt><dd>Pause / resume</dd><dt>D</dt><dd>Show how to open this app</dd></dl>
    </section>
    <section class="card">
      <h2>${esc(dev?.model || 'Raspberry Pi')}</h2>
      <dl class="facts">
        <dt>Address</dt><dd>${esc([dev?.hostname ? dev.hostname + '.local' : '', ...(dev?.addresses || [])].filter(Boolean).join(' · '))}</dd>
        ${dev?.wifiIface ? `<dt>Wi-Fi</dt><dd>${esc(dev.wifiIface)}${dev.wifiSignalDbm != null ? ` · ${dev.wifiSignalDbm} dBm, ${signalLabel(dev.wifiSignalDbm)}` : ''}</dd>` : ''}
        ${dev?.cpuTempC != null ? `<dt>CPU temp</dt><dd>${dev.cpuTempC.toFixed(0)} °C</dd>` : ''}
        ${dev?.uptimeSec ? `<dt>Uptime</dt><dd>${uptime(dev.uptimeSec)}</dd>` : ''}
      </dl>
      <div class="grid g3">
        <button class="btn small" data-cmd="info">Show info</button>
        <button class="btn small" data-restart>Restart</button>
        <button class="btn danger small" data-cmd="clear">Clear screen</button>
      </div>
    </section>
    ${aboutCard(dev)}`, {
    async click(e, c) {
      const b = e.target.closest('button');
      if (!b) return;
      if (b.hasAttribute('data-devmark')) showDevFlash();
      else if (b.dataset.orientation) c.change((s) => { s.display.orientation = b.dataset.orientation; });
      else if (b.dataset.panel) c.change((s) => { s.display.panel = b.dataset.panel; });
      else if (b.dataset.dither) c.change((s) => { s.display.dither = b.dataset.dither; });
      else if (b.dataset.cmd) {
        if (b.dataset.cmd === 'clear' && !confirm('Blank the panel and pause the slideshow? Press Resume to continue.')) return;
        try {
          await api.post('/api/control/' + b.dataset.cmd);
          toast({ refresh: 'Re-rendering — the frame will flash', info: 'Showing info on the frame', clear: 'Clearing the screen' }[b.dataset.cmd]);
        } catch (err) { toast(err.message, true); }
      } else if (b.hasAttribute('data-restart')) {
        if (!confirm('Restart the photoframe service? The app reconnects in a few seconds.')) return;
        try { await api.post('/api/device/restart'); toast('Restarting…'); } catch (err) { toast(err.message, true); }
      }
    },
    input(e, c) {
      const t = e.target;
      if (t.matches('[data-sat]')) { c.change((s) => { s.display.saturation = t.value / 100; }, false); $('[data-satv]', el).textContent = pct(t.value / 100); }
      if (t.matches('[data-con]')) { c.change((s) => { s.display.contrast = t.value / 100; }, false); $('[data-conv]', el).textContent = pct(t.value / 100); }
    },
    change(e, c) {
      if (e.target.matches('[data-name]') && e.target.value.trim()) c.change((s) => { s.name = e.target.value.trim(); }, false);
    },
  }, () => api.get('/api/device').catch(() => null));
}

// ---------- about ----------

// The brand lockup over the running version, with the developer badge beside
// it. The version comes from the frame (/api/device), so it is the version of
// the software actually running, not of the page that was cached.
function aboutCard(dev) {
  const v = dev?.version || '';
  // Patch 0 is an unstamped development build (see internal/version).
  const devBuild = /\.0$/.test(v);
  return `
    <section class="card about">
      <div class="row">
        <div class="brand grow">
          <img src="/icons/icon.svg" alt="" width="40" height="40">
          <div><div class="brand-name">Photoframe</div>
            <div class="brand-version">${esc(v)}${dev?.arch ? ` · ${esc(dev.arch)}` : ''}${devBuild ? ' · development build' : ''}</div></div>
        </div>
        <button type="button" class="devmark" data-devmark aria-label="Show the developer badge">
          <img src="/icons/dev-badge.png" alt="" aria-hidden="true" width="36" height="36">
        </button>
      </div>
    </section>`;
}

// How long the full-size badge stays up. Matches the CSS animations, which
// run on their own clock; this removes the overlay.
const DEV_FLASH_MS = 3000;

// Throws the developer badge up full screen for a moment. A tap anywhere or
// Escape ends it early.
function showDevFlash() {
  if ($('.dev-veil')) return;
  const veil = document.createElement('div');
  veil.className = 'dev-veil';
  veil.innerHTML = `
    <div class="dev-lockup">
      <img src="/icons/dev-badge-full.png" alt="Built by CM Hegday — 0x434d">
      <span>github.com/chinmay28</span>
    </div>`;
  const done = () => {
    clearTimeout(timer);
    window.removeEventListener('keydown', onKey);
    veil.remove();
  };
  const onKey = (e) => { if (e.key === 'Escape') done(); };
  const timer = setTimeout(done, DEV_FLASH_MS);
  veil.addEventListener('click', done);
  window.addEventListener('keydown', onKey);
  document.body.append(veil);
}

// ---------- router ----------

const views = { now: NowView, sources: SourcesView, slideshow: SlideshowView, device: DeviceView };
let active;

function route() {
  const name = location.hash.replace(/^#\//, '') || 'now';
  const view = views[name] ? name : 'now';
  active?.destroy?.();
  const el = $('#view');
  el.innerHTML = '';
  document.querySelectorAll('.tabs a').forEach((a) => {
    if (a.dataset.tab === view) a.setAttribute('aria-current', 'page');
    else a.removeAttribute('aria-current');
  });
  active = views[view](el);
  el.focus({ preventScroll: true });
}

window.addEventListener('hashchange', route);
route();

if ('serviceWorker' in navigator) {
  navigator.serviceWorker.register('/sw.js').catch(() => { /* app still works without it */ });
}
