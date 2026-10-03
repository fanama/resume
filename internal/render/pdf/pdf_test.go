package pdf

import (
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"

	fpdf "github.com/go-pdf/fpdf"

	"github.com/fanama/resume/atscv/internal/layout"
	"github.com/fanama/resume/atscv/internal/model"
)

func testLayout(lang model.Lang) *layout.Layout {
	r := &model.Resume{
		Lang:     lang,
		Name:     "Jean Dupont",
		Headline: "Développeur Fullstack",
		Contact: model.Contact{
			City: "Paris", Country: "France", Email: "jean@example.com", Phone: "+33 6 12 34 56 78",
			Links: []model.Link{{Label: "GitHub", URL: "https://github.com/fanama"}},
		},
		Summary: "Développeur avec 5 ans d'expérience sur des plateformes de production.",
		Experience: []model.Experience{{
			Title: "Développeur", Company: "SFR", Location: "Paris",
			Start: "2023-01", End: "present",
			Summary: "Équipe R&D appliquée à l'IA générative.",
			Highlights: []string{
				"Développement d'une plateforme LLM utilisée par 1000 utilisateurs par jour",
				"Couverture de tests portée à 70 %",
			},
			Team:  "Product Manager, Product Owner et trois développeurs",
			Stack: []string{"React", "NodeJS", "MongoDB"},
		}},
		Skills: []model.SkillGroup{{Category: "Langages", Items: []string{"Go", "Python", "TypeScript"}}},
	}
	return layout.New(r, layout.Options{Lang: lang})
}

// TestRenderReportIsTheRenderItself pins what "build" relies on: the page
// count comes out of the render that wrote the file. Measuring afterwards
// would run the document again, fonts included, for a number this pass knew.
func TestRenderReportIsTheRenderItself(t *testing.T) {
	l := testLayout(model.LangFR)
	opts := DefaultOptions()

	var buf bytes.Buffer
	rep, err := RenderReport(&buf, l, opts)
	if err != nil {
		t.Fatalf("RenderReport: %v", err)
	}
	if !bytes.HasPrefix(buf.Bytes(), []byte("%PDF-")) {
		t.Fatalf("RenderReport wrote %d bytes that are not a PDF", buf.Len())
	}
	measured, err := Measure(l, opts)
	if err != nil {
		t.Fatalf("Measure: %v", err)
	}
	if rep != measured {
		t.Errorf("report = %+v, want the %+v Measure finds", rep, measured)
	}
	if rep.Pages < 1 {
		t.Errorf("Pages = %d, want at least 1", rep.Pages)
	}
}

// TestTheFontFilesAreReadOnce covers the memoising the renderer does: the
// resolution stats every spelling of the family in every font directory and
// the read is more than a megabyte of TrueType data fpdf parses again. Two
// renders with the same options must cost one resolution and one read per
// file, on a machine that has fonts and on the container, which has none.
func TestTheFontFilesAreReadOnce(t *testing.T) {
	fontDataMu.Lock()
	savedData := fontData
	fontData = map[string][]byte{}
	fontDataMu.Unlock()
	fontPathMu.Lock()
	savedPaths := fontPaths
	fontPaths = map[fontKey][2]string{}
	fontPathMu.Unlock()
	t.Cleanup(func() {
		fontDataMu.Lock()
		fontData = savedData
		fontDataMu.Unlock()
		fontPathMu.Lock()
		fontPaths = savedPaths
		fontPathMu.Unlock()
	})

	l := testLayout(model.LangFR)
	opts := DefaultOptions()
	for i := 0; i < 2; i++ {
		data, err := Bytes(l, opts)
		if err != nil {
			t.Fatalf("render %d: %v", i, err)
		}
		if !bytes.HasPrefix(data, []byte("%PDF-")) {
			t.Fatalf("render %d is not a PDF", i)
		}
	}

	fontDataMu.Lock()
	reads := len(fontData)
	fontDataMu.Unlock()
	if reads > 2 {
		t.Errorf("%d font files were read for two renders, want at most 2: the cache did not hold", reads)
	}
	fontPathMu.Lock()
	resolutions := len(fontPaths)
	fontPathMu.Unlock()
	if resolutions != 1 {
		t.Errorf("%d font resolutions for one set of options, want 1: the stat walk ran again", resolutions)
	}
}

func TestRenderProducesAPDF(t *testing.T) {
	data, err := Bytes(testLayout(model.LangFR), DefaultOptions())
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	if len(data) < 1000 {
		t.Fatalf("the pdf is only %d bytes, it looks empty", len(data))
	}
	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		t.Error("the file does not start with the PDF signature")
	}
	body := string(data)
	for _, want := range []string{"/Type /Page", "/Font", "%%EOF"} {
		if !strings.Contains(body, want) {
			t.Errorf("the pdf is missing %q", want)
		}
	}
	// A resume must not carry a page number or a footer template.
	for _, unwanted := range []string{"Page 1 of", "Page %", "/Annots"} {
		if strings.Contains(body, unwanted) {
			t.Errorf("the pdf contains %q, which pollutes the extracted text", unwanted)
		}
	}
}

// TestEmbeddedFontCarriesToUnicode checks the fidelity of the text layer: an
// embedded TrueType font needs a ToUnicode map, otherwise an extractor returns
// mojibake for every accented character.
func TestEmbeddedFontCarriesToUnicode(t *testing.T) {
	regular, _ := resolveFontFiles(DefaultOptions())
	if regular == "" {
		t.Skip("no TrueType font on this machine, the standard font path is used instead")
	}
	data, err := Bytes(testLayout(model.LangFR), DefaultOptions())
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	body := string(data)
	if !strings.Contains(body, "/ToUnicode") {
		t.Error("the embedded font has no ToUnicode map, accents would not be extractable")
	}
	if !strings.Contains(body, "/FontFile2") {
		t.Error("the TrueType program is not embedded in the file")
	}
}

// TestStandardFontFallback exercises the path taken on a machine without any
// TrueType file: the text is then written in cp1252.
func TestStandardFontFallback(t *testing.T) {
	opts := DefaultOptions()
	opts.FontFamily = "NoSuchFontFamilyOnThisMachine"
	opts.FontDir = t.TempDir()
	regular, bold := resolveFontFiles(opts)
	if regular != "" || bold != "" {
		t.Fatalf("a missing family must not resolve, got %q and %q", regular, bold)
	}
	data, err := Bytes(testLayout(model.LangFR), opts)
	if err != nil {
		t.Fatalf("Bytes with the standard font: %v", err)
	}
	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		t.Error("the fallback did not produce a PDF")
	}
}

func TestCP1252(t *testing.T) {
	cases := map[string]string{
		"ascii":     "ascii",
		"é":         "\xe9",
		"É":         "\xc9",
		"€":         "\x80",
		"’":         "\x92",
		"“quoted”":  "\x93quoted\x94",
		"–":         "\x96",
		"…":         "\x85",
		"日本語":       "???",
		"æ":         "\xe6",
		"œ":         "\x9c",
		"tréma ï":   "tr\xe9ma \xef",
		"tab\there": "tab here",
	}
	for in, want := range cases {
		if got := CP1252(in); got != want {
			t.Errorf("CP1252(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestResolveFontFilesPrefersAnExplicitPath(t *testing.T) {
	opts := DefaultOptions()
	opts.FontFile = "/tmp/explicit-regular.ttf"
	opts.BoldFontFile = "/tmp/explicit-bold.ttf"
	regular, bold := resolveFontFiles(opts)
	if regular != opts.FontFile || bold != opts.BoldFontFile {
		t.Errorf("an explicit path must win, got %q and %q", regular, bold)
	}
}

func TestLookupFindsNothingForAnUnknownFamily(t *testing.T) {
	if path := lookup([]string{t.TempDir()}, "NoSuchFont", false); path != "" {
		t.Errorf("lookup returned %q", path)
	}
}

func TestRenderFailsOnAnUnreadableFont(t *testing.T) {
	opts := DefaultOptions()
	opts.FontFile = "/definitely/not/a/font.ttf"
	if _, err := Bytes(testLayout(model.LangFR), opts); err == nil {
		t.Error("a missing font file must be reported instead of ignored")
	}
}

func TestSectionRulesAreOptional(t *testing.T) {
	// The design draws the hairlines by default, so the reference is the
	// plain variant.
	plainOpts := DefaultOptions()
	plainOpts.SectionRules = false
	plain, err := Bytes(testLayout(model.LangFR), plainOpts)
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	ruled, err := Bytes(testLayout(model.LangFR), DefaultOptions())
	if err != nil {
		t.Fatalf("Bytes with rules: %v", err)
	}
	if len(ruled) <= len(plain) {
		t.Error("a rule under each heading should make the file bigger")
	}
}

// paginationFixture returns the layout of the shipped data: it is the document
// that has to paginate, with three jobs, six skill lines and a section that
// lands on the boundary.
func paginationFixture(t *testing.T) *layout.Layout {
	t.Helper()
	resume, err := model.Load(filepath.Join("..", "..", "..", "data", "resume.fr.json"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return layout.New(resume, layout.Options{Lang: model.LangFR})
}

func traceLayout(t *testing.T, l *layout.Layout) ([]drawn, Report) {
	t.Helper()
	var trace []drawn
	opts := DefaultOptions()
	if err := render(io.Discard, l, opts, &trace); err != nil {
		t.Fatalf("render: %v", err)
	}
	rep, err := Measure(l, opts)
	if err != nil {
		t.Fatalf("Measure: %v", err)
	}
	return trace, rep
}

// straddleFixture builds a resume whose second job is taller than a page, so
// the renderer has to split an entry. The shipped data never does, and a rule
// only a test cannot provoke is a rule nobody knows is broken.
func straddleFixture(t *testing.T, filler int) *layout.Layout {
	t.Helper()
	fillerLines := make([]string, 0, filler)
	for i := range filler {
		fillerLines = append(fillerLines, fmt.Sprintf("Réalisation numéro %d d'un chantier de refonte qui tient sur plusieurs lignes et demande une description complète.", i+1))
	}
	long := make([]string, 0, 90)
	for i := range 90 {
		long = append(long, fmt.Sprintf("Action majeure numéro %d, avec son ampleur, son effet mesuré et la décision qui a suivi.", i+1))
	}
	r := &model.Resume{
		Lang:     model.LangFR,
		Name:     "Jean Dupont",
		Headline: "Développeur",
		Summary:  "Un profil.",
		Experience: []model.Experience{
			{
				Title: "Ingénieur", Company: "Acme", Start: "2020", End: "2022",
				Highlights: fillerLines,
			},
			{
				Title: "Consultant", Company: "Globex", Start: "2018", End: "2020",
				Highlights: long,
			},
		},
	}
	return layout.New(r, layout.Options{Lang: model.LangFR})
}

// TestAnEntryTooTallForAPageKeepsItsHeader checks the rule for the other case:
// an entry taller than a page cannot stay whole, but its title, its dates and
// the first line of its body must not be left alone at the bottom of a page.
func TestAnEntryTooTallForAPageKeepsItsHeader(t *testing.T) {
	_, pageHeight := fpdf.NewCustom(&fpdf.InitType{UnitStr: "mm", SizeStr: "A4"}).GetPageSize()
	usable := pageHeight - 2*pageMargin

	// The rule only shows up when the title of the long entry lands near the
	// foot of a page, so the fixture is walked over every plausible length: a
	// test that only tries one length is a test that passes by luck.
	split := 0
	for filler := 0; filler <= 60; filler++ {
		trace, rep := traceLayout(t, straddleFixture(t, filler))
		if rep.Pages < 2 {
			continue
		}
		for i, d := range trace {
			if d.kind != layout.KindEntryTitle || d.entryHeight <= usable {
				continue // this entry fits on a page, another test covers it
			}
			// The header and the first body line have to share the page, and the
			// body has to actually continue overleaf.
			body := -1
			for j := i + 1; j < len(trace) && body < 0; j++ {
				if trace[j].kind != layout.KindEntryMeta {
					body = j
				}
			}
			if body < 0 {
				t.Fatalf("the entry %q has no body", d.text)
			}
			if trace[body].page != d.page {
				t.Errorf("filler %d: the entry %q opens on the page %d and its body starts on the page %d",
					filler, d.text, d.page, trace[body].page)
			}
			last := i
			for j := i + 1; j < len(trace); j++ {
				if trace[j].kind == layout.KindEntryTitle || trace[j].kind == layout.KindSection {
					break
				}
				last = j
			}
			if trace[last].page == d.page {
				t.Errorf("filler %d: the entry %q should have been split over several pages, it all landed on the page %d",
					filler, d.text, d.page)
			}
			split++
		}
	}
	if split == 0 {
		t.Skip("no filler length put a long entry across a page break")
	}
}

// TestPaginationLeavesNoOrphan is the invariant the reader sees: a page must
// never end on a heading with nothing under it, and never on the title of an
// entry whose body is overleaf. The shipped data is what breaks it.
func TestPaginationLeavesNoOrphan(t *testing.T) {
	trace, _ := traceLayout(t, paginationFixture(t))
	if len(trace) < 2 {
		t.Fatalf("the fixture is too small to paginate: %d blocks", len(trace))
	}
	lastOfPage := map[int]drawn{}
	pages := 0
	for _, d := range trace {
		lastOfPage[d.page] = d
		if d.page > pages {
			pages = d.page
		}
	}
	if pages < 2 {
		t.Skip("the fixture now fits on one page, there is no break to check")
	}
	for page := 1; page < pages; page++ {
		last := lastOfPage[page]
		switch last.kind {
		case layout.KindSection:
			t.Errorf("page %d ends on the orphan heading %q", page, last.text)
		case layout.KindEntryTitle, layout.KindEntryMeta:
			t.Errorf("page %d ends on the entry header %q, its body is overleaf", page, last.text)
		}
	}
}

// TestAnEntryThatFitsStaysOnOnePage pins the rule that keeps a whole job
// description together when it can, which is what makes a two page resume read
// like a document rather than a stream.
func TestAnEntryThatFitsStaysOnOnePage(t *testing.T) {
	trace, _ := traceLayout(t, paginationFixture(t))
	_, pageHeight := fpdf.NewCustom(&fpdf.InitType{UnitStr: "mm", SizeStr: "A4"}).GetPageSize()
	usable := pageHeight - 2*pageMargin

	for i, d := range trace {
		if d.kind != layout.KindEntryTitle {
			continue
		}
		// Walk to the end of the entry.
		last := i
		for j := i + 1; j < len(trace); j++ {
			if trace[j].kind == layout.KindEntryTitle || trace[j].kind == layout.KindSection {
				break
			}
			last = j
		}
		if d.entryHeight <= 0 || d.entryHeight > usable {
			continue // taller than a page: it has to split, on purpose
		}
		for j := i; j <= last; j++ {
			if trace[j].page != d.page {
				t.Errorf("the entry %q is split between the pages %d and %d", d.text, d.page, trace[j].page)
				break
			}
		}
	}
}

// TestTheLastPageIsNotAStub keeps the pagination from solving the orphan problem
// by pushing almost everything onto a second page: a trailing page holding a
// couple of lines is a worse document than a slightly unbalanced first one.
func TestTheLastPageIsNotAStub(t *testing.T) {
	_, rep := traceLayout(t, paginationFixture(t))
	if rep.Pages > 1 {
		if rep.LastPageBlocks < 3 {
			t.Errorf("the last page holds %d blocks", rep.LastPageBlocks)
		}
		if rep.LastPageFill < 8 {
			t.Errorf("the last page is only %.1f%% full", rep.LastPageFill)
		}
		if rep.LastPageFill > 100 {
			t.Errorf("the last page is %.1f%% full, the content overflows it", rep.LastPageFill)
		}
	}
}

// TestMeasureCountsTheRealPages checks the number the CLI prints. It is measured,
// not estimated, so a resume that fits on one page is reported as one.
func TestMeasureCountsTheRealPages(t *testing.T) {
	l := paginationFixture(t)
	rep, err := Measure(l, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if rep.Pages < 1 {
		t.Errorf("pages = %d", rep.Pages)
	}
	if rep.Sections != 6 {
		t.Errorf("sections = %d, want the six of the shipped data", rep.Sections)
	}
	// A shorter document must report fewer pages: a name and a title, and the
	// count has to follow.
	short, err := Measure(layout.New(&model.Resume{
		Lang:     model.LangFR,
		Name:     "Jean Dupont",
		Headline: "Développeur",
	}, layout.Options{Lang: model.LangFR}), DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if short.Pages >= rep.Pages {
		t.Errorf("a short document reported %d pages, the full one %d", short.Pages, rep.Pages)
	}
}
