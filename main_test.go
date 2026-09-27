package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fanama/resume/atscv/internal/model"
	"github.com/fanama/resume/atscv/internal/web"
)

func dataFile(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join("data", name)
}

func runCLI(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	var out, errOut bytes.Buffer
	err := run(args, &out, &errOut)
	return out.String(), errOut.String(), err
}

func TestVersionAndHelp(t *testing.T) {
	out, _, err := runCLI(t, "version")
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	if !strings.Contains(out, "atscv") {
		t.Errorf("version output = %q", out)
	}
	out, _, err = runCLI(t, "help")
	if err != nil {
		t.Fatalf("help: %v", err)
	}
	for _, command := range []string{"build", "lint", "text", "check"} {
		if !strings.Contains(out, command) {
			t.Errorf("help does not mention %q", command)
		}
	}
	if _, _, err := runCLI(t, "nope"); err == nil {
		t.Error("an unknown command must fail")
	}
	if _, _, err := runCLI(t); err == nil {
		t.Error("no command must fail")
	}
}

func TestBuildWritesEveryFormat(t *testing.T) {
	dir := t.TempDir()
	out, _, err := runCLI(t, "build", "-i", dataFile(t, "resume.fr.json"),
		"-o", dir, "-f", "docx,pdf,adoc,txt")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	base := filepath.Join(dir, "resume.fr")
	for _, ext := range []string{".docx", ".pdf", ".adoc", ".txt"} {
		path := base + ext
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if info.Size() == 0 {
			t.Errorf("%s is empty", path)
		}
		if !strings.Contains(out, path) {
			t.Errorf("build did not report %s:\n%s", path, out)
		}
	}
	// A PDF must start with its magic number, a docx with the zip signature.
	if head := head(t, base+".pdf", 5); string(head) != "%PDF-" {
		t.Errorf("the pdf header is %q", head)
	}
	if head := head(t, base+".docx", 2); string(head) != "PK" {
		t.Errorf("the docx header is %q, want a zip archive", head)
	}
	if head := head(t, base+".adoc", 2); !strings.HasPrefix(string(head), "= ") {
		t.Errorf("the adoc must start with its title, got %q", head)
	}
}

func head(t *testing.T, path string, n int) []byte {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()
	buf := make([]byte, n)
	if _, err := f.Read(buf); err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return buf
}

func TestBuildHonoursASingleOutputFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "cv.pdf")
	if _, _, err := runCLI(t, "build", "-i", dataFile(t, "resume.fr.json"), "-o", target, "-f", "pdf"); err != nil {
		t.Fatalf("build: %v", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("%s: %v", target, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("an explicit file must be the only output, got %d", len(entries))
	}
}

func TestBuildIsReproducible(t *testing.T) {
	dir := t.TempDir()
	var first []byte
	for i := 0; i < 2; i++ {
		target := filepath.Join(dir, "cv.docx")
		if _, _, err := runCLI(t, "build", "-i", dataFile(t, "resume.fr.json"), "-o", target, "-f", "docx"); err != nil {
			t.Fatalf("build %d: %v", i, err)
		}
		raw, err := os.ReadFile(target)
		if err != nil {
			t.Fatalf("read %d: %v", i, err)
		}
		if i == 0 {
			first = raw
			continue
		}
		if !bytes.Equal(first, raw) {
			t.Fatal("two builds of the same data differ, the archive is not reproducible")
		}
	}
}

func TestTextAndLanguageFlags(t *testing.T) {
	out, _, err := runCLI(t, "text", "-i", dataFile(t, "resume.fr.json"))
	if err != nil {
		t.Fatalf("text: %v", err)
	}
	if !strings.Contains(out, "Rakotoasimbola") {
		t.Errorf("the plain text preview misses the name:\n%s", out)
	}
	if strings.Index(out, "PROFIL") > strings.Index(out, "EXPÉRIENCE") {
		t.Errorf("the sections are out of order:\n%s", out)
	}

	dir := t.TempDir()
	target := filepath.Join(dir, "en.txt")
	if _, _, err := runCLI(t, "text", "-i", dataFile(t, "resume.fr.json"), "-lang", "en", "-o", target); err != nil {
		t.Fatalf("text -lang: %v", err)
	}
	// The text command prints to stdout, so check the flag through a build.
	if _, _, err := runCLI(t, "build", "-i", dataFile(t, "resume.fr.json"), "-lang", "en", "-o", target, "-f", "txt"); err != nil {
		t.Fatalf("build -lang en: %v", err)
	}
	raw, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(raw), "PROFESSIONAL EXPERIENCE") {
		t.Errorf("-lang en did not switch the headings:\n%s", raw)
	}
	if strings.Contains(string(raw), "Aujourd'hui") {
		t.Errorf("the French present label leaked into the English build:\n%s", raw)
	}
}

func TestAsciiFlagRemovesDiacritics(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "ascii.txt")
	if _, _, err := runCLI(t, "build", "-i", dataFile(t, "resume.fr.json"), "-ascii", "-o", target, "-f", "txt"); err != nil {
		t.Fatalf("build: %v", err)
	}
	raw, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if strings.Contains(string(raw), "É") || strings.Contains(string(raw), "è") {
		t.Errorf("-ascii left a diacritic:\n%s", raw)
	}
	if !strings.Contains(string(raw), "EXPERIENCE") {
		t.Errorf("-ascii did not fold the section heading:\n%s", raw)
	}
}

func TestCheckAndLintExitCodes(t *testing.T) {
	if out, _, err := runCLI(t, "check", "-i", dataFile(t, "resume.fr.json")); err != nil {
		t.Fatalf("check on the shipped data: %v", err)
	} else if !strings.Contains(out, "ok:") {
		t.Errorf("check output = %q", out)
	}
	if out, _, err := runCLI(t, "lint", "-i", dataFile(t, "resume.fr.json")); err != nil {
		t.Fatalf("lint on the shipped data: %v", err)
	} else if !strings.Contains(out, "readability") {
		t.Errorf("lint output = %q", out)
	}

	broken := filepath.Join(t.TempDir(), "broken.json")
	if err := os.WriteFile(broken, []byte(`{"name":"Jean","contact":{}}`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, _, err := runCLI(t, "check", "-i", broken); err == nil {
		t.Error("check must fail on a resume without experience")
	}
	if _, _, err := runCLI(t, "lint", "-i", broken); err == nil {
		t.Error("lint must fail when the report holds a blocking error")
	}
}

func TestInputErrors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope.json")
	if _, _, err := runCLI(t, "build", "-i", missing); err == nil {
		t.Error("a missing input must fail")
	}
	if _, _, err := runCLI(t, "build"); err == nil {
		t.Error("a missing -i must fail")
	}
	typo := filepath.Join(t.TempDir(), "typo.json")
	if err := os.WriteFile(typo, []byte(`{"name":"Jean","experiance":[]}`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, _, err := runCLI(t, "build", "-i", typo); err == nil {
		t.Error("an unknown JSON field must fail instead of dropping the section")
	}
}

func TestFlagValidation(t *testing.T) {
	dir := t.TempDir()
	input := dataFile(t, "resume.fr.json")
	if _, _, err := runCLI(t, "build", "-i", input, "-o", dir, "-f", "xls"); err == nil {
		t.Error("an unknown format must fail")
	}
	if _, _, err := runCLI(t, "build", "-i", input, "-o", dir, "-dates", "sometime"); err == nil {
		t.Error("an unknown date format must fail")
	}
	if _, _, err := runCLI(t, "build", "-i", input, "-o", dir, "-lang", "de"); err == nil {
		t.Error("an unsupported language must fail")
	}
	if _, _, err := runCLI(t, "build", "-i", input, "-o", dir, "-accent", "12345"); err == nil {
		t.Error("a malformed color must fail")
	}
	if _, _, err := runCLI(t, "build", "-i", input, "-o", dir, "-dates", "month", "-accent", "#1F3864", "-rules"); err != nil {
		t.Errorf("valid flags were rejected: %v", err)
	}
}

func TestParseFormats(t *testing.T) {
	got, err := parseFormats("docx, PDF ,adoc")
	if err != nil {
		t.Fatalf("parseFormats: %v", err)
	}
	if strings.Join(got, ",") != "docx,pdf,adoc" {
		t.Errorf("parseFormats = %v", got)
	}
	if _, err := parseFormats(""); err == nil {
		t.Error("an empty format list must fail")
	}
	if _, err := parseFormats("docx,doc"); err == nil {
		t.Error("an unknown format must fail")
	}
}

func TestParseAccent(t *testing.T) {
	hex, rgb, err := parseAccent("#1F3864")
	if err != nil {
		t.Fatalf("parseAccent: %v", err)
	}
	if hex != "1F3864" {
		t.Errorf("hex = %q", hex)
	}
	if rgb.R != 0x1F || rgb.G != 0x38 || rgb.B != 0x64 {
		t.Errorf("rgb = %+v", rgb)
	}
	// Without -accent the headings keep the default navy: a color is what gives
	// the document a hierarchy, and it never reaches the extracted text.
	hex, rgb, err = parseAccent("")
	if err != nil {
		t.Fatalf("parseAccent(\"\"): %v", err)
	}
	if hex != defaultAccentHex {
		t.Errorf("default hex = %q, want %q", hex, defaultAccentHex)
	}
	if rgb.R == 0 && rgb.G == 0 && rgb.B == 0 {
		t.Error("the default accent must not be black")
	}
	if _, _, err := parseAccent("12345"); err == nil {
		t.Error("a malformed color must fail")
	}
}

func TestParseDateFormat(t *testing.T) {
	for _, in := range []string{"", "numeric", "NUMERIC"} {
		if got, err := parseDateFormat(in); err != nil || got != "numeric" {
			t.Errorf("parseDateFormat(%q) = %q, %v", in, got, err)
		}
	}
	if got, err := parseDateFormat("month"); err != nil || got != "month" {
		t.Errorf("parseDateFormat(month) = %q, %v", got, err)
	}
	if got, err := parseDateFormat("year"); err != nil || got != "year" {
		t.Errorf("parseDateFormat(year) = %q, %v", got, err)
	}
	if _, err := parseDateFormat("quarter"); err == nil {
		t.Error("an unknown date format must fail")
	}
}

func TestCollectKeywordsIsSortedAndUnique(t *testing.T) {
	r := &model.Resume{
		Headline: "Développeur Fullstack LLM",
		Experience: []model.Experience{
			{Title: "Développeur", Stack: []string{"Go", "C++", "go"}},
		},
		Skills: []model.SkillGroup{
			{Category: "Langages", Items: []string{"Python", "Go"}},
		},
	}
	words := collectKeywords(r)
	if len(words) == 0 {
		t.Fatal("no keyword collected")
	}
	seen := map[string]bool{}
	for i, w := range words {
		key := strings.ToLower(w)
		if seen[key] {
			t.Errorf("duplicate keyword %q", w)
		}
		seen[key] = true
		if i > 0 && strings.ToLower(words[i-1]) > key {
			t.Errorf("keywords are not sorted: %q before %q", words[i-1], w)
		}
		if len(w) < 2 {
			t.Errorf("a one character keyword %q was kept", w)
		}
	}
	for _, want := range []string{"Go", "Python", "Fullstack"} {
		if !seen[strings.ToLower(want)] {
			t.Errorf("missing keyword %q in %v", want, words)
		}
	}
}

// TestPageCountIsMeasuredAndReported pins the number the user acts on. The
// shipped data needs two pages, and a silent second page is how a writer misses
// it: build has to say so, with the size of the cut a single page would take.
func TestPageCountIsMeasuredAndReported(t *testing.T) {
	fr := dataFile(t, "resume.fr.json")

	out, _, err := runCLI(t, "check", "-i", fr)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if !strings.Contains(out, "2 pages") {
		t.Errorf("check did not report the two pages of the shipped data: %q", out)
	}
	if strings.Contains(out, "about") {
		t.Errorf("check still estimates instead of measuring: %q", out)
	}
	if !strings.Contains(out, "one") {
		t.Errorf("check did not say what fitting on one page would cost: %q", out)
	}

	dir := t.TempDir()
	_, errOut, err := runCLI(t, "build", "-i", fr, "-o", filepath.Join(dir, "cv.pdf"))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !strings.Contains(errOut, "2 pages") {
		t.Errorf("build did not warn about the second page: %q", errOut)
	}

	// A short resume fits, and then nothing is said: a warning that fires on a
	// one page document is noise.
	raw, err := os.ReadFile(fr)
	if err != nil {
		t.Fatal(err)
	}
	var resume map[string]any
	if err := json.Unmarshal(raw, &resume); err != nil {
		t.Fatal(err)
	}
	resume["experience"] = resume["experience"].([]any)[:1]
	resume["activities"] = []any{}
	short := filepath.Join(t.TempDir(), "short.json")
	body, err := json.Marshal(resume)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(short, body, 0o644); err != nil {
		t.Fatal(err)
	}
	if out, _, err := runCLI(t, "check", "-i", short); err != nil {
		t.Fatalf("check on the short resume: %v", err)
	} else if !strings.Contains(out, "1 page") {
		t.Errorf("a one page resume was reported as %q", out)
	}
	_, errOut, err = runCLI(t, "build", "-i", short, "-o", filepath.Join(dir, "short.pdf"))
	if err != nil {
		t.Fatalf("build on the short resume: %v", err)
	}
	if strings.Contains(errOut, "note:") {
		t.Errorf("a one page document produced a warning: %q", errOut)
	}
}

// TestLandingPageShowsRealOutput ties the marketing claims to the tool. The
// landing page pastes command output, and pasted output rots: a rule is added,
// the numbers move, and the page keeps promising something the binary stopped
// doing. Worse, a sample copied from a real resume publishes an employer name
// and a word count that fingerprints it. So the samples are produced by
// data/resume.demo.json, a fictional resume, and this test runs the commands.
func TestLandingPageShowsRealOutput(t *testing.T) {
	demo := dataFile(t, "resume.demo.json")

	var checkOut string
	if out, _, err := runCLI(t, "check", "-i", demo); err != nil {
		t.Fatalf("check on the demo: %v", err)
	} else {
		checkOut = strings.TrimSpace(out)
	}

	var lintOut string
	if out, _, err := runCLI(t, "lint", "-i", demo); err != nil {
		t.Fatalf("lint on the demo: %v", err)
	} else {
		// Only the summary line: the findings depend on the fixture.
		for _, line := range strings.Split(out, "\n") {
			if strings.HasPrefix(line, "readability") {
				lintOut = strings.TrimSpace(line)
			}
		}
		if lintOut == "" {
			t.Fatalf("lint printed no summary line:\n%s", out)
		}
	}

	srv, err := web.New(web.Options{})
	if err != nil {
		t.Fatalf("web.New: %v", err)
	}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET / = %d", rec.Code)
	}
	body := rec.Body.String()

	for _, want := range []string{checkOut, lintOut} {
		if !strings.Contains(body, want) {
			t.Errorf("the landing page does not show the real output %q", want)
		}
	}

	// Nothing from the real resume may reach a page meant to be public.
	raw, err := os.ReadFile(dataFile(t, "resume.fr.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{"Cliris", "Dasia", "Rakotoasimbola", "rakotoasimbola"} {
		if strings.Contains(body, leak) {
			t.Errorf("the landing page publishes %q from the real resume", leak)
		}
	}
	if len(raw) == 0 {
		t.Fatal("the real resume is empty")
	}
}

// TestHealthcheckProbesTheRunningServer covers the container probe. The image
// has no shell and no curl, so the binary asks itself: a failure here means a
// deployed instance is restarted for no reason, or not restarted when it should.
func TestHealthcheckProbesTheRunningServer(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"status":"ok"}`)
	})
	go func() { _ = http.Serve(ln, mux) }()
	t.Cleanup(func() { _ = ln.Close() })

	addr := ln.Addr().String()
	t.Setenv("ATSCV_ADDR", addr)
	if _, _, err := runCLI(t, "healthcheck"); err != nil {
		t.Errorf("healthcheck against the running server: %v", err)
	}

	// A closed port must fail, or the probe would always answer ok.
	_ = ln.Close()
	if _, _, err := runCLI(t, "healthcheck"); err == nil {
		t.Error("healthcheck succeeded on a closed port")
	}
	if _, _, err := runCLI(t, "healthcheck", "extra"); err == nil {
		t.Error("healthcheck accepted an argument")
	}
}

// TestServeReadsTheEnvironment pins the container configuration path. A
// platform injects a port and a hostname, not a command line, and the probe
// reads the same variable to find the port.
func TestServeReadsTheEnvironment(t *testing.T) {
	t.Setenv("ATSCV_ADDR", "127.0.0.1:9911")
	t.Setenv("ATSCV_LANG", "en")
	t.Setenv("ATSCV_SIZE", "12")
	if addr := serveAddr(); addr != "127.0.0.1:9911" {
		t.Errorf("serveAddr() = %q, want the value of ATSCV_ADDR", addr)
	}
	// ATSCV_ADDR wins over PORT: an operator who names the address means it.
	t.Setenv("PORT", "10000")
	if addr := serveAddr(); addr != "127.0.0.1:9911" {
		t.Errorf("serveAddr() = %q, PORT should not override ATSCV_ADDR", addr)
	}
	// PORT alone decides the port, and nothing else. Every hosted platform
	// injects it, and binding the wrong port means the proxy talks to a closed
	// port. It is not read as permission to be public: that stays -public.
	t.Setenv("ATSCV_ADDR", "")
	if addr := serveAddr(); addr != "0.0.0.0:10000" {
		t.Errorf("serveAddr() = %q, want 0.0.0.0:10000", addr)
	}
	t.Setenv("PORT", "")
	if addr := serveAddr(); addr != "127.0.0.1:8080" {
		t.Errorf("serveAddr() = %q, want the loopback default", addr)
	}
	if v := envOrFloat("ATSCV_SIZE", 10.5); v != 12 {
		t.Errorf("envOrFloat = %v, want 12", v)
	}
	if v := envOr("ATSCV_LANG", "fr"); v != "en" {
		t.Errorf("envOr = %q, want en", v)
	}
	// An unparseable number falls back rather than crashing the server.
	t.Setenv("ATSCV_SIZE", "grand")
	if v := envOrFloat("ATSCV_SIZE", 10.5); v != 10.5 {
		t.Errorf("envOrFloat on garbage = %v, want the default", v)
	}
}
