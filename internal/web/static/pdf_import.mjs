/* Reading a PDF in the browser, and nothing else.
 *
 * The split with the server is deliberate and it is the only way this works
 * inside this tool's design: the server keeps no file and no draft, so the file
 * is decoded here, where it already is, and only the text it yields is posted to
 * /api/import. That endpoint is then exactly as cheap as the rest of the API.
 *
 * What comes back from an extraction is not text, it is a list of positioned
 * runs. Lines have to be rebuilt from the geometry or the result is a single
 * paragraph, and the server would then find no heading and no bullet. The
 * tolerance below is a fraction of the glyph height rather than a fixed number
 * of points, so it holds for a 7pt line and a 22pt one alike.
 *
 * pdf.js is loaded by the caller and passed in, so this module has no import of
 * its own: the browser reaches it through a URL that only exists there, while
 * the tests reach the same code with a path that exists on disk. */

const LINE_TOLERANCE = 0.5;   // a run this far off the baseline is a new line
const GAP_RATIO = 0.22;       // a gap this wide, relative to the glyphs, is a space
const MIN_LETTERS = 20;       // below this the PDF has no text layer, it is a scan

/* ErrNoTextLayer is what a scan produces. It is reported as such rather than as
 * an empty draft, because "the import found nothing" and "the import worked" have
 * to look different to the person who just clicked the button. */
export class NoTextLayer extends Error {
  constructor() {
    super('Ce PDF ne contient pas de couche texte : il est probablement scanné. ' +
      'Un scan demande de l’OCR, que cet outil ne fait pas. Exportez votre CV ' +
      'depuis un document texte, ou partez du JSON.');
    this.name = 'NoTextLayer';
  }
}

/* extractText turns the bytes of a PDF into plain text, one block per page.
 *
 * pdfjs is the pdf.js module, already loaded. onProgress is called with the page
 * count as the pages arrive, because a two page CV takes long enough that a
 * silent button looks broken. */
export async function extractText(bytes, pdfjs, onProgress) {
  // The loading task is kept because it is what releases the worker and the
  // parsed objects: the document proxy stopped owning that in pdf.js 6.
  const task = pdfjs.getDocument({
    data: new Uint8Array(bytes),
    // The editor's own policy forbids unsafe-eval. pdf.js only needs eval for
    // its font engine, and without it a font that cannot be parsed is skipped
    // instead of failing the page.
    isEvalSupported: false,
  });
  const doc = await task.promise;

  const pages = [];
  try {
    for (let n = 1; n <= doc.numPages; n++) {
      const page = await doc.getPage(n);
      const content = await page.getTextContent();
      pages.push(linesFrom(content.items));
      page.cleanup();
      if (onProgress) onProgress(n, doc.numPages);
    }
  } finally {
    await (typeof task.destroy === 'function' ? task.destroy() : doc.destroy());
  }

  const text = pages.join('\n\n');
  if (countLetters(text) < MIN_LETTERS) throw new NoTextLayer();
  return { text, pages: doc.numPages };
}

function countLetters(s) {
  const m = s.match(/\p{L}/gu);
  return m ? m.length : 0;
}

/* linesFrom rebuilds the lines of a page from its text runs.
 *
 * Two numbers carry the whole job. Runs are grouped by their baseline, because
 * that is what makes a line a line, and within a line a gap wider than a small
 * fraction of the glyphs is a space, because that is what separates words when
 * the PDF does not say so itself. */
export function linesFrom(items) {
  const runs = items
    .filter((it) => it.str !== undefined && it.str !== '')
    .map((it) => {
      const t = it.transform || [1, 0, 0, 1, 0, 0];
      return {
        text: it.str,
        x: t[4],
        y: t[5],
        // The height of the run, which is the unit both tolerances are relative
        // to. A run with no height falls back to the matrix scale.
        size: Math.abs(it.height || t[3] || 10),
        // The advance of the run, used to measure the gap to the next one. Not
        // every producer fills it in, hence the fallback in needsSpace.
        width: Math.abs(it.width || 0),
        eol: Boolean(it.hasEOL),
      };
    });
  if (runs.length === 0) return '';

  // Top to bottom, then left to right. A page is read that way and so is this.
  runs.sort((a, b) => (b.y - a.y) || (a.x - b.x));

  const lines = [];
  let current = null;
  let baseline = 0;
  for (const run of runs) {
    if (current === null || Math.abs(run.y - baseline) > run.size * LINE_TOLERANCE) {
      current = { y: run.y, parts: [], width: 0 };
      lines.push(current);
      baseline = run.y;
    }
    // A run that ends its line says so, and nothing that follows belongs to it.
    if (run.eol) {
      current.parts.push({ ...run, space: false, last: true });
      current = null;
      continue;
    }
    current.parts.push({ ...run, space: false, last: false });
  }

  const out = [];
  for (const line of lines) {
    let s = '';
    let prevEnd = null;
    for (const part of line.parts) {
      if (prevEnd !== null && needsSpace(prevEnd, part)) s += ' ';
      s += part.text;
      // hasEOL aside, the run ends where its own width says it does, which is
      // what the gap to the next run is measured from.
      prevEnd = part.last ? null : part.x + (part.width || 0);
    }
    const trimmed = s.trim();
    if (trimmed) out.push(trimmed);
  }
  return out.join('\n');
}

/* needsSpace reports whether a gap between two runs stands for a space. The
 * width of the run is not always in the content, so the comparison falls back to
 * the start of the next run: a run that begins exactly where the last one ended
 * is a word continuing, not a new one. */
function needsSpace(prevEnd, next) {
  if (next.space) return true;
  const gap = next.x - prevEnd;
  if (gap > next.size * GAP_RATIO) return true;
  if (gap < -next.size * 0.5) return true;  // overlapping runs are two columns
  return false;
}
