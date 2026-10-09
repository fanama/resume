/* The analysis panel: a review of the resume written by a small language model
   that runs in the browser itself, through WebLLM and WebGPU.

   Three properties shape this file. The engine is 6.6 Mo of JavaScript and the
   model is 1.4 Go of weights, so both are fetched the first time the button is
   pressed and never on page load: a dynamic import for the first, WebLLM's own
   cache for the second. The resume is personal data, so the inference happens
   here, in the page — the only outbound requests are the model's own files,
   which is what the Hugging Face sources in the Content-Security-Policy are
   for; the resume itself is never sent anywhere. And the model's answer is
   handed to textContent and never to innerHTML: a model that echoes a
   <script> found in the resume must not become one. */

(function () {
  'use strict';

  // The editor language, when app.js is on the page with its translation
  // table; French when this file is loaded on its own, as in the tests.
  const label = (fr, en) => (typeof T !== 'undefined' ? T.label(fr, en) : fr);

  const MODEL_ID = 'MiniCPM5-2B-q4f16_1-MLC';
  const REPO = 'https://huggingface.co/vs4vijay/MiniCPM5-2B-MLC/resolve/main';

  // The record WebLLM needs to load MiniCPM5-2B: the weights sit in a
  // subfolder of the repository in the standard WebLLM layout, the kernels are
  // the wasm file beside them, and the model is a hybrid thinking model whose
  // 32768-token window can be shrunk at load time — 8k is far more than a
  // resume and its review need, and the smaller window keeps the KV cache
  // small enough for an ordinary laptop GPU.
  const model = {
    model: REPO + '/q4f16_1',
    model_id: MODEL_ID,
    model_lib: REPO + '/q4f16_1/libs/MiniCPM5-2B-q4f16_1-MLC-webgpu.wasm',
    vram_required_MB: 2000,
    overrides: { context_window_size: 8192 },
  };

  const run = document.getElementById('ia-run');
  const statusNode = document.getElementById('ia-status');
  const out = document.getElementById('ia-out');
  const dataField = document.getElementById('data');
  if (!run || !statusNode || !out || !dataField) return;

  // The engine survives between two analyses: reloading it would re-read the
  // cached weights, and the first load is the expensive one.
  let engine = null;
  let busy = false;

  const status = (text) => { statusNode.textContent = text || ''; };
  const hasWebGPU = () => typeof navigator !== 'undefined' && Boolean(navigator.gpu);

  /* messages builds the prompt. The model sees exactly what a parser reads
     back from the page — the same text the "Texte brut" panel shows — because
     that text is the document as the hiring pipeline receives it. */
  const messages = (text) => [
    {
      role: 'system',
      content:
        "Tu es un recruteur senior et un expert de l'optimisation des CV pour les logiciels de tri ATS. " +
        'Tu analyses des CV avec rigueur et tu écris des conseils applicables immédiatement.',
    },
    {
      role: 'user',
      content:
        'Voici le CV, tel qu\'un parseur le lit :\n\n' + text + '\n\n' +
        "Propose des améliorations concrètes, dans la langue du CV (français par défaut), " +
        'avec exactement ces trois rubriques, séparées par une ligne vide :\n' +
        '1. Points forts — ce qui fonctionne déjà, en 3 points maximum.\n' +
        '2. À améliorer — un problème par ligne, chacun suivi de la raison.\n' +
        '3. Suggestions — des réécritures prêtes à coller, verbe d\'action et chiffres en tête de phrase.\n' +
        "Ne rapporte aucune expérience ni aucun chiffre qui n'est pas dans le CV. " +
        'Quatre cents mots au maximum.',
    },
  ];

  run.addEventListener('click', async () => {
    if (busy) return;
    busy = true;
    run.disabled = true;
    out.hidden = true;
    out.textContent = '';
    // Which step failed, so the catch knows whether the engine worth keeping
    // is still worth keeping: a broken draft never touched it, a lost device
    // did.
    let phase = 'text';
    try {
      if (!hasWebGPU()) {
        status(label(
          'WebGPU indisponible : ce navigateur ne peut pas exécuter l\'analyse.',
          'WebGPU is unavailable: this browser cannot run the analysis.'
        ));
        return;
      }

      // The text the parser reads is what the model reviews, and it is asked
      // for first: a draft the server refuses must fail here, before a
      // gigabyte of weights is fetched.
      const res = await fetch('/api/text?format=json', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: dataField.value,
      });
      if (!res.ok) {
        let message = '';
        try { message = (await res.json()).error || ''; } catch (e) { /* no detail */ }
        status(message || label(
          'Le CV contient des erreurs : corrigez-les avant l\'analyse.',
          'The CV has errors: fix them before the analysis.'
        ));
        return;
      }
      const text = await res.text();
      if (!text.trim()) {
        status(label('Le CV est vide.', 'The CV is empty.'));
        return;
      }

      if (!engine) {
        phase = 'engine';
        status(label('Chargement du moteur…', 'Loading the engine…'));
        const webllm = await import('/static/vendor/webllm.js');
        engine = await webllm.CreateMLCEngine(MODEL_ID, {
          appConfig: { model_list: [model] },
          initProgressCallback: (report) => {
            const pct = Math.round((report.progress || 0) * 100);
            status(label('Préparation du modèle — ', 'Preparing the model — ') + pct + ' %');
          },
        });
      }

      phase = 'chat';
      status(label('Analyse en cours…', 'Analysing…'));
      out.hidden = false;
      const stream = await engine.chat.completions.create({
        messages: messages(text),
        stream: true,
        temperature: 0.7,
        top_p: 0.95,
        max_tokens: 700,
      });
      let answer = '';
      for await (const chunk of stream) {
        const delta = chunk && chunk.choices && chunk.choices[0] && chunk.choices[0].delta;
        if (delta && delta.content) {
          answer += delta.content;
          out.textContent = answer;
        }
      }
      if (!answer.trim()) {
        out.hidden = true;
        status(label(
          'Le modèle n\'a rien renvoyé : réessayez.',
          'The model returned nothing: try again.'
        ));
        return;
      }
      status('');
    } catch (err) {
      if (phase === 'chat') engine = null;
      out.hidden = true;
      status(label('Erreur : ', 'Error: ') + ((err && err.message) || String(err)));
    } finally {
      busy = false;
      run.disabled = false;
    }
  });
})();
