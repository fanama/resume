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
      // add and remove, as the DOM has them: the analysis panel streams an
      // answer behind a class it takes off again when the last token lands.
      add(...names) {
        const kept = self.className.split(' ').filter((c) => c && !names.includes(c));
        self.className = kept.concat(names).join(' ');
      },
      remove(...names) {
        self.className = self.className.split(' ').filter((c) => c && !names.includes(c)).join(' ');
      },
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
// Every id the raw HTML carries is on the real page now: the two rewrite
// buttons ship for real, so the harness gives them to the scripts the same
// way the browser does.
for (const id of ids) {
  const node = new Node(id === 'schema-target' || id === 'data' ? 'div' : id === 'import-file' ? 'input' : 'div');
  node.id = id;
  if (id === 'data' || id === 'name-field' || id === 'ascii-field') node.value = '';
  document.body.appendChild(node);
}
for (const tab of ['preview', 'report', 'text', 'tex', 'ia']) {
  const t = new Node('button'); t.className = 'tab'; t.setAttribute('data-tab', tab);
  const p = new Node('div'); p.id = 'panel-' + tab; p.className = 'tabpanel';
  document.body.appendChild(t); document.body.appendChild(p);
}
for (const format of ['docx', 'pdf', 'adoc', 'txt', 'tex']) {
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
let lastChat = null;    // what the last /chat/completions received, said apart from validate
// The replies the provider endpoint gives when the test asks it for one:
// the harness answers what the tests push, like a real OpenAI server reads
// its own configuration.
const providerReplies = [];
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
  if (String(url).indexOf('/api/job') === 0) {
    return { ok: true, json: async () => ({
      url: 'https://emplois.exemple.fr/annonce/123',
      title: 'Ingénieure Go',
      text: 'Recherche une ingénieure Go, trois ans, Kubernetes.',
      fetchedAt: '2026-10-14T00:00:00Z',
    }) };
  }
  if (String(url).indexOf('/chat/completions') >= 0) {
    stored = { url: String(url), body: init && init.body };
    lastChat = { url: String(url), body: init && init.body };
    const reply = providerReplies.shift();
    if (reply === undefined) return { ok: false, status: 500, json: async () => ({}), text: async () => '' };
    // Un appel que le navigateur refuse (CORS) ne donne jamais de réponse :
    // le fetch jette avant, et le message doit en porter la trace.
    if (reply === 'throw') throw new TypeError('Failed to fetch');
    if (reply && reply.status) {
      return { ok: false, status: reply.status, json: async () => (reply.body || {}), text: async () => JSON.stringify(reply.body || {}) };
    }
    return {
      ok: true,
      status: 200,
      body: (() => {
        const encoded = new TextEncoder().encode(reply);
        let done = false;
        return {
          getReader: () => ({
            read: async () => {
              if (done) return { done: true };
              done = true;
              return { done: false, value: encoded };
            },
          }),
        };
      })(),
    };
  }
  stored = { url: String(url), body: init && init.body };
  if (String(url).indexOf('/api/render') === 0) {
    return { ok: true, blob: async () => new Blob(['x']), headers: { get: () => 'attachment; filename="a.pdf"' } };
  }
  if (String(url).indexOf('/api/text') === 0) {
    return { ok: true, text: async () => 'Jeanne Rousseau\nDéveloppeuse Go' };
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
const analysisSrc = readFileSync(new URL('./analysis.js', import.meta.url), 'utf8');
// Both scripts are deferred classics on the real page: they share the global
// scope, so app.js's statements are reachable from analysis.js. Two separate
// Function scopes would hide them one from the other — one Function holding
// the two sources is what the browser hands the page.
new Function(src + '\n;\n' + analysisSrc)();
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
for (const name of ['cv:preview', 'cv:report', 'cv:text', 'cv:tex']) {
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

/* 7c. the LaTeX panel is asked for when its tab is opened, and only then */
const texTab = document.querySelectorAll('.tab').find((b) => b.dataset.tab === 'tex');
check(!!texTab, 'no LaTeX tab in the editor');
refreshes.length = 0;
if (texTab) texTab.click();
check(refreshes.join(',') === 'cv:tex',
  'opening the LaTeX tab asked for [' + refreshes.join(', ') + '], want cv:tex');
refreshes.length = 0;
if (reportTab) reportTab.click(); // leave the editor on a panel the rest reads

/* 7d. the analysis panel is the one panel no server event answers: opening
      its tab switches to it and asks for nothing, and its button refuses to
      run where there is no WebGPU */
const iaTab = document.querySelectorAll('.tab').find((b) => b.dataset.tab === 'ia');
check(!!iaTab, 'no analysis tab in the editor');
refreshes.length = 0;
if (iaTab) iaTab.click();
check(refreshes.length === 0,
  'opening the analysis asked the server for [' + refreshes.join(', ') + '], want nothing');
// The tabpanels are the nodes wireTabs toggles: the ids loop above has
// already built a plain div for every id on the real page.
const iaPanel = document.querySelectorAll('.tabpanel').find((n) => n.id === 'panel-ia');
check(!!iaPanel && iaPanel.classList.contains('is-active'),
  'the analysis tab did not open its panel');
document.getElementById('ia-run').click();
const iaStatusNode = document.getElementById('ia-status');
check((iaStatusNode.textContent || '').indexOf('WebGPU') >= 0,
  'the analysis did not refuse without WebGPU: ' + JSON.stringify(iaStatusNode.textContent));
check(iaStatusNode.classList.contains('is-error'),
  'the refusal is not painted as an error');
check(document.getElementById('ia-run').disabled === false,
  'the analysis button stayed disabled after the refusal');
// A refusal is not a wait: nothing may still look busy after it.
check(document.getElementById('ia-spinner').hidden === true,
  'the spinner stayed visible after the refusal');
check(document.getElementById('ia-progress').hidden === true,
  'the download bar showed without a download');
check(!document.getElementById('ia-out').classList.contains('is-streaming'),
  'the streaming caret was left behind after the refusal');
// The two rewrite actions live behind buttons the page ships or comments out.
// Where a button is on the page, a refusal must leave it free again; where it
// is commented out, the panel must have kept its other buttons alive without
// it — which is the exact check above: ia-run answered.
const optimizeBtn = document.getElementById('ia-optimize');
const translateBtn = document.getElementById('ia-translate');
check(!optimizeBtn || optimizeBtn.disabled === false,
  'the optimize button, if shipped, stayed disabled after the refusal');
check(!translateBtn || translateBtn.disabled === false,
  'the translate button, if shipped, stayed disabled after the refusal');
if (translateBtn) {
  check((translateBtn.textContent || '').indexOf('anglais') >= 0,
    'the translate button does not name its target: ' +
      JSON.stringify(translateBtn.textContent));
}
check(!!optimizeBtn && !!translateBtn,
  'the rewrite buttons are missing from the served page');

/* 7e. the offer of a job is fetched once, kept in this browser, and named for
      as long as it stays: the URL goes out, the text comes back and is stored,
      and the clear button unties the CV from it again */
const jobUrl = document.getElementById('ia-url');
check(!!jobUrl, 'no URL field for the offer');
jobUrl.value = 'https://emplois.exemple.fr/annonce/123';
document.getElementById('ia-job').click();
await new Promise((r) => setTimeout(r, 100));
const savedOffer = JSON.parse(localStorage.getItem('atscv.offer.v1') || 'null');
check(!!savedOffer && savedOffer.title === 'Ingénieure Go',
  'the fetched offer is not kept in this browser: ' + JSON.stringify(savedOffer));
const offerLine = document.getElementById('ia-offer');
check((offerLine.textContent || '').indexOf('Ingénieure Go') >= 0 && !offerLine.hidden,
  'the saved offer is not named in the panel: ' + JSON.stringify(offerLine.textContent));
check(document.getElementById('ia-offer-clear').hidden === false,
  'no way to retire the saved offer');
document.getElementById('ia-offer-clear').click();
await new Promise((r) => setTimeout(r, 100));
check(localStorage.getItem('atscv.offer.v1') === null,
  'the stored offer survived its own clear button');
check(document.getElementById('ia-offer').hidden === true,
  'the offer line stayed visible after the clear');

/* 7f. a provider the reader owns: typed once, kept in this browser, and
      every run after that goes to it — no WebGPU asked for, the answer
      streamed from the endpoint the fields name. Retiring it gives the
      panel back to the local model. */
const provUrlNode = document.getElementById('ia-provider-url');
const provModelNode = document.getElementById('ia-provider-model');
const provKeyNode = document.getElementById('ia-provider-key');
const provSaveNode = document.getElementById('ia-provider-save');
const provClearNode = document.getElementById('ia-provider-clear');
check(!!provUrlNode && !!provModelNode && !!provKeyNode && !!provSaveNode && !!provClearNode,
  'the provider block is missing a field or a button');
provUrlNode.value = 'https://api.exemple.fr/v1';
provModelNode.value = 'gpt-mini';
provKeyNode.value = 'sk-test';
provSaveNode.click();
const savedLLM = JSON.parse(localStorage.getItem('atscv.llm.v1') || 'null');
check(!!savedLLM && savedLLM.baseUrl === 'https://api.exemple.fr/v1' &&
    savedLLM.model === 'gpt-mini' && savedLLM.apiKey === 'sk-test',
  'the provider fields were not kept: ' + JSON.stringify(savedLLM));
check(!provClearNode.hidden, 'no way to retire the saved provider');
check((document.getElementById('ia-note').textContent || '').indexOf('gpt-mini') >= 0,
  'the panel does not name the provider it now answers through: ' +
    JSON.stringify(document.getElementById('ia-note').textContent));
providerReplies.push(
  'data: {"choices":[{"delta":{"content":"Analyse"}}]}\n\n' +
  'data: {"choices":[{"delta":{"content":" du CV"}}]}\n\n' +
  'data: [DONE]\n\n'
);
document.getElementById('ia-run').click();
await new Promise((r) => setTimeout(r, 100));
check(stored !== null && stored.url === 'https://api.exemple.fr/v1/chat/completions',
  'the analysis did not call the provider the panel named: ' + JSON.stringify(stored && stored.url));
if (stored && stored.body) {
  const sent = JSON.parse(stored.body);
  check(sent.model === 'gpt-mini', 'the request did not carry the model: ' + JSON.stringify(sent.model));
  check(sent.stream === true, 'the request is not a stream request');
  check(Array.isArray(sent.messages) && sent.messages.length >= 2, 'no conversation in the request');
}
const llmOut = document.getElementById('ia-out');
check(!llmOut.hidden && llmOut.textContent === 'Analyse du CV',
  'the streamed answer did not reach the panel: ' + JSON.stringify(llmOut.textContent));
check((document.getElementById('ia-status').textContent || '').indexOf('Réponse en ') >= 0 ||
    (document.getElementById('ia-status').textContent || '').indexOf('Answer in ') >= 0,
  'the answer never finished: ' + JSON.stringify(document.getElementById('ia-status').textContent));
check(document.getElementById('ia-run').disabled === false,
  'the analysis button stayed disabled after a provider run');
// Un serveur qui refuse l'appel navigateur (CORS) : le fetch jette, et le
// message nomme la pré-demande OPTIONS et l'URL exacte, pas un vide.
providerReplies.push('throw');
document.getElementById('ia-run').click();
await new Promise((r) => setTimeout(r, 100));
let providerFail = document.getElementById('ia-status').textContent || '';
check(providerFail.indexOf('OPTIONS') >= 0 && providerFail.indexOf('chat/completions') >= 0,
  'a refused call did not name the preflight and the URL: ' + JSON.stringify(providerFail));
// Un 404 porté par une base sans /v1 reçoit le conseil qui la complète.
provUrlNode.value = 'https://api.exemple.fr';
provSaveNode.click();
check((document.getElementById('ia-status').textContent || '').indexOf('gpt-mini') >= 0,
  'saving a base without /v1 refused the save');
providerReplies.push({ status: 404 });
document.getElementById('ia-run').click();
await new Promise((r) => setTimeout(r, 100));
providerFail = document.getElementById('ia-status').textContent || '';
check(providerFail.indexOf('HTTP 404') >= 0 && providerFail.indexOf('/v1') >= 0,
  'a 404 on a /v1-less base stayed silent about it: ' + JSON.stringify(providerFail));
check(document.getElementById('ia-run').disabled === false,
  'the analysis button stayed disabled after a refused provider run');
// An incomplete config is refused, not saved half-way.
provUrlNode.value = 'localhost';
provSaveNode.click();
check(localStorage.getItem('atscv.llm.v1') === null ||
    /^https?:\/\//.test(JSON.parse(localStorage.getItem('atscv.llm.v1')).baseUrl),
  'a provider URL without a scheme was saved');
// Retiring the provider hands the panel back to the local model.
provUrlNode.value = 'https://api.exemple.fr/v1';
provModelNode.value = 'gpt-mini';
provSaveNode.click();
provClearNode.click();
check(localStorage.getItem('atscv.llm.v1') === null, 'the retired provider is still active');
check(provClearNode.hidden === true, 'the clear button stayed visible with nothing to retire');

/* 7g. « Traduire » — the provider answers the whole translated document and
      the read-back verdict checks every pinned field; the new version (a new
      session) opens beside the original, never over it. */
const sessionsOf = () => JSON.parse(localStorage.getItem('atscv.sessions.v1') || '{}');
const beforeTranslate = sessionsOf();
const sourceData = beforeTranslate.sessions.find((s) => s.id === beforeTranslate.current).data;
const translated = JSON.parse(JSON.stringify(sourceData));
translated.lang = 'en';
translated.headline = 'Distributed Go engineer';
// The content of a delta is the TEXT of the document, which is itself a JSON
// string inside the payload — hence the second stringify.
providerReplies.push(
  'data: {"choices":[{"delta":{"content":' + JSON.stringify(JSON.stringify(translated)) + '},"finish_reason":"stop"}]}\n\n' +
  'data: [DONE]\n\n'
);
// Re-activating the provider: the retirement check above cleared it.
provUrlNode.value = 'https://api.exemple.fr/v1';
provModelNode.value = 'gpt-mini';
provKeyNode.value = 'sk-test';
provSaveNode.click();
translateBtn.click();
await new Promise((r) => setTimeout(r, 150));
const translateCall = lastChat && lastChat.body ? JSON.parse(lastChat.body) : null;
check(!!translateCall && translateCall.response_format &&
    translateCall.response_format.type === 'json_schema' &&
    translateCall.response_format.json_schema.name === 'resume',
  'the translation did not constrain the model with the published grammar: ' +
    JSON.stringify(translateCall && translateCall.response_format));
const afterTranslate = sessionsOf();
check(afterTranslate.sessions.length === beforeTranslate.sessions.length + 1,
  'the translation did not open a new version (a new session): ' + afterTranslate.sessions.length);
const applied = afterTranslate.sessions.find((s) => s.id === afterTranslate.current);
check(!!applied && applied.data && applied.data.lang === 'en',
  'the translated version did not become the current session: ' +
    JSON.stringify(applied && applied.data && applied.data.lang));
check(!!applied && (applied.name || '').indexOf('EN') >= 0,
  'the new version is not named after the translation: ' +
    JSON.stringify(applied && applied.name));
const kept = afterTranslate.sessions.find((s) => s.id === beforeTranslate.current);
check(!!kept && kept.data && kept.data.lang === 'fr',
  'the original version did not survive its own rewrite: ' +
    JSON.stringify(kept && kept.data && kept.data.lang));
check(translateBtn.disabled === false,
  'the translate button stayed disabled after the success');
check((document.getElementById('ia-status').textContent || '').indexOf('Nouvelle session') >= 0 ||
    (document.getElementById('ia-status').textContent || '').indexOf('New session') >= 0,
  'the new version was not announced: ' +
    JSON.stringify(document.getElementById('ia-status').textContent));

/* 7h. « Appliquer » — the last analysis' suggestions ride along in the
      prompt, and a cut answer (finish_reason length) is told back once, not
      a dead end twice. */
const optimized = JSON.parse(JSON.stringify(translated));
optimized.headline = 'Ingénieure distribuée en Go';
providerReplies.push(
  'data: {"choices":[{"delta":{"content":"{\\"lang\\":\\"fr"},"finish_reason":"length"}]}\n\n' +
  'data: [DONE]\n\n'
);
providerReplies.push(
  'data: {"choices":[{"delta":{"content":' + JSON.stringify(JSON.stringify(optimized)) + '},"finish_reason":"stop"}]}\n\n' +
  'data: [DONE]\n\n'
);
optimizeBtn.click();
await new Promise((r) => setTimeout(r, 150));
const optimizeCall = lastChat && lastChat.body ? JSON.parse(lastChat.body) : null;
check(!!optimizeCall && Array.isArray(optimizeCall.messages) && optimizeCall.messages.length === 3,
  'the second attempt was not told what went wrong: messages.length = ' +
    JSON.stringify(optimizeCall && optimizeCall.messages && optimizeCall.messages.length));
check(!!optimizeCall && (optimizeCall.messages[1].content || '').indexOf('Analyse du CV') >= 0,
  'the applied rewrite did not ride the last analysis suggestions: ' +
    JSON.stringify(optimizeCall && optimizeCall.messages &&
      optimizeCall.messages.map((m) => String(m.content || '').slice(0, 80))));
const afterOptimize = sessionsOf();
check(afterOptimize.sessions.length === afterTranslate.sessions.length + 1,
  'the applied rewrite did not open a new version: ' + afterOptimize.sessions.length);
const appliedOpt = afterOptimize.sessions.find((s) => s.id === afterOptimize.current);
check(!!appliedOpt && appliedOpt.data && appliedOpt.data.lang === 'en',
  'the applied rewrite lost the language it was asked to keep: ' +
    JSON.stringify(appliedOpt && appliedOpt.data && appliedOpt.data.lang));
check(!!appliedOpt && (appliedOpt.name || '').indexOf('optimisé') >= 0,
  'the optimized version is not named as one: ' + JSON.stringify(appliedOpt && appliedOpt.name));

if (reportTab) reportTab.click(); // leave the editor on a panel the rest reads

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
