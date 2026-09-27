package web

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/fanama/resume/atscv/internal/layout"
	"github.com/fanama/resume/atscv/internal/model"
)

func testServer(t *testing.T) *Server {
	t.Helper()
	s, err := New(DefaultOptions())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

// post sends a resume to an endpoint, the way the editor does.
func post(t *testing.T, s *Server, path string, resume string) *httptest.ResponseRecorder {
	t.Helper()
	form := url.Values{"data": {resume}}
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

func get(t *testing.T, s *Server, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

const sample = `{
  "lang": "fr",
  "name": "Jean Dupont",
  "headline": "Développeur Go",
  "contact": {"email": "jean@exemple.fr", "city": "Paris", "links": [{"label": "GitHub", "url": "github.com/jean"}]},
  "summary": "Développeur backend depuis six ans.",
  "experience": [{
    "title": "Ingénieur", "company": "Acme", "location": "Paris",
    "start": "2021", "end": "present",
    "summary": "Plateforme de paiement.",
    "highlights": ["Migration vers Go, latence divisée par 4.", "Refonte du déploiement."],
    "team": "3 développeurs", "stack": ["Go", "PostgreSQL"]
  }],
  "education": [{"degree": "Master", "school": "SU", "start": "2016", "end": "2018"}],
  "skills": [{"category": "Backend", "items": ["Go", "SQL"]}],
  "languages": [{"name": "Français", "level": "Natif"}]
}`

func TestPageServesTheShell(t *testing.T) {
	rec := get(t, testServer(t), "/editor")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{`<script src="/static/htmx.min.js"`, `id="schema-target"`, `id="data"`, `hx-post="/api/preview"`, `hx-post="/api/lint"`, `hx-post="/api/text"`, `href="/api/style.css?accent=1F3864`} {
		if !strings.Contains(body, want) {
			t.Errorf("the page is missing %q", want)
		}
	}
	// The policy forbids inline script, so the shell must not carry any: a
	// single <script> block with a body would be blocked by the browser and the
	// editor would silently stop working. ParseFS names the templates after
	// their file, so the only scripts allowed here are the external ones.
	for _, tag := range between(body, "<script", ">") {
		if !strings.Contains(tag, "src=") {
			t.Errorf("inline script found: <script%s>", tag)
		}
	}
}

// between returns the text that follows every occurrence of open until close.
func between(s, open, close string) []string {
	var out []string
	for {
		i := strings.Index(s, open)
		if i < 0 {
			return out
		}
		s = s[i+len(open):]
		j := strings.Index(s, close)
		if j < 0 {
			return out
		}
		out = append(out, s[:j])
		s = s[j+len(close):]
	}
}

// TestHomeServesTheLandingPage covers the front door. The page has to sell the
// tool in French, link to the editor, and stay inside the same policy as the
// editor: one stylesheet, no inline script, no third party.
func TestHomeServesTheLandingPage(t *testing.T) {
	rec := get(t, testServer(t), "/")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`<html lang="fr">`,
		`<title>atscv — un CV que les ATS lisent`,
		`href="/static/home.css"`,
		`href="/editor"`,
		`id="regle"`,
		`id="etapes"`,
		`id="linter"`,
		`id="pagination"`,
		`id="installation"`,
		`<h1>`,
		`<h2>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the landing page is missing %q", want)
		}
	}
	// A landing page that shipped its own script would break the policy the
	// editor depends on, and a page that reads a resume would turn a visit into
	// a data access.
	for _, tag := range between(body, "<script", ">") {
		if !strings.Contains(tag, "src=") {
			t.Errorf("inline script found: <script%s>", tag)
		}
	}
	if strings.Contains(body, `style=`) {
		t.Error("an inline style reached the landing page, the policy forbids it")
	}
	for _, third := range []string{"http://", "https://"} {
		if strings.Contains(body, `"`+third) {
			t.Errorf("the landing page loads a third party resource: %s", third)
		}
	}
	if n := strings.Count(body, `href="/editor"`); n < 2 {
		t.Errorf("the landing page offers the editor %d times, want at least 2", n)
	}
}

func TestSecurityHeaders(t *testing.T) {
	rec := get(t, testServer(t), "/")
	want := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "no-referrer",
	}
	for header, value := range want {
		if got := rec.Header().Get(header); got != value {
			t.Errorf("%s = %q, want %q", header, got, value)
		}
	}
	csp := rec.Header().Get("Content-Security-Policy")
	for _, directive := range []string{"default-src 'self'", "script-src 'self'", "object-src 'none'", "frame-ancestors 'none'"} {
		if !strings.Contains(csp, directive) {
			t.Errorf("the policy is missing %q: %s", directive, csp)
		}
	}
}

func TestStaticFilesAreEmbeddedInTheBinary(t *testing.T) {
	s := testServer(t)
	for _, name := range []string{"htmx.min.js", "app.js", "app.css"} {
		rec := get(t, s, "/static/"+name)
		if rec.Code != http.StatusOK {
			t.Errorf("GET /static/%s = %d", name, rec.Code)
			continue
		}
		if rec.Body.Len() == 0 {
			t.Errorf("/static/%s is empty", name)
		}
		if cache := rec.Header().Get("Cache-Control"); cache != "no-cache" {
			t.Errorf("/static/%s Cache-Control = %q, want no-cache", name, cache)
		}
		if rec.Header().Get("ETag") == "" {
			t.Errorf("/static/%s has no ETag, so a browser cannot tell it is stale", name)
		}
	}
}

// TestSchemaCoversTheModel is the drift guard: a field added to model.Resume and
// forgotten in the editor would make it impossible to fill from the browser, and
// the JSON would silently lose it on the next save.
func TestSchemaCoversTheModel(t *testing.T) {
	identity := map[string]bool{}
	for _, f := range IdentitySchema().Fields {
		identity[f.Key] = true
	}
	contact := map[string]bool{}
	for _, f := range ContactSchema().Fields {
		contact[f.Key] = true
	}
	sections := map[string][]Field{}
	for _, s := range Sections() {
		var fields []Field
		for _, f := range s.Fields {
			fields = append(fields, f)
		}
		sections[s.Key] = fields
	}

	contactModel := reflect.TypeOf(model.Contact{})
	for _, f := range fieldsOf(model.Resume{}) {
		// Contact is a struct, not an array: the schema covers it as a block.
		if f.typ == contactModel {
			continue
		}
		switch {
		case identity[f.key]:
		case contact[f.key]:
		case sections[f.key] != nil:
		default:
			t.Errorf("model field %q is not in the editor schema", f.key)
		}
	}
	// The arrays the renderer knows about must be editable.
	for _, f := range fieldsOf(model.Resume{}) {
		if f.kind == reflect.Slice {
			fields, ok := sections[f.key]
			if !ok {
				t.Errorf("the %q array has no section in the schema", f.key)
				continue
			}
			// Every field of the entry type must be editable too, otherwise the
			// array can be created but not filled.
			for _, sub := range fieldsOf(f.typ.Elem()) {
				found := false
				for _, ed := range fields {
					if ed.Key == sub.key {
						found = true
						break
					}
				}
				if !found && sub.key != "" {
					t.Errorf("%s.%s is not in the schema", f.key, sub.key)
				}
			}
		}
	}
	// Contact links are a nested array: the schema must offer a way to add them.
	if !hasFieldKind(ContactSchema().Fields, FieldEntries) {
		t.Error("the contact block cannot hold links")
	}
	for _, f := range fieldsOf(model.Contact{}) {
		if f.kind == reflect.Slice && !contact[f.key] {
			t.Errorf("contact.%s is not in the schema", f.key)
		}
	}
}

func hasFieldKind(fields []Field, kind FieldKind) bool {
	for _, f := range fields {
		if f.Kind == kind {
			return true
		}
	}
	return false
}

type modelField struct {
	key  string
	kind reflect.Kind
	typ  reflect.Type
}

func fieldsOf(v any) []modelField {
	typ := reflect.TypeOf(v)
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	var out []modelField
	for i := range typ.NumField() {
		f := typ.Field(i)
		name := strings.Split(f.Tag.Get("json"), ",")[0]
		if name == "-" || name == "" {
			continue
		}
		out = append(out, modelField{key: name, kind: f.Type.Kind(), typ: f.Type})
	}
	return out
}

func TestPreviewShowsTheRealLayout(t *testing.T) {
	rec := post(t, testServer(t), "/api/preview", sample)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Jean Dupont",
		"Développeur Go",
		"jean@exemple.fr",
		"Migration vers Go, latence divisée par 4.",
		"cv-section",
		"cv-bullet",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the preview is missing %q", want)
		}
	}
	// No inline style: the accent travels in a stylesheet, the policy forbids
	// the alternative.
	if strings.Contains(body, "style=") {
		t.Error("the preview carries an inline style, blocked by the policy")
	}
}

// TestStyleSheetCarriesTheAccent keeps the colour out of the markup and still
// under the control of the server: the value is validated, so a crafted query
// cannot turn the sheet into an injection.
func TestStyleSheetCarriesTheAccent(t *testing.T) {
	s := testServer(t)
	rec := get(t, s, "/api/style.css")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/css") {
		t.Errorf("Content-Type = %q", ct)
	}
	if !strings.Contains(rec.Body.String(), "--accent:#1F3864") {
		t.Errorf("sheet = %q", rec.Body)
	}
	rec = get(t, s, "/api/style.css?accent=0a3b2c&size=12")
	if !strings.Contains(rec.Body.String(), "--accent:#0A3B2C") || !strings.Contains(rec.Body.String(), "--body:12pt") {
		t.Errorf("sheet = %q", rec.Body)
	}
	// A value that is not a colour is refused rather than pasted. Go drops a
	// query pair holding a semicolon, so the payload is percent encoded to
	// prove the check runs on whatever does reach the handler.
	for _, q := range []string{"accent=red", "accent=%3Bbackground:url(x)", "accent=%23zzz", "accent=1F3864%00"} {
		rec = get(t, s, "/api/style.css?"+q)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", q, rec.Code)
		}
	}
	// An out of range size is ignored, not forwarded.
	rec = get(t, s, "/api/style.css?size=900")
	if !strings.Contains(rec.Body.String(), "--body:10.5pt") {
		t.Errorf("an absurd size reached the sheet: %q", rec.Body)
	}
}

func TestPreviewEscapesUserContent(t *testing.T) {
	payload := `{"name":"<script>alert(1)</script>","headline":"\"><img src=x onerror=alert(2)>",
	  "summary":"<svg onload=alert(3)>","contact":{"email":"a@b.fr"},
	  "experience":[{"title":"Dev","company":"<iframe src=javascript:alert(4)>","start":"2020","end":"2022"}]}`
	body := post(t, testServer(t), "/api/preview", payload).Body.String()
	// The angle brackets are escaped, so no tag can be opened. A bare "onerror="
	// left in the text is inert: without a tag there is nothing to fire it.
	for _, live := range []string{"<script", "<img", "<svg", "<iframe", "</script"} {
		if strings.Contains(body, live) {
			t.Errorf("the preview holds a live %q, the content is not escaped", live)
		}
	}
	if !strings.Contains(body, "&lt;script&gt;") {
		t.Error("the payload was dropped instead of being escaped")
	}
	if !strings.Contains(body, "&lt;svg onload=alert(3)&gt;") {
		t.Errorf("the payload is not shown as text: %s", body)
	}
}

func TestTextPanelIsEscaped(t *testing.T) {
	payload := `{"name":"<b>Jean</b>","contact":{"email":"a@b.fr"}}`
	rec := post(t, testServer(t), "/api/text", payload)
	if strings.Contains(rec.Body.String(), "<b>") {
		t.Error("the text panel injects HTML instead of showing the parsed text")
	}
	if !strings.Contains(rec.Body.String(), "&lt;b&gt;") {
		t.Error("the text panel does not show the angle brackets of the content")
	}
	// The JSON shape returns the raw text, which is what a script wants.
	req := httptest.NewRequest(http.MethodPost, "/api/text?format=json", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	testServer(t).Handler().ServeHTTP(rec, req)
	if got := rec.Body.String(); !strings.Contains(got, "<b>Jean</b>") {
		t.Errorf("format=json body = %q", got)
	}
}

func TestLintAnswersBothShapes(t *testing.T) {
	s := testServer(t)
	rec := post(t, s, "/api/lint", sample)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Jean Dupont") &&
		!strings.Contains(rec.Body.String(), "score") {
		t.Errorf("the HTML report is wrong: %d %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `class="score"`) {
		t.Error("the HTML report has no score block")
	}

	form := url.Values{"data": {sample}}
	req := httptest.NewRequest(http.MethodPost, "/api/lint?format=json", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var payload struct {
		Score      int  `json:"score"`
		HasErrors  bool `json:"hasErrors"`
		BySeverity map[string]int
		Findings   []findingJSON
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("the JSON report does not decode: %v (%s)", err, rec.Body)
	}
	if payload.Score <= 0 || payload.Score > 100 {
		t.Errorf("score = %d", payload.Score)
	}
	if payload.BySeverity == nil || payload.Findings == nil {
		t.Error("the JSON report must always carry the counters and an array, never null")
	}
}

func TestParseValidatesAndNormalizes(t *testing.T) {
	s := testServer(t)
	rec := post(t, s, "/api/parse", sample)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	var back model.Resume
	if err := json.Unmarshal(rec.Body.Bytes(), &back); err != nil {
		t.Fatalf("the answer is not a resume: %v", err)
	}
	if back.Name != "Jean Dupont" || len(back.Experience) != 1 {
		t.Errorf("the round trip lost data: %+v", back)
	}

	// A typo is a hard error, never a silently dropped section.
	rec = post(t, s, "/api/parse", `{"name":"Jean","experiance":[]}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("an unknown field returned %d, want 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "experiance") {
		t.Errorf("the error does not name the offending field: %s", rec.Body)
	}

	// A missing payload is a client error, not a panic.
	rec = post(t, s, "/api/parse", "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("an empty body returned %d, want 400", rec.Code)
	}
}

func TestRenderWritesTheFourDocuments(t *testing.T) {
	s := testServer(t)
	for _, c := range []struct{ format, contentType, magic string }{
		{"docx", docxContentType, "PK\x03\x04"},
		{"pdf", "application/pdf", "%PDF"},
		{"adoc", "text/plain; charset=utf-8", ""},
		{"txt", "text/plain; charset=utf-8", ""},
	} {
		rec := post(t, s, "/api/render/"+c.format, sample)
		if rec.Code != http.StatusOK {
			t.Errorf("%s: status = %d (%s)", c.format, rec.Code, rec.Body)
			continue
		}
		if got := rec.Header().Get("Content-Type"); got != c.contentType {
			t.Errorf("%s: Content-Type = %q, want %q", c.format, got, c.contentType)
		}
		if c.magic != "" && !bytes.HasPrefix(rec.Body.Bytes(), []byte(c.magic)) {
			t.Errorf("%s: the body does not start with %q", c.format, c.magic)
		}
		if cd := rec.Header().Get("Content-Disposition"); !strings.HasPrefix(cd, "attachment;") {
			t.Errorf("%s: Content-Disposition = %q", c.format, cd)
		}
	}

	// The docx must be a real archive, not a zip looking blob.
	rec := post(t, s, "/api/render/docx", sample)
	archive, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	if err != nil {
		t.Fatalf("the docx is not a readable archive: %v", err)
	}
	var found bool
	for _, f := range archive.File {
		if f.Name == "word/document.xml" {
			found = true
			rc, err := f.Open()
			if err != nil {
				t.Fatal(err)
			}
			body, _ := io.ReadAll(rc)
			rc.Close()
			if !strings.Contains(string(body), "Jean Dupont") {
				t.Error("the docx does not hold the name")
			}
			if strings.Contains(string(body), "<w:tbl") {
				t.Error("the docx holds a table, which is hostile to a parser")
			}
		}
	}
	if !found {
		t.Error("the docx has no word/document.xml")
	}
}

// TestRenderRejectsAnUnknownFormat pins the two shapes of an error. A script
// gets a status it can test, htmx gets a fragment it can swap: a 4xx body is
// never injected by htmx, so the editor would show nothing at all.
func TestRenderRejectsAnUnknownFormat(t *testing.T) {
	rec := post(t, testServer(t), "/api/render/xls?format=json", sample)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "unknown format") {
		t.Errorf("body = %s", rec.Body)
	}
	rec = post(t, testServer(t), "/api/render/xls", sample)
	if rec.Code != http.StatusOK {
		t.Errorf("the fragment path returned %d, want 200 so htmx can swap it", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "error") {
		t.Errorf("the fragment shows no error: %s", rec.Body)
	}
}

func TestDownloadNameIsSafe(t *testing.T) {
	for _, c := range []struct{ in, ascii, utf8 string }{
		{"../../etc/passwd", "etcpasswd.docx", "etcpasswd.docx"},
		{`Jean" Dupont`, "Jean Dupont.docx", "Jean%20Dupont.docx"},
		{"   ", "resume.docx", "resume.docx"},
		{"Frédéric", "Fr_d_ric.docx", "Fr%C3%A9d%C3%A9ric.docx"},
	} {
		form := url.Values{"data": {sample}, "name": {c.in}}
		req := httptest.NewRequest(http.MethodPost, "/api/render/docx", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		testServer(t).Handler().ServeHTTP(rec, req)
		got := rec.Header().Get("Content-Disposition")
		if !strings.Contains(got, `filename="`+c.ascii+`"`) {
			t.Errorf("name %q gave %q, want the ASCII name %q", c.in, got, c.ascii)
		}
		if !strings.Contains(got, "filename*=UTF-8''"+c.utf8) {
			t.Errorf("name %q gave %q, want the UTF-8 name %q", c.in, got, c.utf8)
		}
		if strings.Contains(got, "..") || strings.Contains(got, "/") {
			t.Errorf("the header still carries a path: %q", got)
		}
	}
}

func TestGetOnAPostEndpointIsRefused(t *testing.T) {
	rec := get(t, testServer(t), "/api/preview")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

func TestBodyLargerThanTheCapIsRefused(t *testing.T) {
	huge := `{"name":"` + strings.Repeat("a", maxBody) + `"}`
	rec := post(t, testServer(t), "/api/lint?format=json", huge)
	if rec.Code == http.StatusOK {
		t.Error("an oversized body was accepted")
	}
}

func TestListenRefusesAnythingButLoopback(t *testing.T) {
	s := testServer(t)
	for _, addr := range []string{"0.0.0.0:8080", "192.168.1.10:8080", "example.com:8080", "8080"} {
		if err := s.checkLoopback(addr); err == nil {
			t.Errorf("%s was accepted", addr)
		}
	}
	for _, addr := range []string{"127.0.0.1:8080", "localhost:8080", "[::1]:8080"} {
		if err := s.checkLoopback(addr); err != nil {
			t.Errorf("%s was refused: %v", addr, err)
		}
	}
}

func TestSanitizeFilename(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"Jean Dupont", "Jean Dupont"},
		{"a/b\\c", "abc"},
		{"..", "resume"},
		{"CV 2024.", "CV 2024"},
		{"O'Brien", "OBrien"},
		{"a:b*c?d", "abcd"},
	} {
		if got := sanitizeFilename(c.in); got != c.want {
			t.Errorf("sanitizeFilename(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParseAccent(t *testing.T) {
	rgb, err := parseAccent("#1F3864")
	if err != nil {
		t.Fatalf("parseAccent: %v", err)
	}
	if rgb.R != 0x1F || rgb.G != 0x38 || rgb.B != 0x64 {
		t.Errorf("rgb = %+v", rgb)
	}
	// A value coming from a flag or a config file may be padded, that is
	// harmless; a wrong length or a non hexadecimal digit is not.
	if _, err := parseAccent("  #1F3864 "); err != nil {
		t.Errorf("parseAccent of a padded value: %v", err)
	}
	for _, bad := range []string{"", "#", "1F386", "1F38644", "zzzzzz", "rebeccapurple"} {
		if _, err := parseAccent(bad); err == nil {
			t.Errorf("parseAccent(%q) was accepted", bad)
		}
	}
}

// TestThePreviewUsesTheShippedData keeps the editor honest against the real file:
// the sample shipped in the repository must render through the API.
func TestThePreviewUsesTheShippedData(t *testing.T) {
	for _, lang := range []model.Lang{model.LangFR, model.LangEN} {
		opts := DefaultOptions()
		opts.Lang = lang
		s, err := New(opts)
		if err != nil {
			t.Fatal(err)
		}
		resume, err := model.Load("../../data/resume." + string(lang) + ".json")
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		raw, err := json.Marshal(resume)
		if err != nil {
			t.Fatal(err)
		}
		rec := post(t, s, "/api/preview", string(raw))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d", lang, rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, resume.Name) {
			t.Errorf("%s: the preview is missing the name", lang)
		}
		labels := layout.New(resume, layout.Options{Lang: lang}).OptsLang().Labels()
		if !strings.Contains(body, labels.Skills) {
			t.Errorf("%s: the preview is missing the section %q", lang, labels.Skills)
		}
		rec = post(t, s, "/api/lint?format=json", string(raw))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: lint status = %d", lang, rec.Code)
		}
	}
}

// TestPublicIsOptIn pins the rule that made the loopback check worth having: a
// routable address is refused unless the operator asked for it. The flag exists
// for a container behind a proxy, and that is exactly why it must never be the
// default: a typo must not publish the API.
func TestPublicIsOptIn(t *testing.T) {
	loopback, err := New(Options{Addr: "0.0.0.0:8080"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := loopback.checkLoopback("0.0.0.0:8080"); err == nil {
		t.Error("a routable address must be refused without -public")
	}
	if err := loopback.checkLoopback("127.0.0.1:8080"); err != nil {
		t.Errorf("127.0.0.1 refused: %v", err)
	}
	if err := loopback.checkLoopback("[::1]:8080"); err != nil {
		t.Errorf("::1 refused: %v", err)
	}
	if err := loopback.checkLoopback("localhost:8080"); err != nil {
		t.Errorf("localhost refused: %v", err)
	}
	for _, bad := range []string{"0.0.0.0", "192.168.1.10:80", "example.com:80", "nope"} {
		if err := loopback.checkLoopback(bad); err == nil {
			t.Errorf("%q should have been refused", bad)
		}
	}
	// Public lifts the rule, and only that: the handler set is identical, so a
	// public instance serves the same API rather than a reduced one.
	public, err := New(Options{Addr: "0.0.0.0:8080", Public: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if public.opts.Addr != "0.0.0.0:8080" || !public.opts.Public {
		t.Error("Public did not lift the loopback rule")
	}
	rec := get(t, public, "/editor")
	if rec.Code != http.StatusOK {
		t.Errorf("a public instance should serve the editor, got %d", rec.Code)
	}
}

// TestDemoResumeIsServedAndOptional covers the deployed instance: with -demo the
// editor has something to open, without it the editor starts empty and the
// endpoint says so rather than handing out an empty object.
func TestDemoResumeIsServedAndOptional(t *testing.T) {
	plain := get(t, testServer(t), "/api/demo")
	if plain.Code != http.StatusNotFound {
		t.Errorf("without a demo, /api/demo = %d, want 404", plain.Code)
	}

	demo := `{"name":"Démo","headline":"h","lang":"fr","contact":{"city":"N","country":"F",` +
		`"email":"a@example.org","phone":"","links":[]},"summary":"s","experience":[],` +
		`"education":[],"skills":[],"languages":[]}`
	srv, err := New(Options{DemoJSON: demo})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	rec := get(t, srv, "/api/demo")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := rec.Body.String(); got != demo {
		t.Errorf("the demo was altered on the way out: %s", got)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q", ct)
	}
	// A served demo is a copy, not a leak of a draft: the endpoint only ever
	// answers with what the operator put in it.
	if rec.Body.Len() > 2<<20 {
		t.Error("the demo endpoint would serve an oversized body")
	}
}

// TestLandingPageStaysFrench pins a decision that looks like a bug otherwise: the
// landing page is French even when the editor is English. The language in the
// html element is what a screen reader uses to choose its pronunciation rules, so
// it has to describe the text that is actually there.
func TestLandingPageStaysFrench(t *testing.T) {
	s, err := New(Options{
		Lang:     model.LangEN,
		Accent:   "1F3864",
		FontSize: 10.5,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	home := get(t, s, "/").Body.String()
	if !strings.Contains(home, `<html lang="fr"`) {
		t.Error("the landing page announced itself as something else than French")
	}
	if strings.Contains(home, "The tool that turns") {
		t.Error("the landing page was translated: it is written in French on purpose")
	}
	// The editor, on the other hand, follows the flag.
	if editor := get(t, s, "/editor").Body.String(); !strings.Contains(editor, `lang="en"`) {
		t.Error("the editor should follow -lang")
	}
}

// TestStaticAssetsRevalidate covers the trap of embedding the editor in the
// binary. The assets only change with a new binary, which looks like a reason
// to cache them for a long time. It is the opposite: with a long max-age and no
// validator, a browser runs the previous version of the editor for that whole
// window and cannot tell. So the served file must carry a tag a browser can
// check, and an unchanged file must answer 304 with no body.
func TestStaticAssetsRevalidate(t *testing.T) {
	srv := testServer(t)
	rec := get(t, srv, "/static/app.js")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	tag := rec.Header().Get("ETag")
	if tag == "" {
		t.Fatal("app.js is served without an ETag, so a browser cannot revalidate it")
	}
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "no-cache") {
		t.Errorf("Cache-Control = %q, want no-cache so the browser asks again", cc)
	}

	for _, header := range []string{tag, "W/" + tag, "*"} {
		req := httptest.NewRequest(http.MethodGet, "/static/app.js", nil)
		req.Header.Set("If-None-Match", header)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusNotModified {
			t.Errorf("If-None-Match %q: status = %d, want 304", header, rec.Code)
		}
		if rec.Body.Len() != 0 {
			t.Errorf("If-None-Match %q: a 304 must have no body, got %d bytes", header, rec.Body.Len())
		}
	}

	// A tag from another build must bring the file back, not a 304.
	req := httptest.NewRequest(http.MethodGet, "/static/app.js", nil)
	req.Header.Set("If-None-Match", `"0000000000000000"`)
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("a stale ETag gave %d, want 200 with the new file", rec.Code)
	}
	if rec.Body.Len() == 0 {
		t.Error("the new app.js came back empty")
	}
}

// TestStaticETagsAreContentAddressed checks that two assets do not share a tag,
// and that a tag follows the bytes. If the tag were derived from the path alone,
// a rebuilt binary would keep the old tag and the browser would keep the old
// editor, which is the whole bug this replaced.
func TestStaticETagsAreContentAddressed(t *testing.T) {
	srv := testServer(t)
	app := get(t, srv, "/static/app.js").Header().Get("ETag")
	css := get(t, srv, "/static/app.css").Header().Get("ETag")
	if app == "" || css == "" {
		t.Fatal("an asset is served without a tag")
	}
	if app == css {
		t.Errorf("app.js and app.css share the tag %s, so it cannot describe the content", app)
	}
	// Two requests for the same file must agree: the tag is computed once, not
	// per request, and must not drift.
	if again := get(t, srv, "/static/app.js").Header().Get("ETag"); again != app {
		t.Errorf("the tag of app.js changed between two requests: %s then %s", app, again)
	}
}

// TestTheServedEditorCanReorderAchievements is a guard on the feature itself.
// The behaviour lives in the browser and cannot be reached from a Go test, but
// the affordance can: if the buttons disappear from the widget, this fails
// instead of the complaint arriving from a user. "Réalisations" is the French
// label of the highlights field, the one the editor draws with buildListField.
func TestTheServedEditorCanReorderAchievements(t *testing.T) {
	script := get(t, testServer(t), "/static/app.js").Body.String()
	for _, want := range []string{
		"buildListField", // the widget that draws a list of achievements
		"T.moveUp()",     // the move button on each row
		"T.moveDown()",
		"disabled: i === 0",               // no moving the first row above itself
		"disabled: i === list.length - 1", // nor the last row below itself
	} {
		if !strings.Contains(script, want) {
			t.Errorf("the served editor no longer contains %q: the achievements cannot be reordered", want)
		}
	}
	// And the schema still calls those lists "Réalisations", otherwise the test
	// above would pass on a field the user cannot see.
	var published struct {
		Sections []struct {
			Key    string  `json:"key"`
			Fields []Field `json:"fields"`
		} `json:"sections"`
	}
	if err := json.Unmarshal(get(t, testServer(t), "/api/schema").Body.Bytes(), &published); err != nil {
		t.Fatalf("the schema is not readable: %v", err)
	}
	found := 0
	for _, section := range published.Sections {
		for _, f := range section.Fields {
			if f.Key != "highlights" {
				continue
			}
			found++
			if f.Kind != FieldList {
				t.Errorf("%s.highlights has kind %q, want %q for a reorderable list", section.Key, f.Kind, FieldList)
			}
			if f.LabelFR != "Réalisations" {
				t.Errorf("%s.highlights is labelled %q in French", section.Key, f.LabelFR)
			}
		}
	}
	// Experience, projects and activities carry achievements. Fewer would mean
	// one of them lost the field.
	if found < 3 {
		t.Errorf("only %d sections expose a highlights field, expected at least 3", found)
	}
}

// TestTheLayoutIsMobileFirst pins the shape of the layout, not its looks. There
// is no browser in this project's test suite, so the properties that can be
// checked are the ones worth checking: a viewport is declared, no rule undoes
// itself at a max-width, and no column is wider than the narrowest screen the
// app claims to support.
func TestTheLayoutIsMobileFirst(t *testing.T) {
	srv := testServer(t)

	// Without a viewport, a phone lays the page out at 980px and zooms out.
	for _, page := range []string{"/", "/editor"} {
		html := get(t, srv, page).Body.String()
		if !strings.Contains(html, `name="viewport"`) {
			t.Errorf("%s declares no viewport: a phone would render it at 980px", page)
		}
		if !strings.Contains(html, `width=device-width`) {
			t.Errorf("%s has a viewport that does not follow the device width", page)
		}
	}

	// Mobile first means the base rules describe a phone and the media queries
	// only add. A max-width query is the desktop-first spelling, and its return
	// is what makes a layout break on a screen nobody tested.
	for _, name := range []string{"app.css", "home.css"} {
		css := get(t, srv, "/static/"+name).Body.String()
		for _, mq := range regexp.MustCompile(`@media[^{]*max-width`).FindAllString(css, -1) {
			t.Errorf("%s uses a desktop-first query %q: write the phone layout as the base instead", name, strings.TrimSpace(mq))
		}
	}

	// A fixed column wider than a 320px screen, minus the shell padding, would
	// scroll the page sideways. It is only safe inside a media query, which
	// already assumes a wider screen, so only the base rules are checked.
	const narrow = 320 - 48
	track := regexp.MustCompile(`minmax\(\s*(\d+)px\s*,\s*1fr\s*\)`)
	for _, name := range []string{"app.css", "home.css"} {
		css := get(t, srv, "/static/"+name).Body.String()

		// Walk the file keeping a stack of open blocks, flagged for whether each
		// one is a media query. A declaration sits one level below its selector,
		// so the stack is the only way to tell "in the base rules" from "inside a
		// query" without a CSS parser.
		var stack []bool
		var buf strings.Builder
		flush := func(text string, inMedia bool) {
			if inMedia {
				return
			}
			for _, m := range track.FindAllStringSubmatch(text, -1) {
				if w := atoiOrZero(m[1]); w > narrow {
					t.Errorf("%s has a %dpx column in its base rules, wider than the %dpx left on a 320px screen", name, w, narrow)
				}
			}
		}
		inQuery := func() bool {
			for _, isMedia := range stack {
				if isMedia {
					return true
				}
			}
			return false
		}
		for _, r := range css {
			switch {
			case r == '{':
				// The text before the brace is a selector, and it is what says
				// whether the block being opened is a media query.
				sel := buf.String()
				buf.Reset()
				flush(sel, inQuery())
				stack = append(stack, strings.Contains(sel, "@media"))
			case r == '}':
				// The text before a closing brace is a declaration block.
				decls := buf.String()
				buf.Reset()
				flush(decls, inQuery())
				if len(stack) > 0 {
					stack = stack[:len(stack)-1]
				}
			default:
				buf.WriteRune(r)
			}
		}
	}
}

// TestTouchTargetsAreSizedForFingers keeps the pointer-coarse rules in place.
// The alternative is hiding them, which is what the masthead used to do, and a
// page whose only navigation disappears on a phone is not responsive.
func TestTouchTargetsAreSizedForFingers(t *testing.T) {
	for _, name := range []string{"app.css", "home.css"} {
		css := get(t, testServer(t), "/static/"+name).Body.String()
		if !strings.Contains(css, "@media (pointer: coarse)") {
			t.Errorf("%s has no touch rules: buttons and links stay mouse-sized on a phone", name)
		}
	}
	// The landing page navigation must not be hidden on a narrow screen.
	home := get(t, testServer(t), "/static/home.css").Body.String()
	if strings.Contains(home, "display: none") && !strings.Contains(home, "nav") {
		t.Error("the landing page hides something on a narrow screen")
	}
	if regexp.MustCompile(`\.masthead nav\s*\{[^}]*display:\s*none`).MatchString(home) {
		t.Error("the landing page hides its navigation on a phone, leaving no way to reach the sections")
	}
}

func atoiOrZero(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
