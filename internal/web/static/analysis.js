/* The analysis panel: a review of the resume written by a small language model
   that runs in the browser itself, through WebLLM and WebGPU — and the things
   the same model can do with the resume afterwards: rewrite its content,
   translate it into the other language the tool supports, and rehearse an
   interview on it. The rewrites open a new version of the CV; the review and
   the interview write their text below the buttons.

   Three properties shape this file. The engine is 6.6 Mo of JavaScript and the
   model is 1.4 Go of weights, so both are fetched the first time a button is
   pressed and never on page load: a dynamic import for the first, WebLLM's own
   cache for the second. The resume is personal data, so the inference happens
   here, in the page — the only outbound requests are the model's own files,
   which is what the Hugging Face sources in the Content-Security-Policy are
   for; the resume itself is never sent anywhere — unless the reader asks for
   it: an OpenAI-compatible provider typed into the panel (base URL, key,
   model) is then called directly, and the text only goes there. And nothing
   the model writes is trusted: its answer reaches textContent and never
   innerHTML, and its JSON reaches the editor only after POST /api/parse has
   accepted and normalized it — the same gate an import goes through. Whatever
   passes opens a new session, so the draft it was written from stays beside
   it, intact.

   Every state the reader waits through has a visible sign: a spinner, a
   measured download bar, a breathing status, a caret at the end of the
   answer. All of it in app.css: the policy forbids inline style, so nothing
   here assigns to element.style. */

(function () {
  'use strict';

  // The editor language, when app.js is on the page with its translation
  // table; French when this file is loaded on its own, as in the tests.
  const label = (fr, en) => (typeof T !== 'undefined' ? T.label(fr, en) : fr);

  const MODEL_ID = 'MiniCPM5-2B-q4f16_1-MLC';
  const REPO = 'https://huggingface.co/vs4vijay/MiniCPM5-2B-MLC/resolve/main';

  // The record WebLLM needs to load MiniCPM5-2B: the weights sit in a
  // subfolder of the repository in the standard WebLLM layout, the kernels are
  // the wasm file beside them, and the 32768-token window the model was
  // compiled for can be shrunk at load time — 8k is far more than a resume
  // and its rewrites need, and the smaller window keeps the KV cache small
  // enough for an ordinary laptop GPU.
  const model = {
    model: REPO + '/q4f16_1',
    model_id: MODEL_ID,
    model_lib: REPO + '/q4f16_1/libs/MiniCPM5-2B-q4f16_1-MLC-webgpu.wasm',
    vram_required_MB: 2000,
    overrides: { context_window_size: 8192 },
  };

  const run = document.getElementById('ia-run');
  const optimize = document.getElementById('ia-optimize');
  const translate = document.getElementById('ia-translate');
  // The interview rehearsal: questions drawn from what the CV actually says,
  // and from the saved offer when there is one.
  const sim = document.getElementById('ia-simulate');
  // The offer of the job the draft is aimed at: one input, one fetch button,
  // one line that keeps naming the offer the analyses will read beside the
  // resume, and the button that unties them.
  const jobRun = document.getElementById('ia-job');
  const jobUrl = document.getElementById('ia-url');
  const offerLine = document.getElementById('ia-offer');
  const offerClear = document.getElementById('ia-offer-clear');
  // The provider the reader owns: base URL, model, key. Typed once, kept in
  // this browser like the offer, used by every run until it is retired.
  const provUrl = document.getElementById('ia-provider-url');
  const provModel = document.getElementById('ia-provider-model');
  const provKey = document.getElementById('ia-provider-key');
  const provSave = document.getElementById('ia-provider-save');
  const provClear = document.getElementById('ia-provider-clear');
  // The provider card is a native <details>: its summary carries the state
  // (which endpoint the panel calls), and the card keeps itself open exactly
  // while a provider is active — nothing to unfold to see where the CV goes.
  const providerCard = document.getElementById('ia-provider-card');
  const providerState = document.getElementById('ia-provider-state');
  const note = document.getElementById('ia-note');
  const statusNode = document.getElementById('ia-status');
  const out = document.getElementById('ia-out');
  const dataField = document.getElementById('data');
  const spinner = document.getElementById('ia-spinner');
  const progress = document.getElementById('ia-progress');
  const bar = document.getElementById('ia-bar');
  const pct = document.getElementById('ia-pct');
  // The four actions. The optional ones may be commented out of the page —
  // and the panel must survive that: a missing optional button removes a
  // feature, it must not leave the others dead.
  // The elements every listener needs are the required ones, never optional.
  // The provider block is not among them: a page without it just loses the
  // custom provider and keeps the local model, the way it loses a rewrite
  // without its button.
  const buttons = [run, optimize, translate, sim, jobRun].filter(Boolean);
  if (!run || !statusNode || !out || !dataField ||
      !spinner || !progress || !bar || !pct ||
      !jobRun || !jobUrl || !offerLine || !offerClear) return;

  /* ------------------------------------------------------------------ */
  /* The custom provider. An OpenAI-compatible endpoint the reader names: */
  /* base URL, model, key. Stored behind the drafts, used by every run    */
  /* until it is retired — the local engine is the default, the provider  */
  /* is the exception the reader asked for, and the resume only ever      */
  /* travels where its owner typed a URL.                                 */
  /* ------------------------------------------------------------------ */
  const LLM_KEY = 'atscv.llm.v1';
  let llm = null; // { baseUrl, apiKey, model } while a provider is active

  const loadLLM = () => {
    try {
      const raw = localStorage.getItem(LLM_KEY);
      if (!raw) return null;
      const cfg = JSON.parse(raw);
      if (!cfg || !/^https?:\/\//i.test(String(cfg.baseUrl || '')) || !String(cfg.model || '').trim()) {
        return null; // a broken record is a record without a provider
      }
      return { baseUrl: String(cfg.baseUrl), apiKey: String(cfg.apiKey || ''), model: String(cfg.model) };
    } catch (e) {
      return null;
    }
  };

  const defaultNote = note ? note.textContent : '';

  const renderProvider = () => {
    llm = loadLLM();
    if (providerState) {
      providerState.textContent = llm
        ? label('actif : ' + llm.model + ' — modifier', 'active: ' + llm.model + ' — edit')
        : label('aucun — le modèle local est utilisé', 'none — the local model is used');
    }
    if (providerCard) providerCard.open = Boolean(llm);
    if (!note) return;
    if (llm) {
      const host = String(llm.baseUrl).replace(/^https?:\/\//, '').split('/')[0];
      note.textContent = label(
        'Analyse par votre fournisseur « ' + llm.model + ' » sur ' + host +
        ' : le CV lui est envoyé. Les trois champs sont gardés dans ce navigateur.',
        'Analysis by your provider “' + llm.model + '” on ' + host +
        ': the CV is sent to it. The three fields are kept in this browser.'
      );
      if (provUrl) provUrl.value = llm.baseUrl;
      if (provModel) provModel.value = llm.model;
      if (provKey) provKey.value = llm.apiKey;
      if (provClear) provClear.hidden = false;
    } else {
      note.textContent = defaultNote;
      if (provClear) provClear.hidden = true;
    }
  };

  const saveProvider = () => {
    const baseUrl = (provUrl.value || '').trim().replace(/\/+$/, '');
    const model = (provModel.value || '').trim();
    const apiKey = (provKey.value || '').trim();
    if (!/^https?:\/\//i.test(baseUrl)) {
      return status(label(
        "L'URL du fournisseur doit commencer par http:// ou https://.",
        'The provider URL must start with http:// or https://.'
      ), 'error');
    }
    if (!model) {
      return status(label(
        'Indiquez le nom du modèle à interroger.',
        'Name the model to query.'
      ), 'error');
    }
    localStorage.setItem(LLM_KEY, JSON.stringify({ baseUrl, model, apiKey }));
    renderProvider();
    status(label('Fournisseur enregistré : ', 'Provider saved: ') + model);
  };

  const clearProvider = () => {
    localStorage.removeItem(LLM_KEY);
    renderProvider();
    status(label('Retour au modèle local.', 'Back to the local model.'));
  };

  /* providerChat calls the endpoint the OpenAI clients call: the model, the
     same conversation shape, the same sampling parameters the local engine
     gets. It answers the streaming response, or an error that names the
     status and the message the provider wrote in its body. */
  const providerChat = async (messages, params) => {
    status(label('Appel de ' + llm.model + '…', 'Calling ' + llm.model + '…'), 'waiting');
    const headers = { 'Content-Type': 'application/json' };
    if (llm.apiKey) headers.Authorization = 'Bearer ' + llm.apiKey;
    const url = llm.baseUrl + '/chat/completions';
    let res;
    try {
      res = await fetch(url, {
        method: 'POST',
        headers,
        body: JSON.stringify({ model: llm.model, messages, stream: true, ...params }),
      });
    } catch (err) {
      // Un fetch qui jette meurt avant la moindre réponse : itinéraire bloqué
      // par CORS — l'appel OPTIONS que le navigateur envoie avant le POST,
      // car l'en-tête Authorization force la pré-demande — ne trouve rien
      // pour le servir, ou l'origine ne figure pas dans
      // Access-Control-Allow-Origin. curl n'en fait rien de tout cela : une
      // ligne de commande réussit là où la page ne passe pas, et la console
      // du navigateur nomme le motif exact.
      throw new Error(
        label(
          "Le navigateur n'a pas atteint " + url + " — là où curl passe. Votre serveur doit répondre à la pré-demande OPTIONS (CORS) et envoyer Access-Control-Allow-Origin avant de relayer le POST d'une page ; la console du navigateur nomme le motif exact.",
          'The browser could not reach ' + url + ' — where curl succeeds. Your server must answer the OPTIONS preflight (CORS) and send Access-Control-Allow-Origin before it relays a POST from a page; the browser console names the exact reason.'
        )
      );
    }
    if (!res.ok) {
      let detail = '';
      try {
        const body = await res.json();
        detail = (body && body.error && body.error.message) || (body && body.message) || '';
      } catch (e) { /* some providers answer an error in plain text */ }
      // Un 404 sans /v1 en bout de base est presque toujours une base à
      // laquelle il manque le segment que curl avait : le conseil le porte.
      const hint = res.status === 404 && !/\/v1\/?$/i.test(llm.baseUrl)
        ? label(
          " — essayez une base qui finit par /v1 : " + llm.baseUrl + '/v1',
          " — try a base ending in /v1: " + llm.baseUrl + '/v1'
        )
        : '';
      throw new Error(label('HTTP ' + res.status, 'HTTP ' + res.status) +
        (detail ? ' — ' + detail : '') + hint);
    }
    if (!res.body || !res.body.getReader) {
      throw new Error(label(
        'Le fournisseur ne diffuse pas de flux : essayez un autre point de terminaison.',
        'The provider does not stream: try another endpoint.'
      ));
    }
    return res;
  };

  /* The grammar of a resume, published by the server beside the editor
     schema. /api/schema answers the same document shape the parser enforces,
     as a JSON Schema — the one constraint the rewrites are given: WebLLM
     compiles it into the decoding, an OpenAI-compatible provider receives it
     as a json_schema response format, and a rewrite comes back a valid
     document by construction instead of by luck. Fetched on the first
     rewrite, not on load, for the same reason the engine is. */
  const grammar = { schema: null, asked: false };

  const loadGrammar = async () => {
    if (grammar.asked) return grammar.schema;
    grammar.asked = true;
    try {
      const res = await fetch('/api/schema');
      if (res.ok) {
        const published = await res.json();
        grammar.schema = (published && published.resumeSchema) || null;
      }
    } catch (e) {
      // Without the schema the rewrites stay free-text: the validation gate
      // below is what keeps a malformed answer out of the editor.
    }
    return grammar.schema;
  };

  /* responseFormat shapes the constraint for the door it will cross: the
     OpenAI spelling for a provider, the local engine's json_object for
     WebLLM. An endpoint that refuses the format alone must not kill the
     rewrite: the loop below drops the constraint once and asks again. */
  const responseFormat = (schema) => {
    if (!schema) return undefined;
    if (llm) {
      return { type: 'json_schema', json_schema: { name: 'resume', strict: true, schema } };
    }
    return { type: 'json_object', schema };
  };

  /* The engine survives between two runs: reloading it would re-read the
     cached weights, and the first load is the expensive one. */
  let engine = null;
  let busy = false;
  // The stop reason of the last stream: the only thing that tells a cut
  // answer from a written one, whichever door the writing came through.
  let streamStop = '';
  // The answer of the last analysis, kept so « Appliquer les améliorations »
  // applies what was proposed rather than rewriting the CV in the dark.
  let lastReview = '';

  const setBusy = (on) => {
    busy = on;
    for (const button of buttons) button.disabled = on;
  };

  /* status writes the one line the reader watches. "waiting" breathes while
     something is on its way, "error" paints the failure red — both classes
     live in app.css, and both are cleared by the next status(''). */
  const status = (text, mood) => {
    statusNode.textContent = text || '';
    statusNode.classList.toggle('is-error', mood === 'error');
    statusNode.classList.toggle('is-waiting', mood === 'waiting');
  };
  const spin = (on) => { spinner.hidden = !on; };
  const download = (fraction) => {
    const value = Math.max(0, Math.min(100, Math.round((fraction || 0) * 100)));
    bar.value = value;
    pct.textContent = value + ' %';
    progress.hidden = false;
  };
  const downloadOff = () => { progress.hidden = true; };
  const showOut = (text) => {
    out.hidden = false;
    out.textContent = text || '';
    out.classList.add('is-streaming');
  };
  const outDone = () => out.classList.remove('is-streaming');

  const hasWebGPU = () => typeof navigator !== 'undefined' && Boolean(navigator.gpu);

  /* What the buttons say the translation will do, decided from the language
     the draft declares: the other language is the only target the tool has.
     The label is set when the panel opens, which is the moment before any
     click — the draft may not be in the field yet when this file loads. */
  const updateTranslateLabel = () => {
    if (!translate) return;
    let lang = '';
    try {
      lang = dataField.value ? (JSON.parse(dataField.value).lang || '') : '';
    } catch (e) { /* a draft mid-edit is still the one to translate */ }
    translate.textContent = String(lang) === 'en'
      ? label('Traduire en français', 'Translate to French')
      : label('Traduire en anglais', 'Translate to English');
  };
  for (const tab of document.querySelectorAll('.tab')) {
    if (tab.dataset.tab === 'ia') tab.addEventListener('click', updateTranslateLabel);
  }
  updateTranslateLabel();

  /* readCV hands the draft to the server the way a parser reads it — the
     same order of reading the plain-text panel shows — or explains why it
     cannot. Answers the text, or null with the status line set. */
  const readCV = async () => {
    spin(true);
    status(label('Lecture du CV…', 'Reading the CV…'), 'waiting');
    const res = await fetch('/api/text?format=json', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: dataField.value,
    });
    if (!res.ok) {
      const body = await res.json().catch(() => ({}));
      status(body.error || draftError(), 'error');
      return null;
    }
    const text = await res.text();
    if (!text.trim()) {
      status(label('Le CV est vide.', 'The CV is empty.'), 'error');
      return null;
    }
    return text;
  };

  /* streamAnswer runs one plain-text turn through startChat and streams it
     into the panel; the two free-text actions — the review and the interview
     simulation — share it. Answers the text, or '' when the model returned
     nothing (the status then still says so, set by the caller). */
  const streamAnswer = async (messages, params, waitLabel) => {
    spin(true);
    status(waitLabel, 'waiting');
    showOut('');
    const started = Date.now();
    const stream = await startChat(messages, params);
    let answer = '';
    for await (const choice of stream) {
      const piece = choice && choice.delta && choice.delta.content;
      if (piece) {
        answer += piece;
        out.textContent = answer;
      }
    }
    outDone();
    if (!answer.trim()) {
      out.hidden = true;
      return '';
    }
    const seconds = Math.max(1, Math.round((Date.now() - started) / 1000));
    status(label('Réponse en ', 'Answer in ') + seconds + ' s');
    return answer;
  };

  /* interviewMessages builds the rehearsal: the questions a recruiter of the
     practicable offer would ask, all drawn from what the CV says — nothing
     invented — plus what each answer must prove. With an offer stored, the
     simulation targets it; without one, it stays a general interview. */
  const interviewMessages = (text, offer) => {
    let intro =
      'Voici le CV, tel qu\'un parseur le lit :\n\n' + text + '\n\n';
    let scope =
      '- 6 questions au total : 2 sur le parcours, 2 techniques directement ' +
      'tirées du CV, 2 mises en situation.\n';
    if (offer && offer.text) {
      intro +=
        'Le CV vise l\'offre ci-dessous' +
        (offer.title ? ' (« ' + offer.title + ' »)' : '') +
        ' :\n\n' + String(offer.text).slice(0, 8000) + '\n\n';
      scope =
        '- 6 questions au total : 2 sur le parcours, 2 techniques tirées du ' +
        'CV au regard de l\'offre, 2 mises en situation propres au poste.\n';
    }
    return [
      {
        role: 'system',
        content:
          'Tu es un recruteur senior qui mène un entretien réaliste et exigeant, ' +
          'orienté ATS et poste visé. Tu poses des questions précises, bref, ' +
          'et tu expliques ce que chaque réponse doit montrer.',
      },
      {
        role: 'user',
        content:
          intro +
          'Génère une simulation d\'entretien pour ce CV, dans la langue du CV :\n' +
          scope +
          '- Numérote chaque question, et fais-la suivre d\'une ligne « ce que la réponse doit montrer ».\n' +
          '- Base-toi uniquement sur ce que le CV dit : ni fait, ni chiffre inventé.\n' +
          '250 mots au maximum.',
      },
    ];
  };

  /* The prompts. The review reads what a parser reads; the two rewrites get
     the JSON itself, because JSON in, JSON out is the only exchange where the
     structure can be checked on the way back in. When an offer is stored, the
     review becomes a comparison: the offer text rides along, and the answer
     gains the rubric that names what the offer asks and the CV does not say. */
  const reviewMessages = (text, offer) => {
    let ask =
      'Propose des améliorations concrètes, dans la langue du CV (français par défaut), ' +
      'avec exactement ces trois rubriques, séparées par une ligne vide :\n';
    let intro =
      'Voici le CV, tel qu\'un parseur le lit :\n\n' + text + '\n\n';
    let rubrics =
      '1. Points forts — ce qui fonctionne déjà, en 3 points maximum.\n' +
      '2. À améliorer — un problème par ligne, chacun suivi de la raison.\n' +
      '3. Suggestions — des réécritures prêtes à coller, verbe d\'action et chiffres en tête de phrase.\n';
    if (offer && offer.text) {
      intro +=
        'Le CV vise l\'offre ci-dessous' +
        (offer.title ? ' (« ' + offer.title + ' »)' : '') +
        ' :\n\n' + String(offer.text).slice(0, 8000) + '\n\n';
      ask =
        'Compare le CV à cette offre : propose des améliorations concrètes, dans la langue du CV ' +
        '(français par défaut), avec exactement ces quatre rubriques, séparées par une ligne vide :\n';
      rubrics +=
        '4. Alignement avec l\'offre — les compétences et mots-clés que l\'offre demande et que le CV ' +
        'ne dit pas, et ce que le CV avance que l\'offre ne demande pas.\n';
    }
    return [
      {
        role: 'system',
        content:
          "Tu es un recruteur senior et un expert de l'optimisation des CV pour les logiciels de tri ATS. " +
          'Tu analyses des CV avec rigueur et tu écris des conseils applicables immédiatement.',
      },
      {
        role: 'user',
        content:
          intro + ask + rubrics +
          "Ne rapporte aucune expérience ni aucun chiffre qui n'est pas dans le CV. " +
          'Quatre cents mots au maximum.',
      },
    ];
  };

  /* The translation rules spell every field of the document by name, each in
     one of three lists: what gets translated, what gets translated as a
     proper noun in context, and what a translation leaves rigorously
     identical — plus the one rule for the lang field, which is switched. The
     lists and the schema the server publishes are kept in step by the tests:
     a field added to the model and forgotten here would come back in the
     source language — a defect only a reader of the translated CV would
     see. */
  const rewriteMessages = (resume, mode, target, suggestions) => {
    const json = JSON.stringify(resume);
    let rules;
    if (mode === 'translate') {
      rules =
        'Traduis ce CV en ' + (target === 'en' ? 'anglais' : 'français') + '.\n' +
        '- Traduis exactement ces champs texte, et seulement eux :\n' +
        '`headline`, `summary`, `contact.note`, `contact.links[].label`, ' +
        '`experience[].title`, `experience[].summary`, `experience[].highlights[]`, ' +
        '`experience[].team`, `education[].degree`, `education[].summary`, ' +
        '`education[].coursework[]`, `certifications[].name`, `projects[].summary`, ' +
        '`projects[].highlights[]`, `activities[].summary`, ' +
        '`activities[].highlights[]`, `skills[].category`, `skills[].items[]`, ' +
        '`languages[].name`, `languages[].level`.\n' +
        '- Traduis aussi ces énoncés de noms propres, sans en retaper ' +
        'l\'orthographe :\n' +
        '`experience[].company`, `education[].school`, `certifications[].issuer`, ' +
        '`projects[].name`, `activities[].organization`, `activities[].name`.\n' +
        '- Mets `lang` à "' + target + '".\n' +
        '- Laisse rigoureusement identiques tous les autres champs — le nom de la ' +
        'personne, les coordonnées, les lieux, les dates, les URLs et les technos :\n' +
        '`name`, `contact.city`, `contact.region`, `contact.country`, ' +
        '`contact.email`, `contact.phone`, `contact.links[].url`, ' +
        '`experience[].location`, `experience[].start`, `experience[].end`, ' +
        '`experience[].stack[]`, `education[].location`, `education[].start`, ' +
        '`education[].end`, `certifications[].date`, `certifications[].id`, ' +
        '`projects[].url`, `projects[].start`, `projects[].end`, ' +
        '`projects[].technologies[]`, `activities[].location`, ' +
        '`activities[].start`, `activities[].end`.\n' +
        '- Traduis fidèlement : n\'invente ni ne supprime aucun fait.';
    } else if (suggestions) {
      rules =
        'Améliore la formulation de ce CV en appliquant d\'abord les ' +
        'suggestions de l\'analyse ci-dessous, quand elles restent fidèles ' +
        'aux faits ; ignore une suggestion qui inventerait un fait, un ' +
        'chiffre ou une date.\n' +
        '- Réécris accroche, résumé et points des entrées : verbe d\'action en tête de phrase, ' +
        'phrases courtes, chiffres conservés tels quels.\n' +
        "- N'invente aucun fait, chiffre, poste, entreprise, date ni compétence : " +
        "c'est la formulation qui change, pas le contenu.\n" +
        '- Ne change ni les clés, ni les dates, ni le champ lang, ni le contact, ni le nom ; ' +
        'les champs vides restent vides ; la langue du CV est conservée.\n\n' +
        'Suggestions de l\'analyse précédente :\n\n' +
        String(suggestions).slice(0, 6000);
    } else {
      rules =
        'Améliore la formulation de ce CV.\n' +
        '- Réécris accroche, résumé et points des entrées : verbe d\'action en tête de phrase, ' +
        'phrases courtes, chiffres conservés tels quels.\n' +
        "- N'invente aucun fait, chiffre, poste, entreprise, date ni compétence : " +
        "c'est la formulation qui change, pas le contenu.\n" +
        '- Ne change ni les clés, ni les dates, ni le champ lang, ni le contact, ni le nom ; ' +
        'les champs vides restent vides ; la langue du CV est conservée.';
    }
    return [
      {
        role: 'system',
        content:
          'Tu es un recruteur senior et un expert ATS. Tu réécris des CV au format JSON : ' +
          'même structure, mêmes clés, faits intacts, formulation percutante.',
      },
      {
        role: 'user',
        content:
          rules + '\n\n' +
          'Renvoie UNIQUEMENT le JSON complet, sans commentaire ni bloc de code.\n\n' +
          json,
      },
    ];
  };

  /* extractJSON digs the object out of the answer: a model that wraps it in a
     code fence or adds a sentence around it is common, and the parse below
     would fail on the wrapper, not on the JSON. */
  const extractJSON = (raw) => {
    const s = String(raw).trim()
      .replace(/^```(?:json)?\s*/i, '')
      .replace(/\s*```$/, '');
    const start = s.indexOf('{');
    const end = s.lastIndexOf('}');
    if (start < 0 || end <= start) {
      throw new Error(label('Pas de JSON dans la réponse du modèle.', 'No JSON in the model answer.'));
    }
    return s.slice(start, end + 1);
  };

  /* ensureEngine loads the library and the model on first use, with the
     download bar taking over from the spinner once the weights move. */
  const ensureEngine = async () => {
    if (engine) return engine;
    status(label('Chargement du moteur…', 'Loading the engine…'), 'waiting');
    const webllm = await import('/static/vendor/webllm.js');
    engine = await webllm.CreateMLCEngine(MODEL_ID, {
      appConfig: { model_list: [model] },
      initProgressCallback: (report) => {
        // The measured phase gets the measured indicator: the bar takes
        // over from the spinner for the download, and comes with the
        // prediction the engine makes of what is left of it.
        spin(false);
        download(report.progress);
        const eta = Number.isFinite(report.timePrediction) && report.timePrediction > 0
          ? ' · ≈ ' + Math.round(report.timePrediction) + ' s'
          : '';
        status(label('Téléchargement du modèle — ', 'Downloading the model — ') +
          Math.round((report.progress || 0) * 100) + ' %' + eta, 'waiting');
      },
    });
    downloadOff();
    return engine;
  };

  /* sseChoices reads the stream the OpenAI servers write: "data:" lines, the
     choices they carry, a [DONE] line at the end. Every OpenAI-compatible
     provider has agreed on that shape, so one reader covers them all — and
     it yields the choice itself, stop reason included, the same shape the
     local engine reports its chunks in. */
  const sseChoices = async function* (res) {
    const reader = res.body.getReader();
    const decoder = new TextDecoder();
    let buffer = '';
    while (true) {
      const { value, done } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });
      let i;
      while ((i = buffer.indexOf('\n')) >= 0) {
        const line = buffer.slice(0, i).trim();
        buffer = buffer.slice(i + 1);
        if (!line.startsWith('data:')) continue;
        const payload = line.slice(5).trim();
        if (payload === '[DONE]')  return;
        try {
          const json = JSON.parse(payload);
          const choice = json && json.choices && json.choices[0];
          if (choice && choice.finish_reason) streamStop = choice.finish_reason;
          if (choice) yield choice;
        } catch (e) { /* a partial line or a provider note: content is what matters */ }
      }
    }
  };

  /* startChat is the one door every action walks through: the configured
     provider when there is one, the local engine when there is not. It
     answers an iterable of the same choices either way, so the streaming
     below is written once. */
  /* localChoices wraps the local engine's stream the same way sseChoices
     wraps a provider's: chunks in, choices out, the stop reason recorded —
     so both doors answer one shape and the loop below is written once. */
  const localChoices = async function* (chunks) {
    for await (const chunk of chunks) {
      const choice = chunk && chunk.choices && chunk.choices[0];
      if (choice) {
        if (choice.finish_reason) streamStop = choice.finish_reason;
        yield choice;
      }
    }
  };

  const startChat = async (messages, params) => {
    if (llm) {
      const res = await providerChat(messages, params);
      return sseChoices(res);
    }
    const eng = await ensureEngine();
    return localChoices(eng.chat.completions.create({ messages, stream: true, ...params }));
  };

  /* rewrite is both actions: the draft is validated first (the model must be
     given a sound baseline), the model returns the whole JSON — constrained
     by the grammar the server publishes when it can — and the result goes
     back through validation, and through the untouched comparison for a
     translation, before a session opens.

     app.js owns all of it — this file only calls. */
  /* showEdit presses the view tab app.js owns: after a rewrite is applied,
     the reader goes back to the form, where the new version now lives. */
  const showEdit = () => {
    const tab = document.querySelector('.viewtab[data-view="edit"]');
    if (tab) tab.click();
  };

  /* applySession hands a validated resume to the editor the way an import
     does: a new session, then every step that decides what the next render
     shows. The session it was written from stays beside it, intact. */
  const applySession = (resume, name) => {
    newSession(resume, name);
    persist();
    resetSection();
    rerender();
    sync();
    refreshOutput();
    showEdit();
    out.hidden = true;
    status(label('Nouvelle session : ', 'New session: ') + name);
  };

  /* validate posts a candidate JSON to the same endpoint an import uses. The
     server answers the normalized resume, or the first defect — never a
     silent drop. */
  const validate = async (candidate) => {
    const res = await fetch('/api/parse?format=json', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: candidate,
    });
    if (!res.ok) {
      const body = await res.json().catch(() => ({}));
      return { ok: false, error: body.error || '' };
    }
    return { ok: true, resume: await res.json() };
  };

  /* The stored offer. It lives in this browser's own storage, beside the
     draft — fetched once by the server, then read from here by every run of
     the review. No session, no account, no server-side copy: Remove the
     stored offer and nothing but the fetch's own memory ever held it. */
  const OFFER_KEY = 'atscv.offer.v1';

  const loadOffer = () => {
    try {
      const raw = localStorage.getItem(OFFER_KEY);
      if (!raw) return null;
      const offer = JSON.parse(raw);
      if (!offer || typeof offer.text !== 'string' || !offer.text.trim()) return null;
      return offer;
    } catch (e) {
      return null;
    }
  };

  /* renderOffer names what every next analysis will compare the CV against:
     the title of the offer, the host it came from, the size of what was
     kept. A reader must always be able to see which offer the answer was
     aimed at — or that none is stored. */
  const renderOffer = () => {
    const offer = loadOffer();
    if (!offer) {
      offerLine.hidden = true;
      offerClear.hidden = true;
      return;
    }
    const host = offer.url ? String(offer.url).replace(/^https?:\/\//, '').split('/')[0] : '';
    offerLine.textContent = label('Offre enregistrée : ', 'Saved offer: ') +
      (offer.title || host || label('page sans titre', 'untitled page')) +
      (host ? ' — ' + host : '') +
      ' (' + offer.text.length + ' ' + label('caractères', 'characters') + ')';
    offerLine.hidden = false;
    offerClear.hidden = false;
  };

  /* fetchOfferJob asks the server for the page of the offer. The URL is the
     only thing that leaves this page; the answer, the readable text of the
     page, is stored locally and named above the panel. */
  const fetchOfferJob = async () => {
    setBusy(true);
    out.hidden = true;
    spin(false);
    downloadOff();
    try {
      const url = jobUrl.value.trim();
      if (!/^https?:\/\//i.test(url)) {
        return status(label(
          'Entrez une URL qui commence par http:// ou https://.',
          'Enter a URL that starts with http:// or https://.'
        ), 'error');
      }
      spin(true);
      status(label('Récupération de la page de l\'offre…', 'Fetching the offer page…'), 'waiting');
      const res = await fetch('/api/job', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ url }),
      });
      const body = await res.json().catch(() => ({}));
      if (!res.ok) {
        return status(body.error || label(
          'La page n\'a pas pu être récupérée.',
          'The page could not be fetched.'
        ), 'error');
      }
      if (!body.text || !String(body.text).trim()) {
        return status(label(
          'La page ne contient pas de texte lisible : essayez une autre URL.',
          'The page carries no readable text: try another URL.'
        ), 'error');
      }
      localStorage.setItem(OFFER_KEY, JSON.stringify(body));
      renderOffer();
      status(label('Offre enregistrée : ', 'Offer saved: ') +
        (body.title || label('page sans titre', 'untitled page')));
    } catch (err) {
      status(label('Erreur : ', 'Error: ') + ((err && err.message) || String(err)), 'error');
    } finally {
      spin(false);
      downloadOff();
      setBusy(false);
    }
  };

  const refusal = () => status(label(
    'WebGPU indisponible : ce navigateur ne peut pas exécuter l\'analyse locale.',
    'WebGPU is unavailable: this browser cannot run the local analysis.'
  ), 'error');

  const draftError = () => label(
    'Le CV contient des erreurs : corrigez-les avant l\'analyse.',
    'The CV has errors: fix them before the analysis.'
  );

  /* review is the original action: read what a parser reads, ask for advice,
     stream the advice. */
  const review = async () => {
    setBusy(true);
    out.hidden = true;
    out.textContent = '';
    outDone();
    spin(false);
    downloadOff();
    status('');
    // Which step failed, so the catch knows whether the engine worth keeping
    // is still worth keeping: a broken draft never touched it, a lost device
    // did.
    let phase = 'text';
    try {
      // WebGPU is the local engine's requirement, not the provider's: a
      // provider configured is a page that works without it.
      if (!llm && !hasWebGPU()) return refusal();

      const text = await readCV();
      if (!text) return;

      phase = 'chat';
      const answer = await streamAnswer(reviewMessages(text, loadOffer()), {
        temperature: 0.7,
        top_p: 0.95,
        max_tokens: 700,
      }, label('Analyse en cours…', 'Analysing…'));
      if (!answer) {
        return status(label(
          'Le modèle n\'a rien renvoyé : réessayez.',
          'The model returned nothing: try again.'
        ), 'error');
      }
      // Kept: « Appliquer les améliorations » will apply exactly what was
      // proposed here.
      lastReview = answer;
    } catch (err) {
      if (phase === 'chat') engine = null;
      out.hidden = true;
      outDone();
      status(label('Erreur : ', 'Error: ') + ((err && err.message) || String(err)), 'error');
    } finally {
      spin(false);
      downloadOff();
      setBusy(false);
    }
  };

  /* runSimulation writes the interview rehearsal: the same reading and gates
     as the review, its own prompt — and nothing ever enters the editor. It
     is a rehearsal, written for the reader to answer out loud. */
  const runSimulation = async () => {
    setBusy(true);
    out.hidden = true;
    out.textContent = '';
    outDone();
    spin(false);
    downloadOff();
    status('');
    let phase = 'text';
    try {
      if (!llm && !hasWebGPU()) return refusal();
      const text = await readCV();
      if (!text) return;
      phase = 'chat';
      const answer = await streamAnswer(interviewMessages(text, loadOffer()), {
        temperature: 0.8,
        top_p: 0.95,
        max_tokens: 700,
      }, label('Simulation de l\'entretien…', 'Simulating the interview…'));
      if (!answer) {
        return status(label(
          'Le modèle n\'a rien renvoyé : réessayez.',
          'The model returned nothing: try again.'
        ), 'error');
      }
    } catch (err) {
      if (phase === 'chat') engine = null;
      out.hidden = true;
      outDone();
      status(label('Erreur : ', 'Error: ') + ((err && err.message) || String(err)), 'error');
    } finally {
      spin(false);
      downloadOff();
      setBusy(false);
    }
  };

  /* untouched compares the document a translation came from with the one it
     came back as, and names every field the rules pinned that the model
     moved: the lang it forgot to switch, the name it renamed, the contact
     details it retyped, the URLs it "cleaned", the dates it translated, the
     entries it dropped. An empty list is a translation that kept its
     promises. */
  const untouched = (before, after, target) => {
    const damaged = [];
    const eq = (x, y) => String(x || '') === String(y || '');
    if (!eq(after.lang, target)) damaged.push('lang', target, after.lang);
    if (!eq(after.name, before.name)) damaged.push('name', before.name, after.name);
    if (!eq(after.contact && after.contact.email, before.contact && before.contact.email)) {
      damaged.push('contact.email', before.contact.email, after.contact.email);
    }
    if (!eq(after.contact && after.contact.phone, before.contact && before.contact.phone)) {
      damaged.push('contact.phone', before.contact.phone, after.contact.phone);
    }
    // The URLs of the links and the dates of the certifications: a retyped
    // URL is a dead link, a translated date a document no ATS can sort.
    const beforeLinks = (before.contact && before.contact.links) || [];
    const afterLinks = (after.contact && after.contact.links) || [];
    if (beforeLinks.length !== afterLinks.length) {
      damaged.push('contact.links', beforeLinks.length, afterLinks.length);
    }
    for (let i = 0; i < beforeLinks.length && i < afterLinks.length; i += 1) {
      if (!eq(afterLinks[i] && afterLinks[i].url, beforeLinks[i] && beforeLinks[i].url)) {
        damaged.push('contact.links[].url',
          (beforeLinks[i] && beforeLinks[i].url) || '',
          (afterLinks[i] && afterLinks[i].url) || '');
      }
    }
    if (before.certifications && after.certifications) {
      for (let i = 0; i < before.certifications.length && i < after.certifications.length; i += 1) {
        if (!eq((after.certifications[i] || {}).date, (before.certifications[i] || {}).date)) {
          damaged.push('certifications[].date',
            (before.certifications[i] || {}).date,
            (after.certifications[i] || {}).date);
        }
      }
    }
    // Every other date of the document, and the size of every section: the
    // counters are what a dropped entry is.
    const sections = ['experience', 'education', 'certifications', 'projects', 'activities', 'skills', 'languages'];
    for (const section of sections) {
      const from = before[section] || [];
      const to = after[section] || [];
      if (from.length !== to.length) {
        damaged.push(section, from.length, to.length);
        continue;
      }
      for (let i = 0; i < from.length; i += 1) {
        for (const field of ['start', 'end']) {
          if (from[i] && from[i][field] !== undefined && !eq(to[i] && to[i][field], from[i][field])) {
            damaged.push(section + '.' + field, from[i][field], to[i] && to[i][field]);
          }
        }
      }
    }
    return damaged;
  };

  /* rewrite is both actions: the draft is validated first (the model must be
     given a sound baseline), the model returns the whole JSON — constrained
     by the grammar the server publishes when it can — and the result goes
     back through validation, and through the untouched comparison for a
     translation, before a session opens. Three gates — nothing enters the
     editor that the server has not accepted. One second chance: whatever
     gate refuses, its reason is handed back to the model as a further
     message — once, and never in a loop. */
  const rewrite = async (mode) => {
    setBusy(true);
    out.hidden = true;
    out.textContent = '';
    outDone();
    spin(false);
    downloadOff();
    status('');
    let phase = 'validate';
    try {
      // Same choice as the review: the provider speaks whatever HTTP the
      // browser speaks, the local engine is the one that needs WebGPU.
      if (!llm && !hasWebGPU()) return refusal();

      spin(true);
      status(label('Lecture du CV…', 'Reading the CV…'), 'waiting');
      const parsed = await validate(dataField.value);
      if (!parsed.ok) return status(parsed.error || draftError(), 'error');
      const base = parsed.resume;
      const target = String(base.lang) === 'en' ? 'fr' : 'en';

      // The grammar the server publishes, when the panel could read it: a
      // constrained rewrite comes back a document by construction. An
      // endpoint that refuses the response format alone drops the
      // constraint once — never the rewrite.
      const schema = await loadGrammar();
      // The guidance of « Appliquer les améliorations » : what the last
      // analysis proposed, so the rewrite applies it rather than rewriting
      // the CV in the dark. No analysis ran yet — the rewrite simply rewords.
      const suggestions = mode === 'optimize' && lastReview.trim() ? lastReview.trim() : '';

      const messages = rewriteMessages(base, mode, target, suggestions);
      let attempt = 1;
      let useGrammar = true;
      for (;;) {
        phase = 'chat';
        streamStop = '';
        spin(true);
        status((mode === 'translate'
          ? label('Traduction en ' + (target === 'en' ? 'anglais' : 'français') + '…',
            'Translating into ' + (target === 'en' ? 'English' : 'French') + '…')
          : label('Réécriture du CV…', 'Rewriting the CV…')) +
          (attempt > 1 ? label(' — nouvelle tentative…', ' — second attempt…') : ''), 'waiting');
        showOut('');
        const params = { temperature: 0.3, top_p: 0.95, max_tokens: 2500 };
        if (useGrammar && schema) params.response_format = responseFormat(schema);
        let stream;
        try {
          stream = await startChat(messages, params);
        } catch (err) {
          if (useGrammar && llm && /^HTTP [45]\d\d/.test(String((err && err.message) || ''))) {
            useGrammar = false; // the format refused, not the rewrite: drop it once
            continue;
          }
          throw err;
        }
        let raw = '';
        for await (const choice of stream) {
          const piece = choice && choice.delta && choice.delta.content;
          if (piece) {
            raw += piece;
            out.textContent = raw;
          }
        }
        outDone();

        // A cut answer is not a bad document: the stop reason tells them
        // apart, and the cut one is what the second attempt is told about.
        if (streamStop === 'length' && attempt < 2) {
          messages.push({ role: 'user', content: label(
            'Ta réponse a été coupée avant la fin. Renvoie le JSON complet, en entier, sans le couper.',
            'Your answer was cut before the end. Send the complete JSON, whole, without cutting it.'
          ) });
          attempt += 1;
          continue;
        }
        if (streamStop === 'length') {
          return status(label(
            'La réponse a été coupée : la limite de tokens a été atteinte avant la fin du JSON.',
            'The answer was cut: the token limit was reached before the end of the JSON.'
          ), 'error');
        }

        // The raw answer stays on screen from here: if it is refused, it is
        // what the reader needs to see, and if it is accepted it is replaced
        // by the form it just filled.
        let candidate;
        try {
          candidate = extractJSON(raw);
        } catch (e) {
          const defect = (e && e.message) || String(e);
          if (attempt < 2) {
            messages.push({ role: 'user', content: label(
              'Ta réponse précédente ne contenait pas le JSON complet. Renvoie UNIQUEMENT le JSON complet, sans commentaire ni bloc de code. La raison : ',
              'Your previous answer did not contain the whole JSON. Send ONLY the complete JSON, no comment and no code fence. The reason: '
            ) + defect + label('. Réessaie avec le JSON complet.', '. Send the complete JSON now.') });
            attempt += 1;
            continue;
          }
          return status(defect, 'error');
        }
        status(label('Validation du JSON…', 'Validating the JSON…'), 'waiting');
        const checked = await validate(candidate);
        if (!checked.ok) {
          const defect = checked.error || label(
            'Le modèle a renvoyé un JSON refusé.',
            'The model returned a JSON that was refused.'
          );
          if (attempt < 2) {
            messages.push({ role: 'user', content: label(
              'Le validateur a refusé ton JSON avec cette erreur : ',
              'The validator refused your JSON with this error: '
            ) + defect + label(
              '. Corrige exactement ce défaut et renvoie le JSON complet corrigé.',
              '. Fix exactly that defect and send the corrected complete JSON.'
            ) });
            attempt += 1;
            continue;
          }
          return status(defect, 'error');
        }

        // The translation is read back against the document it came from:
        // its verdict travels the same road as a refusal from the server,
        // second chance included.
        const damaged = mode === 'translate' ? untouched(base, checked.resume, target) : [];
        if (damaged.length) {
          const hurt = damaged.join(', ');
          if (attempt < 2) {
            messages.push({ role: 'user', content: label(
              'Ta réponse a perdu ou modifié des champs qu\'une traduction ne doit pas toucher : ',
              'Your answer lost or altered fields a translation must not touch: '
            ) + hurt + label(
              '. Renvoie le JSON complet, avec ces champs rigoureusement identiques.',
              '. Send the complete JSON, with those fields strictly identical.'
            ) });
            attempt += 1;
            continue;
          }
          return status(label(
            'La traduction a altéré le document : ',
            'The translation altered the document: '
          ) + hurt, 'error');
        }

        const baseName = (checked.resume.name || '').trim() ||
          (typeof T !== 'undefined' ? T.unnamed() : 'Sans titre');
        const name = mode === 'translate'
          ? baseName + ' — ' + target.toUpperCase()
          : baseName + label(' — optimisé', ' — optimized');
        applySession(checked.resume, name);
        return;
      }
    } catch (err) {
      if (phase === 'chat') engine = null;
      out.hidden = true;
      outDone();
      status(label('Erreur : ', 'Error: ') + ((err && err.message) || String(err)), 'error');
    } finally {
      spin(false);
      downloadOff();
      setBusy(false);
    }
  };

  run.addEventListener('click', () => review());
  // The two rewrites ship with the page: « Appliquer les améliorations »
  // opens a new version guided by the last analysis, « Traduire » opens one
  // in the other language — neither touches the session under the editor.
  if (optimize) optimize.addEventListener('click', () => rewrite('optimize'));
  if (translate) translate.addEventListener('click', () => rewrite('translate'));
  // The rehearsal: written here, never into the editor.
  if (sim) sim.addEventListener('click', () => runSimulation());
  // The offer: fetch on click or on Enter, clear on the clear button, and the
  // saved line restored exactly where it was when the panel is opened again.
  jobRun.addEventListener('click', () => fetchOfferJob());
  jobUrl.addEventListener('keydown', (e) => {
    if ((e && e.key) === 'Enter') {
      e.preventDefault(); // a URL field must not submit the page around it
      fetchOfferJob();
    }
  });
  offerClear.addEventListener('click', () => {
    localStorage.removeItem(OFFER_KEY);
    renderOffer();
    status('');
  });
  // The provider: save on click or on Enter in either field, retire on the
  // clear button, restore the saved values where they were when the panel
  // is opened again. A page that ships no provider block skips all of it and
  // keeps the local model.
  if (provSave && provUrl && provModel && provKey) {
    provSave.addEventListener('click', () => saveProvider());
    for (const field of [provUrl, provModel, provKey]) {
      field.addEventListener('keydown', (e) => {
        if ((e && e.key) === 'Enter') {
          e.preventDefault(); // a config field must not submit the page
          saveProvider();
        }
      });
    }
  }
  if (provClear) provClear.addEventListener('click', clearProvider);
  renderOffer();
  renderProvider();
})();
