/* A miniature DOM, just enough to run app.js outside a browser, plus the checks
   that the Go tests cannot reach: how the editor writes the identity, which
   buttons a singleton shows, and what it hands to the API.

   Usage: node dom_test.mjs <fixture.json>   (ids and schema, produced by Go so
   the test runs against the real page and the real schema) */

import { readFileSync } from 'node:fs';

const fixture = JSON.parse(readFileSync(process.argv[2], 'utf8'));
const ids = fixture.ids;
const failures = [];
const check = (ok, message) => { if (!ok) failures.push(message); };

/* ----------------------------------------------------------------- the DOM */

class Node {
  constructor(tag) {
    this.tagName = (tag || '').toUpperCase();
    this.children = [];
    this.attributes = {};
    this.dataset = {};
    this.listeners = {};
    this.className = '';
    this.hidden = false;
    this.disabled = false;
    this._text = '';
    this.value = '';
  }
  get id() { return this.attributes.id || ''; }
  set id(v) { this.attributes.id = v; }
  setAttribute(k, v) {
    this.attributes[k] = v;
    if (k.startsWith('data-')) this.dataset[k.slice(5).replace(/-(\w)/g, (_, c) => c.toUpperCase())] = v;
  }
  getAttribute(k) { return this.attributes[k] ?? null; }
  appendChild(child) { this.children.push(child); child.parentNode = this; return child; }
  replaceChildren(...nodes) { this.children = []; for (const n of nodes) this.appendChild(n); }
  remove() { const p = this.parentNode; if (p) p.children = p.children.filter((c) => c !== this); }
  set textContent(v) { this._text = v; this.children = []; }
  get textContent() {
    return this._text || this.children.map((c) => c.textContent).join('');
  }
  set className(v) { this._class = v; }
  get className() { return this._class || ''; }
  addEventListener(type, fn) { (this.listeners[type] ||= []).push(fn); }
  dispatchEvent(event) {
    if (!('target' in event) || event.target === undefined) {
      event = Object.assign({ target: this }, event);
    }
    for (const fn of this.listeners[event.type] || []) fn(event);
    return true;
  }
  click() { this.dispatchEvent({ type: 'click', target: this }); }
  get classList() {
    const self = this;
    return {
      toggle(name, on) { const has = self.className.split(' ').includes(name); const want = on === undefined ? !has : on; self.className = want ? (self.className + ' ' + name).trim() : self.className.split(' ').filter((c) => c && c !== name).join(' '); return want; },
      contains: (name) => self.className.split(' ').includes(name),
    };
  }
  descendants() {
    const out = [];
    const walk = (n) => { for (const c of n.children) { out.push(c); walk(c); } };
    walk(this);
    return out;
  }
  matches(selector) {
    // A compound selector like ".tab.is-active" asks for every one of its
    // classes: the editor picks the open tab that way.
    if (selector.startsWith('.')) return selector.slice(1).split('.').every((c) => this.classList.contains(c));
    if (selector.startsWith('#')) return this.id === selector.slice(1);
    if (selector.startsWith('[')) {
      const m = /^\[([^=\]]+)(?:="([^"]*)")?\]$/.exec(selector);
      if (!m) return false;
      return m[2] === undefined ? this.getAttribute(m[1]) !== null : this.getAttribute(m[1]) === m[2];
    }
    return this.tagName === selector.toUpperCase();
  }
  querySelectorAll(selector) { return this.descendants().filter((n) => n.matches(selector)); }
  querySelector(selector) { return this.querySelectorAll(selector)[0] || null; }
}

const document = {
  createElement: (tag) => new Node(tag),
  createTextNode: (text) => { const n = new Node('#text'); n._text = text; return n; },
  getElementById(id) { return document.documentElement.querySelector('#' + id); },
  querySelector: (s) => document.documentElement.querySelector(s),
  querySelectorAll: (s) => document.documentElement.querySelectorAll(s),
  addEventListener: (t, fn) => { (documentListeners[t] ||= []).push(fn); },
  body: new Node('body'),
};
const documentListeners = {};
document.documentElement = new Node('html');
document.body = new Node('body');
document.documentElement.appendChild(document.body);

// The skeleton page.html declares, rebuilt here from the ids Go extracted.
for (const id of ids) {
  const node = new Node(id === 'schema-target' || id === 'data' ? 'div' : id === 'import-file' ? 'input' : 'div');
  node.id = id;
  if (id === 'data' || id === 'name-field' || id === 'ascii-field') node.value = '';
  document.body.appendChild(node);
}
for (const tab of ['preview', 'report', 'text']) {
  const t = new Node('button'); t.className = 'tab'; t.setAttribute('data-tab', tab);
  const p = new Node('div'); p.id = 'panel-' + tab; p.className = 'tabpanel';
  document.body.appendChild(t); document.body.appendChild(p);
}
for (const format of ['docx', 'pdf', 'adoc', 'txt']) {
  const b = new Node('button'); b.setAttribute('data-format', format);
  document.body.appendChild(b);
}

/* ------------------------------------------------------------- the browser */

const storage = new Map();
const localStorage = {
  getItem: (k) => (storage.has(k) ? storage.get(k) : null),
  setItem: (k, v) => storage.set(k, String(v)),
  removeItem: (k) => storage.delete(k),
};
let stored = null;      // what the last API call received
const schemaServed = fixture.schema;
const window = {
  prompt: () => 'Renommé',
  confirm: () => true,
  addEventListener: () => {},
};
const queue = [];
const requestAnimationFrame = (fn) => { queue.push(fn); return queue.length; };
const runFrames = () => { let guard = 0; while (queue.length && guard++ < 50) queue.shift()(); };
globalThis.document = document;
globalThis.localStorage = localStorage;
globalThis.window = window;
globalThis.requestAnimationFrame = requestAnimationFrame;
globalThis.fetch = async (url, init) => {
  if (String(url).indexOf('/api/schema') === 0) {
    return { ok: true, json: async () => schemaServed };
  }
  stored = { url: String(url), body: init && init.body };
  if (String(url).indexOf('/api/render') === 0) {
    return { ok: true, blob: async () => new Blob(['x']), headers: { get: () => 'attachment; filename="a.pdf"' } };
  }
  return { ok: true, json: async () => JSON.parse(String(init.body)) };
};

/* ------------------------------------------------------------------ run it */

// The store is seeded with the shape the server actually answers: Go marshals
// a nil slice as null and Normalize drops the empty lists, so every section an
// import left untouched comes back as null rather than []. The editor must
// repair that, or the first add into an empty list goes nowhere.
storage.set('atscv.sessions.v1', JSON.stringify({
  sessions: [{
    id: 'seed',
    name: 'Seed',
    updatedAt: 1,
    data: {
      lang: 'fr', name: '', headline: '', summary: '',
      contact: { email: '', phone: '', city: '', region: '', country: '', note: '', links: null },
      // One entry whose dates the calendar must always show: a month at its
      // own month, a running period at today — and the model untouched.
      education: [{ degree: 'Master 2', school: 'Université', start: '2022-09', end: 'present' }],
      // Two certifications for the shapes no calendar points at by
      // default: a bare year, a month written with its name, and a
      // month/year pair as an import writes one.
      certifications: [
        { name: 'CKA', issuer: 'CNCF', date: '2021' },
        { name: 'Kubernetes', issuer: 'CNCF', date: 'mars 2019' },
        { name: 'Terraform', issuer: 'HashiCorp', date: '12/2018' },
      ],
      experience: null, skills: [],
      projects: [], activities: [], languages: [],
    },
  }],
  current: 'seed',
}));

const src = readFileSync(new URL('./app.js', import.meta.url), 'utf8');
new Function(src)();
for (const fn of documentListeners.DOMContentLoaded || []) await fn();
runFrames();

const target = document.getElementById('schema-target');
const snapshot = () => JSON.parse(document.getElementById('data').value || '{}');
const fields = () => target.descendants().filter((n) => n.classList.contains('field'));
const fieldOf = (key) => {
  const section = [schemaServed.identity, schemaServed.contact].concat(schemaServed.sections)
    .find((s) => s.fields.some((f) => f.key === key));
  const field = section.fields.find((f) => f.key === key);
  const label = schemaServed.lang === 'en' ? (field.label_en || field.label_fr) : field.label_fr;
  const node = fields().find((f) => (f.textContent || '').indexOf(label) === 0);
  return node ? node.descendants().find((n) => n.tagName === 'INPUT' || n.tagName === 'TEXTAREA') : null;
};
const labelOf = (section) => (schemaServed.lang === 'en' ? (section.label_en || section.label_fr) : section.label_fr);
const cardOf = (section) => target.descendants().find((n) => n.tagName === 'SECTION' && (n.textContent || '').indexOf(labelOf(section)) === 0);

/* 0. a list the model never wrote is turned into a real array, so the add
      button has somewhere to push to */
check(Array.isArray(snapshot().experience),
  'a null section list was not repaired: ' + JSON.stringify(snapshot().experience));

/* 1. every section of the schema got a card, and the blocks that are not
      lists got their fields */
const cards = target.descendants().filter((n) => n.tagName === 'SECTION' && n.classList.contains('card'));
check(cards.length === schemaServed.sections.length + 2,
  'cards = ' + cards.length + ', want ' + (schemaServed.sections.length + 2));
for (const section of [schemaServed.identity, schemaServed.contact].concat(schemaServed.sections)) {
  check(!!cardOf(section), 'no card for the section ' + section.key);
}
check(!!fieldOf('name') && !!fieldOf('email'), 'the identity or the contact block has no input');

/* 2. the identity writes to the root, not to a nested object */
const name = fieldOf('name');
name.value = 'Jeanne Rousseau';
name.dispatchEvent({ type: 'input' });
const headline = fieldOf('headline');
headline.value = 'Ingénieure Go';
headline.dispatchEvent({ type: 'input' });
const email = fieldOf('email');
email.value = 'jeanne@exemple.fr';
email.dispatchEvent({ type: 'input' });

const data = snapshot();
check(data.identity === undefined, 'the editor created a nested "identity" object: ' + JSON.stringify(Object.keys(data)));
check(data.name === 'Jeanne Rousseau', 'the name did not reach the root of the model: ' + JSON.stringify(data.name));
check(data.headline === 'Ingénieure Go', 'the headline did not reach the root of the model');
check(data.contact && data.contact.email === 'jeanne@exemple.fr', 'the contact did not reach its own block');
for (const key of ['experience', 'education', 'skills', 'languages', 'certifications', 'projects', 'activities']) {
  check(Array.isArray(data[key]), key + ' is not an array');
}

/* 3. a singleton offers no delete and no duplicate, a repeatable entry offers both */
const identity = cardOf(schemaServed.identity);
const marks = (node) => node.querySelectorAll('button').map((b) => b.textContent);
check(!marks(identity).includes('✕'), 'the identity offers a delete button: ' + marks(identity).join(' '));
check(!marks(identity).includes('⧉'), 'the identity offers a duplicate button: ' + marks(identity).join(' '));
for (const button of target.descendants().filter((n) => n.tagName === 'BUTTON')) {
  check(button.getAttribute('type') === 'button', 'a button without type=button would submit the form: ' + button.textContent);
}

const experience = schemaServed.sections.find((s) => s.key === 'experience');
check(!!cardOf(experience).querySelectorAll('button').find((b) => /^\+ /.test(b.textContent)),
  'the experience section has no add button');

/* 4. adding, copying and deleting an entry */
const addExperience = cardOf(experience).querySelectorAll('button').find((b) => /^\+ /.test(b.textContent));
addExperience.click();
runFrames();
check(snapshot().experience.length === 1, 'the add button did not add an entry: ' + snapshot().experience.length);

const entryCard = () => target.descendants().find((n) => n.tagName === 'ARTICLE');
check(!!entryCard(), 'the new entry was not drawn');
const company = fieldOf('company');
company.value = 'Acme';
company.dispatchEvent({ type: 'input' });
runFrames();

const dup = cardOf(experience).querySelectorAll('button').find((b) => b.textContent === '⧉');
check(!!dup, 'no duplicate button on a repeatable entry');
dup.click();
runFrames();
const after = snapshot().experience;
check(after.length === 2, 'the duplicate button did not copy the entry: ' + after.length);
check(after[0].company === 'Acme' && after[1].company === 'Acme', 'the copy did not carry the fields: ' + JSON.stringify(after.map((e) => e.company)));

const del = cardOf(experience).querySelectorAll('button').find((b) => b.textContent === '✕');
del.click();
runFrames();
check(snapshot().experience.length === 1, 'the delete button did not remove the entry');

/* 4b. an empty list field takes an item, even though the model held null */
const contactSection = schemaServed.contact;
const addLink = cardOf(contactSection).querySelectorAll('button').find((b) => /^\+ /.test(b.textContent));
check(!!addLink, 'the contact block has no add button for the links');
if (addLink) { addLink.click(); runFrames(); }
check(Array.isArray(snapshot().contact.links) && snapshot().contact.links.length === 1,
  'the add button did not add a link to the empty list: ' + JSON.stringify(snapshot().contact.links));

/* 4c. a date is one field — the native calendar — and it writes the month
      the parser reads back into the model */
const experienceSection = schemaServed.sections.find((s) => s.key === 'experience');
const startDef = experienceSection.fields.find((f) => f.key === 'start');
check(startDef && startDef.kind === 'date', 'the start field is not published as a date: ' + (startDef && startDef.kind));
const startDate = fieldOf('start');
const startField = startDate && fields().find((f) => f.descendants().indexOf(startDate) >= 0);
check(!!startDate, 'the date field has no input');
check(!!startField && startField.descendants().filter((n) => n.tagName === 'INPUT').length === 1,
  'the date field draws more than one input');
check(!!startDate && ['month', 'date'].includes(startDate.getAttribute('type')),
  'the date input is a "' + (startDate && startDate.getAttribute('type')) + '", want the native calendar');
if (startDate) {
  startDate.value = '2023-07';
  startDate.dispatchEvent({ type: 'input' });
  runFrames();
  check(snapshot().experience[0].start === '2023-07',
    'the calendar wrote ' + JSON.stringify(snapshot().experience[0].start) + ' for a july pick, want 2023-07');
  startDate.value = '2023-11';
  startDate.dispatchEvent({ type: 'change' });
  runFrames();
  check(snapshot().experience[0].start === '2023-11',
    'committing the calendar wrote ' + JSON.stringify(snapshot().experience[0].start) + ', want 2023-11');
  startDate.value = '';
  startDate.dispatchEvent({ type: 'change' });
  runFrames();
  check(snapshot().experience[0].start === '',
    'clearing the calendar left ' + JSON.stringify(snapshot().experience[0].start));
}

/* 4d. a date the model holds is always shown by the calendar — a month at
      its own month, a bare year at January, a running period at today — and
      what the calendar points at reaches the model only when it is picked */
const fieldLabelOf = (section, key) => {
  const def = section.fields.find((f) => f.key === key);
  return schemaServed.lang === 'en' ? (def.label_en || def.label_fr) : def.label_fr;
};
// The inputs of a card whose field label starts with that label, in order:
// a repeatable section draws one field per entry.
const inputsOf = (card, label) => card.descendants()
  .filter((n) => n.classList.contains('field') && (n.textContent || '').indexOf(label) === 0)
  .map((n) => n.descendants().find((d) => d.tagName === 'INPUT'));

const educationSection = schemaServed.sections.find((s) => s.key === 'education');
const educationCard = cardOf(educationSection);
const eduStart = educationCard && inputsOf(educationCard, fieldLabelOf(educationSection, 'start'))[0];
const eduEnd = educationCard && inputsOf(educationCard, fieldLabelOf(educationSection, 'end'))[0];
check(!!eduStart && eduStart.value === '2022-09',
  'the calendar does not show the month the model holds: ' + JSON.stringify(eduStart && eduStart.value));
check(!!eduEnd && /^\d{4}-\d{2}$/.test(eduEnd.value),
  'a running period shows an empty calendar: ' + JSON.stringify(eduEnd && eduEnd.value));
check(snapshot().education[0].end === 'present',
  'the model lost "present": ' + JSON.stringify(snapshot().education[0].end));

const certificationsSection = schemaServed.sections.find((s) => s.key === 'certifications');
const certificationsCard = cardOf(certificationsSection);
const certDates = certificationsCard
  ? inputsOf(certificationsCard, fieldLabelOf(certificationsSection, 'date'))
  : [];
check(certDates.length === 3, 'the certification dates did not draw: ' + certDates.length);
check(certDates[0] && certDates[0].value === '2021-01',
  'a year alone shows ' + JSON.stringify(certDates[0] && certDates[0].value) + ', want 2021-01');
check(certDates[1] && certDates[1].value === '2019-03',
  'a month name shows ' + JSON.stringify(certDates[1] && certDates[1].value) + ', want 2019-03');
check(certDates[2] && certDates[2].value === '2018-12',
  'a month/year pair shows ' + JSON.stringify(certDates[2] && certDates[2].value) + ', want 2018-12');
check(snapshot().certifications.map((c) => c.date).join(', ') === '2021, mars 2019, 12/2018',
  'the calendar rewrote the dates it only displays: ' +
    JSON.stringify(snapshot().certifications.map((c) => c.date)));

/* 5. the draft survives a reload */
await new Promise((r) => setTimeout(r, 600));
const raw = localStorage.getItem('atscv.sessions.v1');
check(raw !== null, 'nothing was written to localStorage');
const store2 = JSON.parse(raw || '{}');
check(Array.isArray(store2.sessions) && store2.sessions.length >= 1, 'the store has no session');
check(store2.sessions.some((s) => s.data && s.data.name === 'Jeanne Rousseau'), 'the stored draft has no name');
check(store2.current && store2.sessions.some((s) => s.id === store2.current), 'the current session id is dangling');
check(localStorage.getItem('atscv.last.v1') === null, 'the stale last-session key is still written');
check(store2.sessions.every((s) => s.updatedAt > 0), 'a session has no timestamp');

/* 6. renaming, duplicating and removing a session */
const firstId = store2.current;
document.getElementById('session-rename').click();
check(snapshot().name === 'Jeanne Rousseau', 'renaming the session lost the draft');
document.getElementById('session-duplicate').click();
runFrames();
check(snapshot().experience.length === 1, 'duplicating the session lost the draft');
await new Promise((r) => setTimeout(r, 600));
const store3 = JSON.parse(localStorage.getItem('atscv.sessions.v1'));
check(store3.sessions.length === 2, 'the session was not duplicated: ' + store3.sessions.length);
check(store3.current !== firstId, 'the copy did not become the current session');
document.getElementById('session-delete').click();
runFrames();
await new Promise((r) => setTimeout(r, 600));
const store4 = JSON.parse(localStorage.getItem('atscv.sessions.v1'));
check(store4.sessions.length === 1 && store4.current === firstId, 'the session was not deleted: ' + JSON.stringify(store4.sessions.map((s) => s.id)));

/* 7. an edit is drawn while it is written: a keystroke asks at once, a burst
      asks once for the window, and the last keystroke is never dropped */
const refreshes = [];
for (const name of ['cv:preview', 'cv:report', 'cv:text']) {
  document.body.addEventListener(name, () => refreshes.push(name));
}
await new Promise((r) => setTimeout(r, 150)); // the refresh window is open again
fieldOf('headline').value = 'Ingénieure distribuée';
fieldOf('headline').dispatchEvent({ type: 'input' });
check(refreshes.join(',') === 'cv:preview',
  'a keystroke was not drawn as it was typed: [' + refreshes.join(', ') + ']');
refreshes.length = 0;
for (let i = 0; i < 5; i += 1) {
  fieldOf('headline').value = 'Ingénieure distribuée ' + i;
  fieldOf('headline').dispatchEvent({ type: 'input' });
}
check(refreshes.length === 0,
  'five keystrokes asked for ' + refreshes.length + ' refreshes without waiting for the window');
await new Promise((r) => setTimeout(r, 300));
check(refreshes.length === 1,
  'five keystrokes asked for ' + refreshes.length + ' refreshes, want the burst collapsed into 1');
check(snapshot().headline === 'Ingénieure distribuée 4',
  'the last keystroke of the burst never reached the model: ' + snapshot().headline);

/* 7b. opening a tab asks for the panel it was hiding, and only then */
const reportTab = document.querySelectorAll('.tab').find((b) => b.dataset.tab === 'report');
check(!!reportTab, 'no diagnostic tab');
refreshes.length = 0;
reportTab.click();
check(refreshes.join(',') === 'cv:report',
  'opening the diagnostic asked for [' + refreshes.join(', ') + '], want cv:report');
refreshes.length = 0;
reportTab.click();
check(refreshes.length === 0,
  'clicking the tab already open asked again: ' + refreshes.join(', '));

check(document.getElementById('name-field').value === 'Jeanne Rousseau',
  'the name sent to the renderer is not the draft name: ' + document.getElementById('name-field').value);
document.getElementById('ascii').checked = true;
document.getElementById('ascii').dispatchEvent({ type: 'change' });
check(document.getElementById('ascii-field').value === 'on', 'the ascii flag did not follow the checkbox');

const pdfButton = document.querySelectorAll('[data-format]').find((b) => b.dataset.format === 'pdf');
pdfButton.click();
await new Promise((r) => setTimeout(r, 50));
check(stored !== null && stored.url.endsWith('/api/render/pdf'), 'the download did not reach the renderer');
if (stored) {
  const sent = new URLSearchParams(stored.body);
  check(sent.get('data') !== undefined, 'no resume in the download payload');
  check(JSON.parse(sent.get('data')).name === 'Jeanne Rousseau', 'the download does not carry the draft');
  check(sent.get('name') !== undefined, 'no file name in the download payload');
  check(pdfButton.disabled === false, 'the button stayed disabled after the download');
}

if (failures.length) {
  console.error('FAIL');
  for (const f of failures) console.error(' - ' + f);
  process.exit(1);
}
console.log('dom: all checks passed');
