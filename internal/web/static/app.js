/* Payam Logger — web UI (vanilla JS). */
'use strict';

// ---------- state ----------
const S = {
  day: null,           // current day payload
  viewStart: 8 * 3600, viewSpan: 10 * 3600,
  hover: -1, hoverX: 0,
  selStart: -1, selEnd: -1, selPressX: 0,
  active: 'apps',
  panning: false, panStartView: 0,
  viewIdx: -1,
  tsData: [],
};

const DAY = 86400, MINSPAN = 3600, PAD = 6, AUTOPAD = 1800;
const TRACKS = [
  { key: 'shots',    h: 34 },
  { key: 'usage',    h: 22 },
  { key: 'apps',     h: 28 },
  { key: 'tags',     h: 24 },
  { key: 'autotags', h: 22 },
];
const PALETTE = ['#3584e4','#9c66d9','#d3641f','#26a19a','#c14f9a','#968c49','#2b88b3','#d54c4a','#66854f','#8d65a9'];
const css = getComputedStyle(document.documentElement);
const C = {
  blue: '#3584e4', grid: '#d5d0cc', text: '#77767b',
  hover: '#26a269', now: '#e01b24', border: '#ebeae4', chip: '#3a3a38',
  active: '#26a269', away: '#e01b24',
};

const $ = id => document.getElementById(id);
const canvas = $('timeline');
const ctx = canvas.getContext('2d');

// ---------- helpers ----------
function appColor(name) {
  let h = 0;
  for (const ch of (name || '')) h = (h * 31 + ch.charCodeAt(0)) >>> 0;
  return PALETTE[h % PALETTE.length];
}
const firstLevel = n => (n.includes(',') ? n.split(',')[0].trim() : n);
const fmtClock = s => `${String(Math.floor(s / 3600)).padStart(2, '0')}:${String(Math.floor(s % 3600 / 60)).padStart(2, '0')}`;
function fmtDur(sec) {
  sec = Math.round(sec); const h = Math.floor(sec / 3600), m = Math.floor(sec % 3600 / 60), s = sec % 60;
  if (h && m) return `${h}س ${m}د`; if (h) return `${h}س`; if (m) return `${m}د ${s}ث`; return `${s}ث`;
}
const todayStr = () => new Date().toISOString().slice(0, 10);
const parseClock = t => { const [h, m] = t.split(':').map(Number); return h * 3600 + m * 60; };
function trackTop(key) { let y = 18; for (const t of TRACKS) { if (t.key === key) return y; y += t.h + 2; } return y; }
function trackAtY(y) {
  let top = 18;
  for (const t of TRACKS) { if (y >= top && y <= top + t.h) return { key: t.key, top, h: t.h }; top += t.h + 2; }
  return null;
}
const trackData = key => key === 'shots' ? S.day.shots :
  key === 'usage' ? S.day.computerUsage :
  key === 'apps' ? S.day.appUsage :
  key === 'tags' ? S.day.tags :
  S.day.autotagSegments;

// ---------- geometry ----------
function width() { return canvas.clientWidth; }
function pxPerSec() { return (width() - 2 * PAD) / S.viewSpan; }
function xToSec(x) {
  const s = S.viewStart + x / width() * S.viewSpan;
  return Math.max(S.viewStart, Math.min(s, S.viewStart + S.viewSpan));
}
function secToX(sec, w) { return PAD + (sec - S.viewStart) * pxPerSec(); }

function setViewStart(s) {
  const max = DAY - S.viewSpan;
  S.viewStart = Math.max(0, Math.min(s, max));
  draw();
}
function zoomAt(x, dy) {
  const f = Math.pow(1.25, dy);
  const newSpan = Math.max(MINSPAN, Math.min(DAY, S.viewSpan * f));
  const anchor = xToSec(x);
  const rel = (anchor - S.viewStart) / S.viewSpan;
  S.viewSpan = newSpan;
  setViewStart(anchor - rel * newSpan);
}

// snap: sticky to segment boundaries, else whole minutes
function snap(sec) {
  const radius = 8 / pxPerSec();
  let best = sec, bd = Infinity;
  for (const key of ['usage', 'apps', 'tags', 'autotags']) {
    for (const g of trackData(key)) {
      for (const p of [g.start, g.end]) {
        const d = Math.abs(sec - p);
        if (d < bd) { bd = d; best = p; }
      }
    }
  }
  if (bd <= radius) return best;
  return Math.round(sec / 60) * 60;
}

// ---------- data ----------
async function loadDay(day) {
  const res = await fetch(`/api/day/${day}`);
  if (!res.ok) { setStatus('خطا در بارگذاری روز'); return; }
  S.day = await res.json();
  S.selStart = S.selEnd = -1; S.hover = -1; S.viewIdx = -1;
  autoZoom();
  $('viewer').removeAttribute('src');
  $('shotInfo').textContent = '';
  renderLabels(); renderPanels(); dayStatus(); draw();
  const t = todayStr();
  $('nextDay').disabled = day === t;
  const d = new Date(day + 'T00:00:00');
  $('dateLabel').textContent =
    new Intl.DateTimeFormat('fa-IR', { weekday: 'long', year: 'numeric', month: 'long', day: 'numeric' }).format(d) +
    ` (${day})`;
}
function autoZoom() {
  let lo = Infinity, hi = -Infinity;
  const iv = S.day.intervalSec, has = new Set(S.day.shots.map(s => Math.floor(s.t / iv)));
  for (const b of has) { lo = Math.min(lo, b * iv); hi = Math.max(hi, (b + 1) * iv); }
  for (const key of ['usage', 'apps', 'tags', 'autotags'])
    for (const g of trackData(key)) { lo = Math.min(lo, g.start); hi = Math.max(hi, g.end); }
  if (hi < lo) { S.viewStart = 8 * 3600; S.viewSpan = 10 * 3600; return; }
  lo = Math.max(0, lo - AUTOPAD); hi = Math.min(DAY, hi + AUTOPAD);
  if (hi - lo < MINSPAN) { const m = (lo + hi) / 2; lo = m - MINSPAN / 2; hi = m + MINSPAN / 2; }
  S.viewStart = lo; S.viewSpan = hi - lo;
}
function shiftDay(delta) {
  const d = new Date(S.day.day + 'T00:00:00');
  d.setDate(d.getDate() + delta);
  loadDay(d.toISOString().slice(0, 10));
}

// ---------- drawing ----------
function resizeCanvas() {
  const wrap = $('timelineWrap');
  canvas.width = wrap.clientWidth - 88;
  canvas.height = TRACKS.reduce((a, t) => a + t.h + 2, 18);
  draw();
}
function draw() {
  if (!S.day) return;
  const W = canvas.width, H = canvas.height;
  ctx.clearRect(0, 0, W, H);
  drawRuler(W);

  for (const t of TRACKS) drawTrack(t, W);

  // hover line + chip
  if (S.hover >= 0) {
    const x = secToX(S.hover);
    ctx.strokeStyle = C.hover; ctx.lineWidth = 1.5;
    ctx.beginPath(); ctx.moveTo(x, 18); ctx.lineTo(x, H); ctx.stroke();
    chip(fmtClock(S.hover), x, H - 16, C.hover);
  }
}
function drawRuler(W) {
  const steps = [1800, 3600, 7200, 10800, 21600, 43200];
  let step = steps.at(-1);
  for (const s of steps) if (s * pxPerSec() >= 52) { step = s; break; }
  ctx.font = '10px sans-serif'; ctx.fillStyle = C.text; ctx.strokeStyle = C.grid;
  for (let s = Math.ceil(S.viewStart / step) * step; s <= S.viewStart + S.viewSpan; s += step) {
    const x = secToX(s); if (x < PAD || x > W - PAD) continue;
    ctx.beginPath(); ctx.moveTo(x, 3); ctx.lineTo(x, 18); ctx.stroke();
    ctx.fillText(fmtClock(s), x + 3, 12);
  }
}
function drawTrack(t, W) {
  const top = trackTop(t.key), h = t.h;
  // background
  ctx.fillStyle = '#fff'; ctx.fillRect(PAD, top, W - 2 * PAD, h);

  if (t.key === 'shots') {
    const iv = S.day.intervalSec;
    const b0 = Math.max(0, Math.floor(S.viewStart / iv));
    const b1 = Math.min(Math.floor(DAY / iv) - 1, Math.floor((S.viewStart + S.viewSpan) / iv));
    const has = new Set(S.day.shots.map(s => Math.floor(s.t / iv)));
    ctx.fillStyle = C.blue;
    let b = b0;
    while (b <= b1) {
      if (!has.has(b)) { b++; continue; }
      let e = b; while (e + 1 <= b1 && has.has(e + 1)) e++;
      const x0 = secToX(b * iv), x1 = secToX((e + 1) * iv);
      ctx.fillRect(x0, top + 2, Math.max(1, x1 - x0), h - 4);
      b = e + 1;
    }
  } else {
    ctx.font = '10px sans-serif';
    for (const g of trackData(t.key)) {
      const x0 = Math.max(PAD, secToX(g.start)), x1 = Math.min(W - PAD, secToX(g.end));
      if (x1 <= x0) continue;
      let color, label = '';
      if (t.key === 'usage') color = g.state === 'away' ? C.away : C.active;
      else if (t.key === 'apps') { color = appColor(g.app); label = g.app || '(میزکار)'; }
      else { const n = t.key === 'tags' ? g.name : g.name; color = appColor(firstLevel(n)); label = firstLevel(n); }
      ctx.fillStyle = color;
      ctx.fillRect(x0, top + 2, x1 - x0, h - 4);
      if (label && x1 - x0 > 44) {
        ctx.fillStyle = '#fff';
        ctx.save(); ctx.beginPath(); ctx.rect(x0 + 3, top, x1 - x0 - 6, h); ctx.clip();
        ctx.fillText(label, x0 + 5, top + h / 2 + 3.5); ctx.restore();
      }
    }
  }

  // selection overlay
  if (S.selStart >= 0) {
    const lo = Math.min(S.selStart, S.selEnd), hi = Math.max(S.selStart, S.selEnd);
    const x0 = Math.max(PAD, secToX(lo)), x1 = Math.min(W - PAD, secToX(hi));
    if (x1 > x0) {
      ctx.fillStyle = 'rgba(40,40,40,.16)'; ctx.fillRect(x0, top, x1 - x0, h);
      ctx.strokeStyle = C.chip; ctx.lineWidth = 2;
      ctx.beginPath(); ctx.moveTo(x0, top); ctx.lineTo(x0, top + h);
      ctx.moveTo(x1, top); ctx.lineTo(x1, top + h); ctx.stroke();
      const lbl = fmtDur(hi - lo);
      ctx.font = '10px sans-serif';
      chip(lbl, (x0 + x1) / 2, canvas.height - 16, C.chip);
    }
  }

  // now marker
  if (S.day.day === todayStr()) {
    const now = new Date();
    const ns = now.getHours() * 3600 + now.getMinutes() * 60 + now.getSeconds();
    if (ns >= S.viewStart && ns <= S.viewStart + S.viewSpan) {
      const x = secToX(ns);
      ctx.strokeStyle = C.now; ctx.lineWidth = 2.5;
      ctx.beginPath(); ctx.moveTo(x, 18); ctx.lineTo(x, canvas.height); ctx.stroke();
    }
  }

  // border
  ctx.strokeStyle = C.border; ctx.lineWidth = 1;
  ctx.strokeRect(PAD, top, W - 2 * PAD, h);
}
function chip(text, cx, y, color) {
  ctx.font = '10px sans-serif';
  const w = ctx.measureText(text).width + 10;
  let x = Math.min(Math.max(cx - w / 2, PAD), canvas.width - PAD - w);
  ctx.fillStyle = color; ctx.fillRect(x, y, w, 15);
  ctx.fillStyle = '#fff'; ctx.fillText(text, x + 5, y + 11);
}

// ---------- panels ----------
function renderLabels() {
  document.querySelectorAll('.tlabel').forEach(el =>
    el.classList.toggle('active', el.dataset.track === S.active));
}
function renderPanels() {
  const rows = [], groups = new Map();
  const add = (title, start, end, extra, sel) => {
    rows.push({ title, start, end, extra, sel });
  };
  if (S.active === 'apps') {
    for (const g of S.day.appUsage) {
      add(g.title || '(میزکار)', g.start, g.end, g.app || '(میزکار)', [g.start, g.end]);
      const k = g.app || '(میزکار)';
      groups.set(k, (groups.get(k) || 0) + g.end - g.start);
    }
    $('detailsTitle').textContent = 'برنامه‌ها';
  } else if (S.active === 'usage') {
    for (const g of S.day.computerUsage) {
      const n = g.state === 'away' ? 'غایب' : 'فعال';
      add(n, g.start, g.end, n, [g.start, g.end]);
      groups.set(n, (groups.get(n) || 0) + g.end - g.start);
    }
    $('detailsTitle').textContent = 'استفاده از سیستم';
  } else if (S.active === 'tags') {
    for (const t of S.day.tags) {
      add(`${t.name}${t.note ? ' — ' + t.note : ''}${t.billable ? ' 💰' : ''}`,
        t.start, t.end, '', [t.start, t.end]);
      groups.set(firstLevel(t.name), (groups.get(firstLevel(t.name)) || 0) + t.end - t.start);
    }
    $('detailsTitle').textContent = 'تگ‌ها';
  } else if (S.active === 'autotags') {
    for (const g of S.day.autotagSegments) {
      add(g.name, g.start, g.end, g.app, [g.start, g.end]);
      groups.set(firstLevel(g.name), (groups.get(firstLevel(g.name)) || 0) + g.end - g.start);
    }
    $('detailsTitle').textContent = 'اتوتگ‌ها';
  } else {
    for (const s of S.day.shots) add(`عکس #${s.id}`, s.t, s.t, '', [s.t, s.t + S.day.intervalSec]);
    $('detailsTitle').textContent = 'اسکرین‌شات‌ها';
  }

  // details with filter
  const f = ($('filterInput').value || '').toLowerCase();
  const frag = document.createDocumentFragment();
  for (const r of rows) {
    if (f && !(r.title + ' ' + r.extra).toLowerCase().includes(f)) continue;
    const div = document.createElement('div');
    div.className = 'row';
    div.innerHTML = `<span class="title"></span><span class="meta"></span>`;
    div.querySelector('.title').textContent = r.title;
    div.querySelector('.meta').textContent = `${fmtClock(r.start)} – ${fmtClock(r.end)} • ${fmtDur(r.end - r.start)}`;
    div.onclick = () => { S.selStart = r.sel[0]; S.selEnd = r.sel[1]; draw(); selStatus(); };
    frag.appendChild(div);
  }
  $('detailsRows').replaceChildren(frag);

  // summary
  const total = [...groups.values()].reduce((a, b) => a + b, 0);
  const sorted = [...groups.entries()].sort((a, b) => b[1] - a[1]);
  const frag2 = document.createDocumentFragment();
  for (const [name, v] of sorted) {
    const div = document.createElement('div');
    div.className = 'row';
    div.innerHTML = `<span class="title"></span><span class="meta"></span>`;
    div.querySelector('.title').textContent = name;
    div.querySelector('.meta').textContent =
      `${fmtDur(v)} • ${total ? Math.round(v / total * 100) : 0}%`;
    frag2.appendChild(div);
  }
  $('summaryRows').replaceChildren(frag2);
}

// ---------- status ----------
function setStatus(txt) { $('status').textContent = txt; }
function dayStatus() {
  const n = S.day.shots.length;
  const total = S.day.shots.reduce((a, s) => a + s.sizeBytes, 0);
  setStatus(n ? `${S.day.day} — ${n} اسکرین‌شات • ${(total / 1048576).toFixed(1)} MB` : `${S.day.day} — اطلاعاتی ثبت نشده`);
}
function selStatus() {
  if (S.selStart < 0) { dayStatus(); return; }
  const lo = Math.min(S.selStart, S.selEnd), hi = Math.max(S.selStart, S.selEnd);
  const n = S.day.shots.filter(s => s.t >= lo && s.t <= hi).length;
  let active = 0;
  for (const g of S.day.appUsage) active += Math.max(0, Math.min(hi, g.end) - Math.max(lo, g.start));
  setStatus(`انتخاب ${fmtClock(lo)} → ${fmtClock(hi)} • ${fmtDur(hi - lo)} • ${n} عکس • فعال: ${fmtDur(active)}`);
}

// ---------- viewer ----------
function showShot(idx) {
  const shots = S.day.shots;
  if (!shots.length) return;
  S.viewIdx = (idx + shots.length) % shots.length;
  const s = shots[S.viewIdx];
  $('viewer').src = `/api/screenshot/${s.id}`;
  $('shotSave').href = `/api/screenshot/${s.id}?download=1`;
  $('shotInfo').textContent = `${s.time} • ${s.width}×${s.height} • ${(s.sizeBytes / 1024).toFixed(0)} KB`;
}
function shotAt(sec) {
  const iv = S.day.intervalSec, shots = S.day.shots;
  if (!shots.length) return -1;
  let idx = -1;
  for (let i = 0; i < shots.length; i++) { if (shots[i].t <= sec) idx = i; else break; }
  if (idx < 0) return -1;
  if (sec - shots[idx].t > iv) return -1;
  return idx;
}

// ---------- events ----------
canvas.addEventListener('mousemove', e => {
  const r = canvas.getBoundingClientRect(), x = e.clientX - r.left;
  S.hoverX = x; S.hover = xToSec(x); draw();
  const idx = shotAt(S.hover);
  if (idx >= 0) { showShot(idx); setStatus(`ساعت ${fmtClock(S.hover)} — عکس ${S.day.shots[idx].time}`); }
});
canvas.addEventListener('mouseleave', () => { S.hover = -1; draw(); });
canvas.addEventListener('wheel', e => {
  e.preventDefault();
  const r = canvas.getBoundingClientRect(), x = e.clientX - r.left;
  if (Math.abs(e.deltaX) > Math.abs(e.deltaY)) setViewStart(S.viewStart + e.deltaX / pxPerSec());
  else zoomAt(x, e.deltaY > 0 ? 1 : -1);
}, { passive: false });
canvas.addEventListener('mousedown', e => {
  const r = canvas.getBoundingClientRect(), x = e.clientX - r.left, y = e.clientY - r.top;
  if (e.button === 1) { S.panning = true; S.panStartView = S.viewStart; e.preventDefault(); return; }
  if (e.button !== 0) return;
  S.selPressX = x;
  const start = snap(xToSec(x));
  S.selStart = S.selEnd = start; draw(); selStatus();
});
window.addEventListener('mousemove', e => {
  const r = canvas.getBoundingClientRect(), x = e.clientX - r.left;
  if (S.panning) { setViewStart(S.panStartView - (x - S._panX0) / pxPerSec()); return; }
  if (S.selStart < 0 || S.selPressX == null || e.buttons !== 1) return;
  if (e.target !== canvas) return;
  S.selEnd = snap(xToSec(S.selPressX + (x - S.selPressX)));
  // during drag the offset is x - pressX, but press was at S.selPressX:
  S.selEnd = snap(xToSec(x));
  draw(); selStatus();
});
window.addEventListener('mouseup', () => {
  if (S.panning) { S.panning = false; return; }
  if (S.selStart >= 0 && Math.abs(S.selEnd - S.selStart) * pxPerSec() < 4) {
    S.selStart = S.selEnd = -1; draw(); selStatus();
  }
});
canvas.addEventListener('dblclick', e => {
  const r = canvas.getBoundingClientRect(), x = e.clientX - r.left, y = e.clientY - r.top;
  const tr = trackAtY(y); if (!tr) return;
  const sec = xToSec(x);
  if (tr.key === 'shots') {
    const iv = S.day.intervalSec, has = new Set(S.day.shots.map(s => Math.floor(s.t / iv)));
    let b = Math.floor(sec / iv); if (!has.has(b)) return;
    let lo = b, hi = b;
    while (has.has(lo - 1)) lo--; while (has.has(hi + 1)) hi++;
    S.selStart = lo * iv; S.selEnd = (hi + 1) * iv;
  } else {
    const g = trackData(tr.key).find(g => sec >= g.start && sec < g.end);
    if (!g) return;
    S.selStart = g.start; S.selEnd = g.end;
  }
  draw(); selStatus();
});
canvas.addEventListener('contextmenu', e => {
  e.preventDefault();
  const r = canvas.getBoundingClientRect(), x = e.clientX - r.left, y = e.clientY - r.top;
  const tr = trackAtY(y); if (!tr) return;
  const sec = xToSec(x);
  if (tr.key === 'apps') {
    const g = trackData('apps').find(g => sec >= g.start && sec < g.end);
    openAutoDlg(g ? g.app : '', g ? g.title : '');
  } else if (tr.key === 'tags') {
    const g = trackData('tags').find(g => sec >= g.start && sec < g.end);
    if (g && confirm(`حذف تگ «${g.name}»؟`)) {
      fetch(`/api/tags/${g.id}`, { method: 'DELETE' }).then(() => loadDay(S.day.day));
    }
  }
});
canvas.addEventListener('click', e => {
  const r = canvas.getBoundingClientRect(), y = e.clientY - r.top;
  const tr = trackAtY(y); if (!tr) return;
  S.active = tr.key; renderLabels(); renderPanels();
});
document.querySelectorAll('.tlabel').forEach(el =>
  el.onclick = () => { S.active = el.dataset.track; renderLabels(); renderPanels(); });

$('prevDay').onclick = () => shiftDay(-1);
$('nextDay').onclick = () => shiftDay(1);
$('todayBtn').onclick = () => loadDay(todayStr());
$('zoomIn').onclick = () => zoomAt(width() / 2, -1);
$('zoomOut').onclick = () => zoomAt(width() / 2, 1);
$('shotPrev').onclick = () => showShot(S.viewIdx - 1);
$('shotNext').onclick = () => showShot(S.viewIdx + 1);
$('shotDelete').onclick = async () => {
  const s = S.day.shots[S.viewIdx];
  if (!s || !confirm('این اسکرین‌شات برای همیشه حذف شود؟')) return;
  await fetch(`/api/screenshot/${s.id}`, { method: 'DELETE' });
  loadDay(S.day.day);
};
$('filterInput').oninput = () => renderPanels();

document.addEventListener('keydown', e => {
  if (e.target.tagName === 'INPUT' || e.target.tagName === 'SELECT' || document.querySelector('dialog[open]')) return;
  if (e.key === 'ArrowLeft') shiftDay(1);
  else if (e.key === 'ArrowRight') shiftDay(-1);
  else if (e.key === 'Home') loadDay(todayStr());
  else if (e.key === 'Escape') { S.selStart = S.selEnd = -1; draw(); selStatus(); }
  else if (e.key === '+') zoomAt(width() / 2, -1);
  else if (e.key === '-') zoomAt(width() / 2, 1);
});

// ---------- dialogs ----------
const tagDlg = $('tagDlg'), autoDlg = $('autoDlg'), rulesDlg = $('rulesDlg'), tsDlg = $('tsDlg'), setDlg = $('setDlg');

$('addTagBtn').onclick = () => {
  const f = $('tagForm');
  const lo = Math.min(S.selStart, S.selEnd), hi = Math.max(S.selStart, S.selEnd);
  f.start.value = fmtClock(S.selStart >= 0 ? lo : Math.max(0, S.hover - 1800));
  f.end.value = fmtClock(S.selStart >= 0 ? hi : Math.max(0, S.hover));
  $('tagErr').textContent = '';
  const seen = new Set(), rc = $('tagRecent');
  rc.replaceChildren();
  for (const t of [...S.day.tags].reverse()) {
    if (seen.has(t.name) || seen.size >= 6) continue;
    seen.add(t.name);
    const b = document.createElement('button');
    b.type = 'button'; b.textContent = firstLevel(t.name);
    b.onclick = () => f.name.value = t.name;
    rc.appendChild(b);
  }
  tagDlg.showModal();
};
$('tagForm').onsubmit = async e => {
  e.preventDefault();
  const f = e.target;
  const start = parseClock(f.start.value), end = parseClock(f.end.value);
  if (end <= start) { $('tagErr').textContent = 'پایان باید بعد از شروع باشد'; return; }
  const res = await fetch('/api/tags', {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ day: S.day.day, startSec: start, endSec: end,
      name: f.name.value.trim(), note: f.note.value.trim(), billable: f.billable.checked }),
  });
  if (!res.ok) { $('tagErr').textContent = (await res.json()).error; return; }
  tagDlg.close(); loadDay(S.day.day);
};

function openAutoDlg(app, title) {
  const f = $('autoForm');
  f.app.value = app || ''; f.title.value = title || ''; f.name.value = '';
  $('autoErr').textContent = '';
  autoDlg.showModal();
}
$('autoForm').onsubmit = async e => {
  e.preventDefault();
  const f = e.target;
  const res = await fetch('/api/autotags', {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name: f.name.value.trim(), appPattern: f.app.value.trim(), titlePattern: f.title.value.trim() }),
  });
  if (!res.ok) { $('autoErr').textContent = (await res.json()).error; return; }
  autoDlg.close(); loadDay(S.day.day);
};

$('rulesBtn').onclick = async () => { await renderRules(); rulesDlg.showModal(); };
async function renderRules() {
  const rules = await (await fetch('/api/autotags')).json();
  const list = $('rulesList'); list.replaceChildren();
  if (!rules.length) {
    const d = document.createElement('div');
    d.style.color = 'var(--muted)';
    d.textContent = 'هنوز قانونی نیست — روی برنامه‌ای در تایم‌لاین راست‌کلیک کنید.';
    list.appendChild(d); return;
  }
  for (const r of rules) {
    const div = document.createElement('div');
    div.className = 'rule';
    div.innerHTML = `<div><b></b><br><code></code></div>`;
    div.querySelector('b').textContent = r.name;
    div.querySelector('code').textContent =
      `app ~ "${r.appPattern || '*'}" | title ~ "${r.titlePattern || '*'}"`;
    const del = document.createElement('button');
    del.textContent = '🗑';
    del.onclick = async () => { await fetch(`/api/autotags/${r.id}`, { method: 'DELETE' }); renderRules(); loadDay(S.day.day); };
    div.appendChild(del);
    list.appendChild(div);
  }
}

// timesheet
$('timesheetBtn').onclick = async () => {
  const to = new Date(), from = new Date(); from.setDate(from.getDate() - 6);
  $('tsFrom').value = from.toISOString().slice(0, 10);
  $('tsTo').value = to.toISOString().slice(0, 10);
  tsDlg.showModal();
  genTimesheet();
};
async function genTimesheet() {
  const res = await fetch(`/api/timesheet?from=${$('tsFrom').value}&to=${$('tsTo').value}&level=${$('tsLevel').value}`);
  S.tsData = await res.json();
  const frag = document.createDocumentFragment();
  for (const r of S.tsData) {
    const div = document.createElement('div');
    div.className = 'row';
    div.innerHTML = `<span class="title"></span><span class="meta"></span>`;
    div.querySelector('.title').textContent = r.group;
    div.querySelector('.meta').textContent = `${fmtDur(r.totalSeconds)} • ${r.days} ردیف`;
    frag.appendChild(div);
  }
  $('tsRows').replaceChildren(frag);
}
$('tsGen').onclick = genTimesheet;
$('tsLevel').onchange = genTimesheet;
$('tsCsv').onclick = () => {
  const csv = 'group,total,days\n' + S.tsData.map(r =>
    `"${r.group}","${fmtDur(r.totalSeconds)}",${r.days}`).join('\n');
  const a = document.createElement('a');
  a.href = URL.createObjectURL(new Blob(['\ufeff' + csv], { type: 'text/csv' }));
  a.download = 'payam-timesheet.csv'; a.click();
};
$('tsCopy').onclick = () => navigator.clipboard.writeText(
  S.tsData.map(r => `${r.group}\t${fmtDur(r.totalSeconds)}`).join('\n'));

// settings
$('settingsBtn').onclick = async () => {
  const st = await (await fetch('/api/status')).json();
  $('setInterval').value = String(st.intervalSec);
  $('setForever').checked = st.retentionDays === 0;
  $('setRetention').value = st.retentionDays || 10;
  $('setRetention').disabled = st.retentionDays === 0;
  $('setErr').textContent = '';
  setDlg.showModal();
};
$('setForever').onchange = e => $('setRetention').disabled = e.target.checked;
$('setSave').onclick = async () => {
  const body = {
    intervalSec: Number($('setInterval').value),
    retentionDays: $('setForever').checked ? 0 : Number($('setRetention').value),
  };
  const res = await fetch('/api/settings', {
    method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body),
  });
  if (!res.ok) { $('setErr').textContent = (await res.json()).error; return; }
  setDlg.close();
  setStatus('تنظیمات ذخیره شد — سرویس در حال ری‌استارت… (چند ثانیه صبر کنید و رفرش کنید)');
  setTimeout(() => location.reload(), 2500);
};

// ---------- boot ----------
window.addEventListener('resize', resizeCanvas);
resizeCanvas();
loadDay(todayStr());
