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
let refreshTimer = 0;
let lastRefresh = 0;

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
  } else if (field.kind === 'date') {
    return buildDateField(field, target, onChange);
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

/* ------------------------------------------------------------------ dates */

// The dates are read by a parser, not by a human: "2023", "01/2023",
// "2023-03", "mars 2023" and "present" are all values the model takes, and
// the day was never one of them. So a date is a single field — the native
// calendar the browser already draws — which writes the month back as
// "2023-03", the shape the parser, the preview and the exports keep.
//
// A date the model holds is always shown: pickerValue reads the date the way
// layout.ParsePeriod does, so a bare year points at January, a running period
// at today and a month name at its own month — never an empty calendar for a
// date the document has. What the calendar points at is the calendar's own:
// it is written back only when the candidate picks, so a 2023 start stays
// "2023" in the model and still prints as "2023" in the document. Only a
// value the parser itself would refuse leaves the field empty.
//
// A browser with no month widget is given the day picker, and one with no
// calendar at all the plain text field.

// DATE_PICKER is the input type that can hold a month, then one that can hold
// a day, or "" for no calendar at all. The property tells, not the attribute:
// a type the browser does not implement reads back as "text".
const DATE_PICKER = (() => {
  const probe = document.createElement('input');
  probe.type = 'month';
  if (probe.type === 'month') return 'month';
  probe.type = 'date';
  if (probe.type === 'date') return 'date';
  return '';
})();

// PRESENT_WORDS and MONTH_NAMES mirror layout.go: the widget must point at
// every date the parser reads, or the editor shows an empty calendar for a
// date the document holds.
const PRESENT_WORDS = new Set([
  'present', 'now', 'current', 'ongoing', "aujourd'hui", 'aujourd hui',
  'présent', 'en cours', 'actuel', 'actuelle',
]);
const MONTH_NAMES = {
  jan: 1, january: 1, janv: 1, janvier: 1,
  feb: 2, february: 2, fevr: 2, fev: 2, 'février': 2,
  mar: 3, march: 3, mars: 3,
  apr: 4, april: 4, avr: 4, avril: 4,
  may: 5, mai: 5,
  jun: 6, june: 6, juin: 6,
  jul: 7, july: 7, juil: 7, juillet: 7,
  aug: 8, august: 8, aout: 8, 'août': 8,
  sep: 9, sept: 9, september: 9, septembre: 9,
  oct: 10, october: 10, octobre: 10,
  nov: 11, november: 11, novembre: 11,
  dec: 12, december: 12, decembre: 12, 'décembre': 12,
};

/** monthNumber is layout.monthNumber: a token that names a month, 0 if it
 *  does not. */
function monthNumber(token) {
  const t = String(token || '').toLowerCase().replace(/\.$/, '');
  if (!t) return 0;
  if (/^\d+$/.test(t)) {
    const n = Number(t);
    return n >= 1 && n <= 12 ? n : 0;
  }
  return MONTH_NAMES[t] || 0;
}

/** numericDate is layout.parseNumericDate: "2023", "2023-03", "03/2023",
 *  "03 2023" — the month being 0 when the value names only a year. Null is a
 *  value no numeric date can be read from. */
function numericDate(value) {
  const nums = [];
  for (const raw of value.replace(/[-. ]/g, '/').split('/')) {
    const part = raw.trim();
    if (!part) continue;
    if (!/^\d+$/.test(part)) return null;
    nums.push(Number(part));
  }
  if (nums.length === 1) return nums[0] >= 1900 ? { year: nums[0], month: 0 } : null;
  if (nums.length === 2) {
    const [a, b] = nums;
    if (a >= 1000 && b >= 1 && b <= 12) return { year: a, month: b };
    if (b >= 1000 && a >= 1 && a <= 12) return { year: b, month: a };
  }
  return null;
}

/** monthOf is layout.parseDate: the year and the month a value names, the
 *  month being 0 for a bare year, or null when the parser would refuse it. */
function monthOf(text) {
  const value = String(text || '').trim();
  if (!value) return null;
  const numeric = numericDate(value);
  if (numeric) return numeric;
  // "Jan 2023", "mars 2022", "01 Jan 2023": any token order, at least two.
  const fields = value.split(/\s+/);
  if (fields.length < 2) return null;
  let year = 0;
  let month = 0;
  for (const f of fields) {
    const m = monthNumber(f);
    if (m) month = m;
    else if (/^\d+$/.test(f) && Number(f) > 1900) year = Number(f);
  }
  if (!year) return null;
  return { year, month };
}

/** pickerValue turns what the model holds into the month the widget points
 *  at: the month itself, January for a bare year, today for a running period
 *  — so a date the document holds is never shown as an empty calendar. The
 *  day of a day picker is the first of the month, except for a running period
 *  where it is today. */
function pickerValue(text, type) {
  const value = String(text || '').trim();
  if (!value) return '';
  let year;
  let month;
  let day = 1;
  if (PRESENT_WORDS.has(value.toLowerCase())) {
    const now = new Date();
    year = now.getFullYear();
    month = now.getMonth() + 1;
    day = now.getDate();
  } else {
    const parsed = monthOf(value);
    if (!parsed) return '';
    year = parsed.year;
    month = parsed.month || 1;
  }
  const iso = year + '-' + String(month).padStart(2, '0');
  return type === 'date' ? iso + '-' + String(day).padStart(2, '0') : iso;
}

// pickedMonth is what the calendar wrote: the month, and only the month.
function pickedMonth(value) {
  return String(value || '').slice(0, 7);
}

/** A period date: one field, the native calendar, writing the month the model
 *  reads. A browser with no calendar gets the plain text field, which is the
 *  only widget that can still be told "present". */
function buildDateField(field, target, onChange) {
  const commit = (value) => { target[field.key] = value; onChange(); };
  const label = fieldLabel(field);
  const current = target[field.key] || '';

  if (!DATE_PICKER) {
    const text = el('input', { type: 'text', class: field.mono ? 'mono' : null, 'aria-label': label });
    text.value = current;
    text.addEventListener('input', () => commit(text.value));
    return el('div', { class: 'field' }, [el('label', { text: label }), text]);
  }

  const picker = el('input', {
    type: DATE_PICKER,
    class: (field.mono ? 'mono ' : '') + 'date-picker',
    'aria-label': label,
  });
  picker.value = pickerValue(current, DATE_PICKER);

  // While the segments are being typed the model only follows a month the
  // widget can already name: an incomplete date reads back as "", and wiping
  // the model with it on every keystroke would be wrong. The clearing itself
  // comes with "change", where "" means the candidate emptied the field.
  picker.addEventListener('input', () => {
    if (picker.value) commit(pickedMonth(picker.value));
  });
  picker.addEventListener('change', () => commit(pickedMonth(picker.value)));

  return el('div', { class: 'field' }, [el('label', { text: label }), picker]);
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
    // A list the model never filled arrives as null, not as []: the server
    // marshals a nil slice as null and Normalize drops the empty ones, so an
    // imported resume has one per unused section. The array has to be attached
    // to the resume here, otherwise the push below goes into a copy the next
    // render never reads and the entry added to an empty section vanishes.
    const list = Array.isArray(data[section.key]) ? data[section.key] : [];
    data[section.key] = list;
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

    const cards = [
      { key: 'identity', label: sectionLabel(schema.identity), node: buildSingleton(schema.identity, data, onChange) },
      { key: 'contact', label: sectionLabel(schema.contact), node: buildSingleton(schema.contact, data, onChange) },
      ...schema.sections.map((section) => ({
        key: section.key,
        label: sectionLabel(section),
        count: Array.isArray(data[section.key]) ? data[section.key].length : 0,
        node: buildSection(section, data, onChange),
      })),
    ];
    target.replaceChildren(buildSectionTabs(cards));
    target.scrollTop = scroll;
    renderSessionList();
  });
}

// activeSection is the section the editor is showing. It lives outside the
// render because rerender() rebuilds every card from scratch: without it, adding
// a job would throw the user back to Identité on every click.
let activeSection = 'identity';

// An imported CV is a new document, so the review starts at the top. Staying on
// whatever section happened to be open would hide the name and the experience
// behind a tab the user never chose for this file.
function resetSection() { activeSection = 'identity'; }

/** buildSectionTabs turns the stack of section cards into a tabbed editor.
 *
 *  Nine sections stacked came to some twelve thousand pixels with a real CV,
 *  seventeen screens on a phone, and Expérience alone was four thousand of
 *  them. Finding Langues meant scrolling past every job. One section at a time
 *  costs a row of tabs and makes the rest reachable in one tap.
 *
 *  Every card stays in the DOM. Only the selected one is displayed, because the
 *  inputs hold live listeners and the model is written on the fly: building the
 *  cards on demand would mean rewiring them, and hiding is both cheaper and
 *  impossible to get wrong. */
function buildSectionTabs(cards) {
  if (!cards.some((c) => c.key === activeSection)) activeSection = cards[0].key;

  const wrap = el('div', { class: 'sectiontabs' });
  const bar = el('nav', { class: 'sectiontabs-bar', role: 'tablist' });
  const panels = el('div', { class: 'sectiontabs-panels' });

  const show = (key) => {
    activeSection = key;
    for (const btn of bar.children) {
      const on = btn.dataset.section === key;
      btn.classList.toggle('is-active', on);
      btn.setAttribute('aria-selected', String(on));
    }
    for (const panel of panels.children) {
      panel.classList.toggle('is-active', panel.dataset.section === key);
    }
  };

  for (const card of cards) {
    const label = el('span', { text: card.label });
    const children = [label];
    // The count is the reason to open a section, or to leave it alone: an empty
    // one says so on the tab rather than after a tap.
    if (card.count) children.push(el('span', { class: 'tabcount', text: String(card.count) }));

    const btn = el('button', {
      type: 'button',
      class: 'sectiontab',
      role: 'tab',
      'data-section': card.key,
      onclick: () => show(card.key),
    }, children);
    bar.appendChild(btn);

    const panel = el('div', { class: 'sectiontabs-panel', 'data-section': card.key, role: 'tabpanel' });
    panel.appendChild(card.node);
    panels.appendChild(panel);
  }

  wrap.appendChild(bar);
  wrap.appendChild(panels);
  show(activeSection);
  return wrap;
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

/* --------------------------------------------------------------- refresh */

// One event per output panel, and only the panel that is on screen is asked
// for: every keystroke used to post the whole resume three times, to a
// diagnostic and a plain text panel sitting hidden under the active tab.
// page.html gives each panel its own trigger; the panels that are not asked
// for here are refreshed when their tab is selected.
const PANEL_EVENTS = { preview: 'cv:preview', report: 'cv:report', text: 'cv:text' };

/** The output panel the tabs have selected, the one any refresh targets. */
function activePanel() {
  const tab = document.querySelector('.tab.is-active');
  return (tab && tab.dataset.tab) || 'preview';
}

/** Whether the output column is on screen. Above 1000px it never hides; on a
 *  phone it is only there when the "Aperçu" view is selected. Without
 *  matchMedia, as in the test DOM, the wide layout is assumed: a refresh that
 *  happens when it should not is a wasted request, one that never happens is
 *  a preview that stays empty. */
function outputVisible() {
  if (typeof window.matchMedia === 'function' && !window.matchMedia('(min-width: 1000px)').matches) {
    const view = document.querySelector('.viewtab.is-active');
    return !view || view.dataset.view === 'output';
  }
  return true;
}

/** refreshOutput asks for the one panel a reader can see, and for none when
 *  the form alone is on screen: whatever it cancels is asked for again the
 *  moment the output comes back. */
function refreshOutput() {
  clearTimeout(refreshTimer);
  refreshTimer = 0;
  lastRefresh = Date.now();
  if (!outputVisible()) return;
  refreshPanel(activePanel());
}

/** refreshPanel is the refresh a deliberate action earns: opening a tab that
 *  may have been left behind by every edit since it was last shown. */
function refreshPanel(key) {
  const event = PANEL_EVENTS[key];
  if (event) document.body.dispatchEvent(new CustomEvent(event));
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

// A preview that waits for the hands to leave the keyboard is not a preview:
// the resume is redrawn while it is still being written. The window keeps
// typing from becoming one request per key: the first change of a burst is
// answered at once, the ones that follow wait for the window to reopen, and
// the last of the burst is never dropped. The server draws the resume in about
// a tenth of a millisecond, so what is left to save is the round trip itself.
const REFRESH_MS = 90;

function scheduleRefresh() {
  const wait = REFRESH_MS - (Date.now() - lastRefresh);
  if (wait <= 0) {
    refreshOutput();
    return;
  }
  clearTimeout(refreshTimer);
  refreshTimer = setTimeout(refreshOutput, wait);
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
  refreshOutput();
}

function wireSessions() {
  document.getElementById('session').addEventListener('change', (e) => selectSession(e.target.value));

  document.getElementById('session-new').addEventListener('click', () => {
    newSession(null, T.label('Nouveau CV', 'New resume'));
    rerender();
    sync();
    refreshOutput();
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
    refreshOutput();
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
    refreshOutput();
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
      if (isPDF(file)) await importPDF(file);
      else await importJSON(file);
    } catch (err) {
      // The progress line is shown while the file is read, so a failure has to
      // clear it: leaving it in place makes a broken import look like one still
      // running.
      hideProgress();
      // A bug in this file reaches here as a ReferenceError or a TypeError with
      // a message that means nothing to the candidate. The message is still
      // logged, because that is what makes it reportable, but what is shown
      // says which step failed.
      console.error('import failed', err);
      const bug = err instanceof ReferenceError || err instanceof TypeError;
      toast(bug
        ? T.label('Import impossible : erreur interne de l’éditeur.',
                  'Import failed: an internal editor error.')
        : err.message, 'error');
    } finally {
      picker.value = '';
    }
  });
}

function isPDF(file) {
  return /\.pdf$/i.test(file.name || '') || file.type === 'application/pdf';
}

/* A JSON file is the schema's own format, so the server answers with the resume
   it parsed and the draft replaces the session directly. */
async function importJSON(file) {
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
  // Both of these decide what the next render shows, so they come before it.
  resetSection();
  rerender();
  sync();
  refreshOutput();
  // The point of an import is the form it filled: on a phone the preview tab
  // may be the one on screen, and leaving it there hides the whole result.
  showView('edit');
  toast(T.label('Importé', 'Imported'));
}

/* A PDF is a rendering, not data, and the reading of one is a guess from start
   to finish: the layout decides what a line is, and the heuristics decide what
   a line means. The file is decoded here, where it already is, and only the
   text it yields is posted. The server never sees the file, which is what keeps
   /api/import as cheap as the rest of the API.

   What comes back fills the form directly. The draft is the editable document:
   every field the reader guessed is a field the candidate can see and correct
   in place, which is a better review than a summary that only counts them. The
   session it replaces is a new one, so the work already in the editor stays
   where it is and the import is undone by switching back to it. */
async function importPDF(file) {
  showProgress(T.label('Lecture du PDF…', 'Reading the PDF…'));

  const pdfjs = await import('/static/vendor/pdf.min.mjs');
  pdfjs.GlobalWorkerOptions.workerSrc = '/static/vendor/pdf.worker.min.mjs';
  const { extractText, NoTextLayer } = await import('/static/pdf_import.mjs');

  const bytes = await file.arrayBuffer();
  let read;
  try {
    read = await extractText(bytes, pdfjs, (n, total) => {
      showProgress(T.label(`Lecture du PDF… page ${n}/${total}`, `Reading the PDF… page ${n}/${total}`));
    });
  } catch (err) {
    if (err instanceof NoTextLayer || err.name === 'NoTextLayer') {
      throw new Error(T.label(
        'Ce PDF est scanné : il n’a pas de couche texte. Un scan demande de l’OCR, ' +
        'que cet outil ne fait pas. Exporte ton CV depuis un document texte, ou importe son JSON.',
        'This PDF is a scan and has no text layer. A scan needs OCR, which this tool does not do. ' +
        'Export the resume from a text document, or import its JSON.'));
    }
    throw new Error(T.label('PDF illisible : ', 'Unreadable PDF: ') + (err.message || err));
  }

  const res = await fetch('/api/import', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ text: read.text }),
  });
  const body = await res.json().catch(() => ({}));
  if (!res.ok) {
    throw new Error(body.error || T.label('Import impossible', 'Import failed'));
  }

  hideProgress();
  newSession(body.resume, (file.name || 'resume').replace(/\.pdf$/i, '') || T.unnamed());
  persist();
  // resetSection decides what the next render shows, so it comes before it.
  resetSection();
  rerender();
  sync();
  refreshOutput();
  // The fields are the review, so they are what must be on screen: a phone
  // sitting on the preview tab would otherwise hide everything just imported.
  showView('edit');

  // A section the schema had nowhere to put is content the candidate wrote and
  // the form does not show. Counting it in a toast is the only place left to
  // say so now that there is no summary panel.
  const unmapped = body.unmapped || [];
  toast(unmapped.length
    ? T.label(
        `Importé — à vérifier. Non repris : ${unmapped.join(', ')}`,
        `Imported — check it. Not carried over: ${unmapped.join(', ')}`)
    : T.label('Importé — c’est un brouillon, vérifie chaque champ.',
              'Imported — this is a draft, check every field.'),
    unmapped.length ? 'error' : 'info');
}

/* The progress line reuses the toast: reading a PDF takes seconds, and a button
   that does nothing visible for that long looks broken. */
function showProgress(message) {
  const node = document.getElementById('toast');
  if (!node) return;
  node.textContent = message;
  node.dataset.kind = 'info';
  node.hidden = false;
  clearTimeout(Number(node.dataset.timer) || 0);
}

function hideProgress() {
  const node = document.getElementById('toast');
  if (node) node.hidden = true;
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
      const was = document.querySelector('.tab.is-active');
      for (const other of document.querySelectorAll('.tab')) {
        const active = other === tab;
        other.classList.toggle('is-active', active);
        other.setAttribute('aria-selected', String(active));
      }
      for (const panel of document.querySelectorAll('.tabpanel')) {
        panel.classList.toggle('is-active', panel.id === 'panel-' + tab.dataset.tab);
      }
      // The panel that was hidden was not asked for while it was: opening it
      // is the moment to bring it up to date with every edit it missed.
      if (was !== tab) refreshPanel(tab.dataset.tab);
    });
  }
}

// wireViewTabs switches the phone between the form and the output.
//
// The panels are shown by a class rather than by a style, so that the media
// query at 1000px can put both back on screen without this code being told:
// widening a window must not leave half the page hidden behind a tab bar that
// is no longer displayed.
// showView switches the phone to one of the two panels. It is a no-op on a wide
// screen, where both are on display and the tab bar is hidden.
let showView = () => {};

function wireViewTabs() {
  const tabs = [...document.querySelectorAll('.viewtab')];
  if (tabs.length === 0) return;

  function show(view) {
    for (const tab of tabs) {
      const active = tab.dataset.view === view;
      tab.classList.toggle('is-active', active);
      tab.setAttribute('aria-selected', String(active));
    }
    for (const panel of document.querySelectorAll('[data-view-panel]')) {
      panel.classList.toggle('is-active', panel.dataset.viewPanel === view);
    }
    // The two panels scroll independently, and the tab bar sits at the top of
    // the page: switching while halfway down the form would otherwise land the
    // user in the middle of the preview with no idea where they are.
    window.scrollTo(0, 0);
    // The output was not refreshed while the form alone was on screen, so
    // opening it is when the preview of everything typed has to be asked for.
    if (view === 'output') refreshOutput();
  }

  for (const tab of tabs) {
    tab.addEventListener('click', () => show(tab.dataset.view));
  }
  showView = show;
  show('edit');
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
  wireViewTabs();
  wireSessions();
  wireFiles();
  wireDownloads();

  document.getElementById('ascii').addEventListener('change', () => {
    sync();
    refreshOutput();
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
  // The form is drawn on the next frame, so sync once it exists. The refresh
  // goes with it, for the same reason: the preview is filled by this call and
  // not by the "load" trigger, which would post a form still empty.
  requestAnimationFrame(() => {
    sync();
    refreshOutput();
  });
}

document.addEventListener('DOMContentLoaded', boot);
