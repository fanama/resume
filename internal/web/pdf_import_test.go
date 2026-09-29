package web

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestThePDFImportRoundTrips is the test that covers the whole feature: a resume
// rendered by this project, read back by the pdf.js it vendors, and turned into
// the text the import endpoint is given.
//
// The unit tests on either side can both pass while the two disagree, and that
// disagreement is invisible until a real file is read: a renderer that writes a
// line as one run, an extraction that reads it as two, and a draft whose
// sections are silently empty. So the PDF is built here, with the real renderer,
// and the vendored library does the reading.
//
// It is skipped when node is missing, for the same reason the editor test is:
// the tool is a Go binary and nothing in its build should require node.
func TestThePDFImportRoundTrips(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}

	pdf := renderPDFForImport(t)
	noText := pdfWithNoTextLayer(t)

	fixture := map[string]any{
		"pdfBase64":       base64.StdEncoding.EncodeToString(pdf),
		"noTextPDFBase64": base64.StdEncoding.EncodeToString(noText),
		"expectedPages":   1,
		// The lines of the rendered sample, as the extraction has to give them
		// back. They are the section headings and the identity, because those
		// are what the heuristics read: a renderer that emitted them as one run
		// and an extractor that split them would leave the draft empty.
		"expectedLines": []string{
			"Jean Dupont",
			"jean@exemple.fr",
			"EXPÉRIENCE PROFESSIONNELLE",
			"Acme, Paris",
		},
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "pdf-fixture.json")
	fixture["writeTextTo"] = filepath.Join(dir, "extracted.txt")
	body, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}

	out, err := exec.Command(node, filepath.Join("static", "pdf_extract_test.mjs"), path).CombinedOutput()
	if err != nil {
		t.Fatalf("the pdf round trip failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "pdf: all checks passed") {
		t.Fatalf("unexpected output:\n%s", out)
	}

	// The last link of the chain: the text the library produced is what the
	// endpoint is given, and the endpoint has to answer with a filled draft.
	// Every stage above can pass while this one fails, because the extractor's
	// idea of a line and the reader's idea of a section are not the same thing.
	extracted, err := os.ReadFile(fixture["writeTextTo"].(string))
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]string{"text": string(extracted)})
	if err != nil {
		t.Fatal(err)
	}
	rec := postImport(t, string(payload))
	if rec.Code != 200 {
		t.Fatalf("importing the extracted text: status %d: %s", rec.Code, rec.Body)
	}
	var got importResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Resume.Name != "Jean Dupont" {
		t.Errorf("the name came back as %q, want Jean Dupont", got.Resume.Name)
	}
	if got.Resume.Contact.Email != "jean@exemple.fr" {
		t.Errorf("the email came back as %q", got.Resume.Contact.Email)
	}
	if len(got.Resume.Experience) != 1 {
		t.Fatalf("the round trip found %d jobs, want 1", len(got.Resume.Experience))
	}
	if e := got.Resume.Experience[0]; e.Company != "Acme" {
		t.Errorf("the company came back as %q, want Acme", e.Company)
	}
}

// renderPDFForImport produces a one page PDF through the real renderer, so the
// bytes under test are the bytes a user gets.
func renderPDFForImport(t *testing.T) []byte {
	t.Helper()
	r := testServer(t)
	rec := post(t, r, "/api/render/pdf", sample)
	if rec.Code != 200 {
		t.Fatalf("rendering the PDF: status %d: %s", rec.Code, rec.Body)
	}
	if !strings.HasPrefix(rec.Body.String(), "%PDF") {
		t.Fatalf("the renderer did not produce a PDF: %.40q", rec.Body.String())
	}
	return rec.Body.Bytes()
}

// pdfWithNoTextLayer is a structurally valid PDF whose only page has no text
// operator at all: the shape a scan leaves behind. The import has to say so.
func pdfWithNoTextLayer(t *testing.T) []byte {
	t.Helper()
	// One page, one empty content stream, drawn as a rectangle: a document with
	// ink and no characters, which is what a scan looks like to an extractor.
	const objs = `1 0 obj
<< /Type /Catalog /Pages 2 0 R >>
endobj
2 0 obj
<< /Type /Pages /Kids [3 0 R] /Count 1 >>
endobj
3 0 obj
<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Contents 4 0 R /Resources << >> >>
endobj
4 0 obj
<< /Length 44 >>
stream
0.5 0.5 0.5 rg 10 10 180 180 re f
endstream
endobj
`
	var b strings.Builder
	b.WriteString("%PDF-1.4\n")
	offsets := make([]int, 0, 5)
	for i := 1; i <= 4; i++ {
		offsets = append(offsets, b.Len())
		b.WriteString(objs[strings.Index(objs, itoa(i)+" 0 obj"):])
	}
	xref := b.Len()
	b.WriteString("xref\n0 5\n0000000000 65535 f \n")
	for _, off := range offsets {
		b.WriteString(pad10(off) + " 00000 n \n")
	}
	b.WriteString("trailer\n<< /Size 5 /Root 1 0 R >>\nstartxref\n" + itoa(xref) + "\n%%EOF\n")
	return []byte(b.String())
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

func pad10(n int) string {
	s := itoa(n)
	for len(s) < 10 {
		s = "0" + s
	}
	return s
}
