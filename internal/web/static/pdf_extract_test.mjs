/* End to end check of the PDF import: a PDF this project renders, read back by
 * the pdf.js it vendors, turned into the text the endpoint is then given.
 *
 * It is the only test that covers the whole path, and the only one that can
 * catch the failure that matters, which is a mismatch between what the renderer
 * writes and what an extraction reads back. Usage:
 *
 *   node pdf_extract_test.mjs <fixture.json>
 *
 * The fixture carries the PDF produced by the Go renderer and the ids the page
 * declares, so the test runs against the real files and the real schema. */

import { readFileSync } from 'node:fs';
import { pathToFileURL } from 'node:url';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const fixture = JSON.parse(readFileSync(process.argv[2], 'utf8'));

const failures = [];
const check = (ok, message) => { if (!ok) failures.push(message); };

/* pdf.js in node needs the same two settings the browser gets, and a worker it
 * can actually load from disk. */
const pdfjs = await import(pathToFileURL(join(here, 'vendor', 'pdf.min.mjs')).href);
pdfjs.GlobalWorkerOptions.workerSrc = pathToFileURL(join(here, 'vendor', 'pdf.worker.min.mjs')).href;

const { extractText, linesFrom, NoTextLayer } = await import(
  pathToFileURL(join(here, 'pdf_import.mjs')).href
);

/* ---------------------------------------------------------- the round trip */

const bytes = Uint8Array.from(atob(fixture.pdfBase64), (c) => c.charCodeAt(0));
const { text, pages } = await extractText(bytes, pdfjs);

// The text is handed back to the caller, which is how the Go test feeds it to
// /api/import. Writing it out is what makes the round trip complete: the file
// the renderer produced, read by the vendored library, is the file the endpoint
// is then given.
if (fixture.writeTextTo) {
  const { writeFileSync } = await import('node:fs');
  writeFileSync(fixture.writeTextTo, text, 'utf8');
}

check(pages === fixture.expectedPages,
  `pages = ${pages}, want ${fixture.expectedPages}`);
check(text.length > 0, 'the extraction produced no text at all');

for (const line of fixture.expectedLines) {
  check(text.includes(line), `the extracted text does not contain ${JSON.stringify(line)}`);
}

/* The reconstruction is the fragile half: a PDF stores glyphs, and the lines
 * have to be rebuilt from the positions. Every line the renderer laid out has to
 * come back as one line, not as one paragraph. */
const extractedLines = text.split('\n').map((l) => l.trim()).filter(Boolean);
for (const line of fixture.expectedLines) {
  const hit = extractedLines.find((l) => l.includes(line));
  check(hit !== undefined, `${JSON.stringify(line)} is not a line of its own in the output`);
}

/* And the order has to survive: a document read in the wrong order is a resume
 * with its jobs reversed, which the heuristics cannot detect. */
const positions = fixture.expectedLines.map((l) => {
  const i = extractedLines.findIndex((x) => x.includes(l));
  return i;
});
const ordered = positions.every((p, i) => i === 0 || p > positions[i - 1]);
check(ordered, 'the lines came back out of order, so the sections would be scrambled');

/* ------------------------------------------------------------ the no-text case */

try {
  const empty = Uint8Array.from(atob(fixture.noTextPDFBase64), (c) => c.charCodeAt(0));
  let threw = false;
  try {
    await extractText(empty, pdfjs);
  } catch (err) {
    threw = err instanceof NoTextLayer;
  }
  check(threw, 'a PDF with no text layer did not report NoTextLayer');
} catch (err) {
  check(false, `the no-text fixture could not be read: ${err.message}`);
}

/* ------------------------------------------------- the line grouping, alone */

/* A PDF of a two column page interleaves the columns when read row by row. The
 * grouping has to notice the jump back to the left margin, or the two columns
 * turn into a single sentence. */
const twoColumns = [
  { str: 'Droite un', transform: [1, 0, 0, 1, 300, 700], height: 11, width: 40 },
  { str: 'Gauche un', transform: [1, 0, 0, 1, 50, 700], height: 11, width: 45 },
  { str: 'Droite deux', transform: [1, 0, 0, 1, 300, 688], height: 11, width: 50 },
  { str: 'Gauche deux', transform: [1, 0, 0, 1, 50, 688], height: 11, width: 55 },
];
const rebuilt = linesFrom(twoColumns).split('\n');
check(rebuilt.length === 2, `a two column page came back as ${rebuilt.length} lines, want 2`);
check(rebuilt[0] === 'Gauche un Droite un', `line 1 = ${JSON.stringify(rebuilt[0])}`);
check(rebuilt[1] === 'Gauche deux Droite deux', `line 2 = ${JSON.stringify(rebuilt[1])}`);

/* A font size change on the same baseline is a new paragraph, not a new line,
 * as long as the baseline holds. */
const sizes = [
  { str: 'Titre', transform: [1, 0, 0, 1, 50, 700], height: 18, width: 30 },
  { str: 'suite', transform: [1, 0, 0, 1, 84, 700], height: 11, width: 25 },
];
check(linesFrom(sizes) === 'Titre suite', 'a size change split a line that was one');

if (failures.length) {
  console.error('pdf: ' + failures.length + ' check(s) failed');
  for (const f of failures) console.error(' - ' + f);
  process.exit(1);
}
console.log('pdf: all checks passed');
