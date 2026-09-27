/* The editor: localStorage holds the draft, the schema drives the form, HTMX
   refreshes the preview and the report. The server never stores anything. */
'use strict';

const STORE_KEY = 'atscv.sessions.v1';
const SCHEMA_URL = '/api/schema';
const DEMO_URL = '/api/demo';

/** @type {any} */
let schema = null;
/** @type {{sessions: Array, current: string}} */
let store = { sessions: [], current: '' };
let saveTimer = 0;
let syncTimer = 0;

/* ------------------------------------------------------------------ storage */

/** Reads the sessions, repairing anything a previous version or a hand edit left
 *  behind rather than throwing the whole draft away. */
function loadStore() {
  try {
    const raw = localStorage.getItem(STORE_KEY);
    if (!raw) return { sessions: [], current: '' };
    const parsed = JSON.parse(raw);
    if (!parsed || !Array.isArray(parsed.sessions)) return { sessions: [], current: '' };
    parsed.sessions = parsed.sessions.filter((s) => s && typeof s.id === 'string' && s.data);
    if (!parsed.sessions.length) return { sessions: [], current: '' };
    if (!parsed.sessions.some((s) => s.id === parsed.current)) {
      parsed.current = parsed.sessions[0].id;
    }
    return parsed;
  } catch (err) {
    console.warn('atscv: unreadable session store, starting fresh', err);
    return { sessions: [], current: '' };
  }
}

function persist() {
  try {
    localStorage.setItem(STORE_KEY, JSON.stringify(store));
    return true;
  } catch (err) {
    // A full quota must not break the editor: the draft stays in memory and the
    // user is told, instead of losing it silently.
    toast('Sauvegarde impossible : espace local plein. Exporte ton JSON.', 'error');
    console.error(err);
    return false;
  }
}

const current = () => store.sessions.find((s) => s.id === store.current) || null;

/** An empty resume, shaped like the model so Parse accepts it. */
function emptyResume() {
  return {
    lang: schema && schema.lang === 'en' ? 'en' : 'fr',
    name: '',
    headline: '',
    contact: { email: '', phone: '', city: '', region: '', country: '', note: '', links: [] },
    summary: '',
    experience: [],
    education: [],
    skills: [],
    certifications: [],
    projects: [],
    activities: [],
    languages: [],
  };
}

function newSession(data, name) {
  const id = 's' + Date.now().toString(36) + Math.random().toString(36).slice(2, 6);
  const session = { id, name: name || 'Sans titre', updatedAt: Date.now(), data: data || emptyResume() };
  store.sessions.push(session);
  store.current = id;
  return session;
}

/* -------------------------------------------------------------------- i18n */

const isEN = () => Boolean(schema && schema.lang === 'en');

/** Picks the label of the editor language, falling back to French. */
const T = {
  label: (fr, en) => (isEN() ? en : fr),
  add: () => T.label('Ajouter', 'Add'),
  remove: () => T.label('Supprimer', 'Delete'),
  duplicate: () => T.label('Dupliquer', 'Duplicate'),
  moveUp: () => T.label('Monter', 'Move up'),
  moveDown: () => T.label('Descendre', 'Move down'),
  empty: () => T.label('Vide', 'Empty'),
  saved: () => T.label('Enregistré', 'Saved'),
  saving: () => T.label('Enregistrement…', 'Saving…'),
  noResume: () => T.label('Le CV est vide : écris ton nom.', 'The resume is empty: type your name.'),
  downloaded: (n) => T.label('Téléchargé : ', 'Downloaded: ') + n,
  failed: () => T.label('Échec du téléchargement. Voir la console.', 'Download failed. See the console.'),
  confirmDelete: (n) => T.label('Supprimer la session « ', 'Delete session "') + n + T.label(' » ?', '"?'),
  unnamed: () => T.label('Sans titre', 'Untitled'),
};

/* --------------------------------------------------------------------- DOM */

function el(tag, attrs, children) {
  const node = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs || {})) {
    if (v === null || v === undefined || v === false) continue;
    if (k === 'class') node.className = v;
    else if (k === 'text') node.textContent = v;
    else if (k.startsWith('on')) node.addEventListener(k.slice(2), v);
    else node.setAttribute(k, v === true ? '' : v);
  }
  for (const child of [].concat(children || [])) {
    if (child === null || child === undefined) continue;
    node.appendChild(typeof child === 'string' ? document.createTextNode(child) : child);
  }
  return node;
}

/* ------------------------------------------------------------ form building */

/** The label of a field, in the editor language. */
function fieldLabel(field) {
  return isEN() ? field.label_en || field.label_fr : field.label_fr;
}

function sectionLabel(section) {
  return isEN() ? section.label_en || section.label_fr : section.label_fr;
}

function entryLabel(section) {
  return isEN() ? section.entry_en || section.entry_fr : section.entry_fr;
}

/** A card holding the fields of one entry, or of a whole singleton section. */
function buildEntry(section, entry, index, onChange, onRemove, onMove) {
  const nameOf = () => entrySummary(entry, section);
  const header = el('header', {}, [
    el('span', { class: 'entry-name', text: nameOf() }),
  ]);
  if (onMove) {
    header.appendChild(el('button', {
      type: 'button', title: T.moveUp(), 'aria-label': T.moveUp(),
      onclick: () => onMove(index, -1),
    }, ['↑']));
    header.appendChild(el('button', {
      type: 'button', title: T.moveDown(), 'aria-label': T.moveDown(),
      onclick: () => onMove(index, 1),
    }, ['↓']));
  }
  // Only a repeatable entry can be removed or duplicated. A singleton is a
  // struct: there is one of it, so a delete button would either do nothing or
  // throw, and a copy would need a place to go.
  if (onRemove) {
    header.appendChild(el('button', {
      type: 'button', class: 'danger', title: T.remove(), 'aria-label': T.remove(),
      onclick: onRemove,
    }, ['✕']));
    header.appendChild(el('button', {
      type: 'button', title: T.duplicate(), 'aria-label': T.duplicate(),
      onclick: () => {
        const list = section.array;
        if (!Array.isArray(list)) return;
        list.splice(index + 1, 0, JSON.parse(JSON.stringify(entry)));
        onChange();
        rerender();
      },
    }, ['⧉']));
  }

  const body = el('div', { class: 'body' });
  for (const field of section.fields) {
    body.appendChild(buildField(field, entry, index, onChange, section));
  }
  return el('article', { class: 'entry' }, [header, body]);
}

/** The text shown on an entry header: the first meaningful string it holds. */
function entrySummary(entry, section) {
  if (!entry || typeof entry !== 'object') return T.unnamed();
  for (const field of section.fields) {
    if (field.kind === 'list' || field.kind === 'entries' || field.kind === 'textarea') continue;
    const v = entry[field.key];
    if (typeof v === 'string' && v.trim()) return v.trim();
  }
  return T.unnamed();
}

function buildField(field, target, index, onChange, section) {
  const value = target[field.key];
  let input;

  if (field.kind === 'textarea') {
    input = el('textarea', { rows: '3' });
    input.value = value || '';
    input.addEventListener('input', () => { target[field.key] = input.value; onChange(); });
  } else if (field.kind === 'list') {
    return buildListField(field, target, onChange);
  } else if (field.kind === 'entries') {
    return buildEntriesField(field, target, onChange);
  } else {
    input = el('input', { type: 'text', class: field.mono ? 'mono' : null });
    input.value = value || '';
    input.addEventListener('input', () => { target[field.key] = input.value; onChange(); });
  }

  return el('div', { class: 'field' }, [
    el('label', { text: fieldLabel(field) }),
    input,
  ]);
}

/** A repeatable list of short strings: one textarea per item, so a long
 *  achievement can be written without a horizontal scrollbar. */
function buildListField(field, target, onChange) {
  const list = Array.isArray(target[field.key]) ? target[field.key] : [];
  const wrap = el('div', { class: 'list' });

  const commit = () => {
    target[field.key] = list.slice();
    onChange();
  };

  list.forEach((item, i) => {
    const area = el('textarea', { rows: '2' });
    area.value = item;
    area.addEventListener('input', () => { list[i] = area.value; commit(); });
    // The items are read in the order they are written, so the order is part of
    // what the author is saying. A move is the same splice the sections use.
    // The ends are disabled rather than silent: a button that does nothing
    // looks broken, and this one is a button the reader can see.
    const move = (delta) => {
      const to = i + delta;
      if (to < 0 || to >= list.length) return;
      const [moved] = list.splice(i, 1);
      list.splice(to, 0, moved);
      commit();
      rerender();
    };
    wrap.appendChild(el('div', { class: 'list-row' }, [
      area,
      el('button', {
        type: 'button', title: T.moveUp(), 'aria-label': T.moveUp() + ' ' + (i + 1),
        disabled: i === 0, onclick: () => move(-1),
      }, ['↑']),
      el('button', {
        type: 'button', title: T.moveDown(), 'aria-label': T.moveDown() + ' ' + (i + 1),
        disabled: i === list.length - 1, onclick: () => move(1),
      }, ['↓']),
      el('button', {
        type: 'button', class: 'danger', title: T.remove(), 'aria-label': T.remove() + ' ' + (i + 1),
        onclick: () => { list.splice(i, 1); commit(); rerender(); },
      }, ['✕']),
    ]));
  });

  wrap.appendChild(el('button', {
    type: 'button', class: 'add',
    onclick: () => { list.push(''); commit(); rerender(); },
  }, ['+ ' + fieldLabel(field)]));
  return el('div', { class: 'field' }, [el('label', { text: fieldLabel(field) }), wrap]);
}

/** A repeatable list of objects with scalar fields, used by the contact links. */
function buildEntriesField(field, target, onChange) {
  const list = Array.isArray(target[field.key]) ? target[field.key] : [];
  const wrap = el('div', { class: 'list' });
  const linkFields = [
    { key: 'label', kind: 'text', label_fr: 'Libellé', label_en: 'Label' },
    { key: 'url', kind: 'text', label_fr: 'URL', label_en: 'URL' },
  ];

  const commit = () => { target[field.key] = list.slice(); onChange(); };

  list.forEach((item, i) => {
    const row = el('div', { class: 'list-row' });
    for (const lf of linkFields) {
      const input = el('input', { type: 'text', placeholder: fieldLabel(lf) });
      input.value = item[lf.key] || '';
      input.addEventListener('input', () => { item[lf.key] = input.value; commit(); });
      row.appendChild(input);
    }
    row.appendChild(el('button', {
      type: 'button', class: 'danger', title: T.remove(), 'aria-label': T.remove(),
      onclick: () => { list.splice(i, 1); commit(); rerender(); },
    }, ['✕']));
    wrap.appendChild(row);
  });

  wrap.appendChild(el('button', {
    type: 'button', class: 'add',
    onclick: () => { list.push({ label: '', url: '' }); commit(); rerender(); },
  }, ['+ ' + fieldLabel(field)]));
  return el('div', { class: 'field' }, [el('label', { text: fieldLabel(field) }), wrap]);
}

/** Builds one whole section card. */
function buildSection(section, data, onChange) {
  const card = el('section', { class: 'card' });
  card.appendChild(el('header', {}, [el('span', { text: sectionLabel(section) })]));

  const body = el('div', { class: 'body' });

  if (section.singleton) {
    data[section.key] = data[section.key] || {};
    body.appendChild(buildEntry(section, data[section.key], 0, onChange, null, null));
  } else {
    const list = Array.isArray(data[section.key]) ? data[section.key] : [];
    section.array = list;
    if (!list.length) {
      body.appendChild(el('p', { class: 'empty', text: T.empty() }));
    }
    list.forEach((entry, i) => {
      body.appendChild(buildEntry(section, entry, i, onChange,
        () => { list.splice(i, 1); onChange(); rerender(); },
        (index, delta) => {
          const to = index + delta;
          if (to < 0 || to >= list.length) return;
          const [moved] = list.splice(index, 1);
          list.splice(to, 0, moved);
          onChange();
          rerender();
        }));
    });
    body.appendChild(el('button', {
      type: 'button', class: 'add',
      onclick: () => {
        list.push(blankEntry(section));
        onChange();
        rerender();
      },
    }, ['+ ' + entryLabel(section)]));
  }

  card.appendChild(body);
  return card;
}

function blankEntry(section) {
  const entry = {};
  for (const field of section.fields) {
    if (field.kind === 'list' || field.kind === 'entries') entry[field.key] = [];
    else entry[field.key] = '';
  }
  return entry;
}

/* ------------------------------------------------------------------ render */

let rerenderQueued = false;

/** Redraws the form. Rerendering on every keystroke would move the caret, so
 *  only structural changes (add, remove, reorder, switch session) go through
 *  it; typing just updates the model and the hidden field. */
function rerender() {
  if (rerenderQueued) return;
  rerenderQueued = true;
  requestAnimationFrame(() => {
    rerenderQueued = false;
    const data = current() ? current().data : emptyResume();
    const target = document.getElementById('schema-target');
    const scroll = target.scrollTop;
    const onChange = () => { sync(); };

    target.replaceChildren(
      buildSingleton(schema.identity, data, onChange),
      buildSingleton(schema.contact, data, onChange),
      ...schema.sections.map((section) => buildSection(section, data, onChange)),
    );
    target.scrollTop = scroll;
    renderSessionList();
  });
}

/** The identity and the contact block are structs: the same card, without the
 *  add and remove buttons. The identity writes to the root of the resume, the
 *  contact to its own key. */
function buildSingleton(section, data, onChange) {
  const card = el('section', { class: 'card' });
  card.appendChild(el('header', {}, [el('span', { text: sectionLabel(section) })]));
  const body = el('div', { class: 'body' });
  let entry;
  if (section.root) {
    entry = data;
  } else {
    if (!data[section.key] || typeof data[section.key] !== 'object') data[section.key] = {};
    entry = data[section.key];
  }
  const grid = el('div', { class: 'grid2' });

  for (const field of section.fields) {
    const node = buildField(field, entry, 0, onChange, section);
    if (field.kind === 'textarea' || field.kind === 'list' || field.kind === 'entries') {
      body.appendChild(node);
    } else {
      grid.appendChild(node);
    }
  }
  body.appendChild(grid);
  card.appendChild(body);
  return card;
}

/* ------------------------------------------------------------------- sync */

/** Pushes the model into the hidden fields and asks HTMX to refresh. */
function sync() {
  const session = current();
  const data = session ? session.data : emptyResume();
  document.getElementById('data').value = JSON.stringify(data);
  document.getElementById('name-field').value = (data.name || '').trim() || T.unnamed();
  document.getElementById('ascii-field').value = document.getElementById('ascii').checked ? 'on' : '';
  scheduleSave();
  scheduleRefresh();
}

function scheduleRefresh() {
  clearTimeout(syncTimer);
  syncTimer = setTimeout(() => {
    document.body.dispatchEvent(new CustomEvent('cv:change'));
  }, 250);
}

function scheduleSave() {
  clearTimeout(saveTimer);
  setSaveState(T.saving());
  saveTimer = setTimeout(saveNow, 400);
}

function saveNow() {
  const session = current();
  if (!session) return;
  session.updatedAt = Date.now();
  if (persist()) setSaveState(T.saved());
}

function setSaveState(text) {
  const node = document.getElementById('save-state');
  if (node) node.textContent = text;
}

function toast(message, kind) {
  const node = document.getElementById('toast');
  if (!node) return;
  node.textContent = message;
  node.dataset.kind = kind || 'info';
  node.hidden = false;
  clearTimeout(node.dataset.timer);
  node.dataset.timer = String(setTimeout(() => { node.hidden = true; }, kind === 'error' ? 6000 : 2400));
}

/* ---------------------------------------------------------------- sessions */

function renderSessionList() {
  const select = document.getElementById('session');
  select.replaceChildren(...store.sessions.map((s) => {
    const when = new Date(s.updatedAt);
    const stamp = Number.isNaN(when.getTime()) ? '' : ' · ' + when.toLocaleDateString();
    return el('option', { value: s.id, selected: s.id === store.current ? true : null },
      [s.name + stamp]);
  }));
}

function selectSession(id) {
  const session = store.sessions.find((s) => s.id === id);
  if (!session) return;
  store.current = id;
  saveNow();
  rerender();
  sync();
  document.body.dispatchEvent(new CustomEvent('cv:change'));
}

function wireSessions() {
  document.getElementById('session').addEventListener('change', (e) => selectSession(e.target.value));

  document.getElementById('session-new').addEventListener('click', () => {
    newSession(null, T.label('Nouveau CV', 'New resume'));
    rerender();
    sync();
    document.body.dispatchEvent(new CustomEvent('cv:change'));
  });

  document.getElementById('session-rename').addEventListener('click', () => {
    const session = current();
    if (!session) return;
    const name = window.prompt(T.label('Nom de la session', 'Session name'), session.name);
    if (name === null) return;
    session.name = name.trim() || T.unnamed();
    persist();
    renderSessionList();
  });

  document.getElementById('session-duplicate').addEventListener('click', () => {
    const session = current();
    if (!session) return;
    newSession(JSON.parse(JSON.stringify(session.data)), session.name + T.label(' (copie)', ' (copy)'));
    rerender();
    sync();
    document.body.dispatchEvent(new CustomEvent('cv:change'));
  });

  document.getElementById('session-delete').addEventListener('click', () => {
    const session = current();
    if (!session) return;
    if (!window.confirm(T.confirmDelete(session.name))) return;
    store.sessions = store.sessions.filter((s) => s.id !== session.id);
    if (!store.sessions.length) newSession(null, T.label('Nouveau CV', 'New resume'));
    else store.current = store.sessions[0].id;
    persist();
    rerender();
    sync();
    document.body.dispatchEvent(new Event('cv:change'));
  });
}

/* -------------------------------------------------------------- import/export */

function wireFiles() {
  document.getElementById('export-btn').addEventListener('click', () => {
    const session = current();
    if (!session) return;
    const blob = new Blob([JSON.stringify(session.data, null, 2) + '\n'], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const a = el('a', { href: url, download: (session.data.name || 'resume').replace(/[^\w.-]+/g, '_') + '.json' });
    document.body.appendChild(a);
    a.click();
    a.remove();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  });

  const picker = document.getElementById('import-file');
  document.getElementById('import-btn').addEventListener('click', () => picker.click());
  picker.addEventListener('change', async () => {
    const file = picker.files && picker.files[0];
    if (!file) return;
    try {
      const text = await file.text();
      const parsed = JSON.parse(text);
      // The server is the only authority on the schema: let it validate before
      // the draft enters the editor.
      const res = await fetch('/api/parse?format=json', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: text,
      });
      if (!res.ok) {
        const body = await res.json().catch(() => ({}));
        throw new Error(body.error || T.label('Fichier invalide', 'Invalid file'));
      }
      // The answer is the resume the server parsed: unknown fields are gone and
      // the missing ones filled, so the draft can go straight back to the API.
      const normalized = await res.json();
      newSession(normalized && normalized.name !== undefined ? normalized : parsed,
        file.name.replace(/\.json$/i, ''));
      persist();
      rerender();
      sync();
      document.body.dispatchEvent(new Event('cv:change'));
      toast(T.label('Importé', 'Imported'));
    } catch (err) {
      toast(err.message, 'error');
    } finally {
      picker.value = '';
    }
  });
}

/* --------------------------------------------------------------- downloads */

function wireDownloads() {
  for (const button of document.querySelectorAll('[data-format]')) {
    button.addEventListener('click', async () => {
      const format = button.dataset.format;
      const body = new URLSearchParams();
      body.set('data', document.getElementById('data').value);
      body.set('name', document.getElementById('name-field').value);
      body.set('ascii', document.getElementById('ascii-field').value);
      button.disabled = true;
      try {
        const res = await fetch('/api/render/' + format, {
          method: 'POST',
          headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
          body,
        });
        if (!res.ok) {
          const payload = await res.json().catch(() => ({}));
          throw new Error(payload.error || res.statusText);
        }
        const blob = await res.blob();
        const disposition = res.headers.get('Content-Disposition') || '';
        const match = /filename="([^"]+)"/.exec(disposition);
        const url = URL.createObjectURL(blob);
        const a = el('a', { href: url, download: match ? match[1] : 'resume.' + format });
        document.body.appendChild(a);
        a.click();
        a.remove();
        setTimeout(() => URL.revokeObjectURL(url), 1000);
        toast(T.downloaded(match ? match[1] : format));
      } catch (err) {
        console.error(err);
        toast(err.message + ' — ' + T.failed(), 'error');
      } finally {
        button.disabled = false;
      }
    });
  }
}

/* -------------------------------------------------------------------- tabs */

function wireTabs() {
  for (const tab of document.querySelectorAll('.tab')) {
    tab.addEventListener('click', () => {
      for (const other of document.querySelectorAll('.tab')) {
        const active = other === tab;
        other.classList.toggle('is-active', active);
        other.setAttribute('aria-selected', String(active));
      }
      for (const panel of document.querySelectorAll('.tabpanel')) {
        panel.classList.toggle('is-active', panel.id === 'panel-' + tab.dataset.tab);
      }
    });
  }
}

/* -------------------------------------------------------------------- boot */

// loadDemo asks the server for the resume a fresh editor starts from. A 404 is
// the normal answer of a local instance, and is not an error: the editor simply
// starts empty. Anything else that goes wrong is swallowed too, because a demo
// is a convenience and must never keep the editor from opening.
async function loadDemo() {
  try {
    const res = await fetch(DEMO_URL, { headers: { Accept: 'application/json' } });
    if (!res.ok) return null;
    const data = await res.json();
    return data && typeof data === 'object' ? data : null;
  } catch (err) {
    return null;
  }
}

async function boot() {
  wireTabs();
  wireSessions();
  wireFiles();
  wireDownloads();

  document.getElementById('ascii').addEventListener('change', () => {
    sync();
    document.body.dispatchEvent(new Event('cv:change'));
  });

  let res;
  try {
    res = await fetch(SCHEMA_URL, { headers: { Accept: 'application/json' } });
  } catch (err) {
    document.getElementById('schema-target').replaceChildren(
      el('p', { class: 'finding finding-error', text: T.label('Serveur injoignable.', 'Server unreachable.') }));
    return;
  }
  if (!res.ok) {
    document.getElementById('schema-target').replaceChildren(
      el('p', { class: 'finding finding-error', text: T.label('Schéma indisponible.', 'Schema unavailable.') }));
    return;
  }
  schema = await res.json();

  // loadStore already repairs a dangling current id, so a reload lands on the
  // same draft. An empty store starts a blank one, unless the server offers a
  // demo: a visitor who opens a deployed instance should land on something to
  // look at rather than on an empty form. The demo is copied into the store and
  // becomes an ordinary draft, owned by the browser like any other.
  store = loadStore();
  if (!store.sessions.length) {
    const demo = await loadDemo();
    newSession(demo, T.label(demo ? 'Mon CV (démo)' : 'Mon CV', demo ? 'My resume (demo)' : 'My resume'));
  } else if (!store.sessions.some((s) => s.id === store.current)) {
    store.current = store.sessions[0].id;
  }
  persist();

  rerender();
  // The form is drawn on the next frame, so sync once it exists.
  requestAnimationFrame(() => {
    sync();
    document.body.dispatchEvent(new Event('cv:change'));
  });
}

document.addEventListener('DOMContentLoaded', boot);
