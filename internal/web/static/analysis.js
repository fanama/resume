/* The analysis panel: a review of the resume written by a small language model
   that runs in the browser itself, through WebLLM and WebGPU — and two things
   the same model can do with the resume afterwards: rewrite its content, and
   translate it into the other language the tool supports.

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
  const note = document.getElementById('ia-note');
  const statusNode = document.getElementById('ia-status');
  const out = document.getElementById('ia-out');
  const dataField = document.getElementById('data');
  const spinner = document.getElementById('ia-spinner');
  const progress = document.getElementById('ia-progress');
  const bar = document.getElementById('ia-bar');
  const pct = document.getElementById('ia-pct');
  // The three actions. Optimize and translate may be commented out of the
  // page — they are today — and the panel must survive that: a missing
  // optional button removes a feature, it must not leave the others dead.
  // The elements every listener needs are the required ones, never optional.
  // The provider block is not among them: a page without it just loses the
  // custom provider and keeps the local model, the way it loses optimize
  // without its button.
  const buttons = [run, optimize, translate, jobRun].filter(Boolean);
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

  /* The engine survives between two runs: reloading it would re-read the
     cached weights, and the first load is the expensive one. */
  let engine = null;
  let busy = false;

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

  const rewriteMessages = (resume, mode, target) => {
    const json = JSON.stringify(resume);
    const rules = mode === 'translate'
      ? 'Traduis ce CV en ' + (target === 'en' ? 'anglais' : 'français') + '.\n' +
        '- Traduis le contenu textuel : accroche, résumé, titres et points des entrées, ' +
        'compétences, langues, activités, projets, certifications, libellés de liens.\n' +
        '- Mets le champ lang à "' + target + '".\n' +
        "- Ne traduis pas le nom de la personne, les emails, téléphones, villes et URLs, " +
        'et garde les dates au format AAAA-MM ou AAAA.\n' +
        '- Traduis fidèlement : n\'invente ni ne supprime aucun fait.'
      : 'Améliore la formulation de ce CV.\n' +
        '- Réécris accroche, résumé et points des entrées : verbe d\'action en tête de phrase, ' +
        'phrases courtes, chiffres conservés tels quels.\n' +
        "- N'invente aucun fait, chiffre, poste, entreprise, date ni compétence : " +
        "c'est la formulation qui change, pas le contenu.\n" +
        '- Ne change ni les clés, ni les dates, ni le champ lang, ni le contact, ni le nom ; ' +
        'les champs vides restent vides ; la langue du CV est conservée.';
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

  /* sseDeltas reads the stream the OpenAI servers write: "data:" lines, the
     deltas of the choices they carry, a [DONE] line at the end. Every
     OpenAI-compatible provider has agreed on that shape, so one reader
     covers them all — and every line the model writes here is content, in
     the same shape the local engine reports its chunks in. */
  const sseDeltas = async function* (res) {
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
          const delta = json && json.choices && json.choices[0] && json.choices[0].delta;
          if (delta && delta.content) yield delta;
        } catch (e) { /* a partial line or a provider note: content is what matters */ }
      }
    }
  };

  /* startChat is the one door both actions walk through: the configured
     provider when there is one, the local engine when there is not. It
     answers an iterable of the same deltas either way, so the streaming
     below is written once. */
  const startChat = async (messages, params) => {
    if (llm) {
      const res = await providerChat(messages, params);
      return sseDeltas(res);
    }
    const eng = await ensureEngine();
    return eng.chat.completions.create({ messages, stream: true, ...params });
  };

  /* applySession hands a validated resume to the editor the way an import
     does: a new session, then every step that decides what the next render
     shows. app.js owns all of it — this file only calls. */
  const applySession = (resume, name) => {
    newSession(resume, name);
    persist();
    resetSection();
    rerender();
    sync();
    refreshOutput();
    showView('edit');
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

      spin(true);
      status(label('Lecture du CV…', 'Reading the CV…'), 'waiting');
      const res = await fetch('/api/text?format=json', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: dataField.value,
      });
      if (!res.ok) {
        const body = await res.json().catch(() => ({}));
        return status(body.error || draftError(), 'error');
      }
      const text = await res.text();
      if (!text.trim()) {
        return status(label('Le CV est vide.', 'The CV is empty.'), 'error');
      }

      phase = 'chat';
      spin(true);
      status(label('Analyse en cours…', 'Analysing…'), 'waiting');
      showOut('');
      const started = Date.now();
      const stream = await startChat(reviewMessages(text, loadOffer()), {
        temperature: 0.7,
        top_p: 0.95,
        max_tokens: 700,
      });
      let answer = '';
      for await (const delta of stream) {
        if (delta && delta.content) {
          answer += delta.content;
          out.textContent = answer;
        }
      }
      outDone();
      if (!answer.trim()) {
        out.hidden = true;
        return status(label(
          'Le modèle n\'a rien renvoyé : réessayez.',
          'The model returned nothing: try again.'
        ), 'error');
      }
      const seconds = Math.max(1, Math.round((Date.now() - started) / 1000));
      status(label('Réponse en ', 'Answer in ') + seconds + ' s');
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

  /* rewrite is both actions: the draft is validated first (the model must be
     given a sound baseline), the model returns the whole JSON, and the result
     goes back through validation before a session opens. Three gates —
     nothing enters the editor that the server has not accepted. */
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

      phase = 'chat';
      spin(true);
      status(mode === 'translate'
        ? label('Traduction en ' + (target === 'en' ? 'anglais' : 'français') + '…',
          'Translating into ' + (target === 'en' ? 'English' : 'French') + '…')
        : label('Réécriture du CV…', 'Rewriting the CV…'), 'waiting');
      showOut('');
      const stream = await startChat(rewriteMessages(base, mode, target), {
        temperature: 0.3,
        top_p: 0.95,
        max_tokens: 2500,
      });
      let raw = '';
      for await (const delta of stream) {
        if (delta && delta.content) {
          raw += delta.content;
          out.textContent = raw;
        }
      }
      outDone();

      // The raw answer stays on screen from here: if it is refused, it is
      // what the reader needs to see, and if it is accepted it is replaced
      // by the form it just filled.
      let candidate;
      try {
        candidate = extractJSON(raw);
      } catch (e) {
        return status((e && e.message) || String(e), 'error');
      }
      status(label('Validation du JSON…', 'Validating the JSON…'), 'waiting');
      const checked = await validate(candidate);
      if (!checked.ok) {
        return status(checked.error || label(
          'Le modèle a renvoyé un JSON refusé : réessayez.',
          'The model returned a JSON that was refused: try again.'
        ), 'error');
      }

      const baseName = (checked.resume.name || '').trim() ||
        (typeof T !== 'undefined' ? T.unnamed() : 'Sans titre');
      const name = mode === 'translate'
        ? baseName + ' — ' + target.toUpperCase()
        : baseName + label(' — optimisé', ' — optimized');
      applySession(checked.resume, name);
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
  // The two rewrites live behind the buttons the page chooses to ship: a
  // commented-out button gets no listener and costs nothing.
  if (optimize) optimize.addEventListener('click', () => rewrite('optimize'));
  if (translate) translate.addEventListener('click', () => rewrite('translate'));
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
