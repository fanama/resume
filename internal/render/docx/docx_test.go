package docx

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"strings"
	"testing"

	"github.com/fanama/resume/atscv/internal/layout"
)

func testLayout() *layout.Layout {
	return &layout.Layout{
		Blocks: []layout.Block{
			{Kind: layout.KindName, Text: "Jean Dupont"},
			{Kind: layout.KindHeadline, Text: "Développeur Go"},
			{Kind: layout.KindContact, Text: "Paris | jean@example.com"},
			{Kind: layout.KindContact, Text: "github.com/jeandupont"},
			{Kind: layout.KindSection, Text: "PROFIL"},
			{Kind: layout.KindSummary, Text: "Développeur backend avec 5 ans d'expérience."},
			{Kind: layout.KindSection, Text: "EXPÉRIENCE PROFESSIONNELLE"},
			{Kind: layout.KindEntryTitle, Text: "Développeur", Dim: "Entreprise"},
			{Kind: layout.KindEntryMeta, Text: "Entreprise, Paris | 2020 - 2024"},
			{Kind: layout.KindBullet, Text: "Migration de 3 services vers Go"},
			{Kind: layout.KindField, Bold: "Technologies", Text: "Go, PostgreSQL"},
		},
	}
}

func build(t *testing.T, l *layout.Layout) []byte {
	t.Helper()
	data, err := Bytes(FromLayout(l), DefaultOptions())
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	return data
}

func parts(t *testing.T, data []byte) map[string]string {
	t.Helper()
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("not a valid zip: %v", err)
	}
	out := make(map[string]string, len(r.File))
	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", f.Name, err)
		}
		raw, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("read %s: %v", f.Name, err)
		}
		out[f.Name] = string(raw)
	}
	return out
}

func TestPackageLayout(t *testing.T) {
	p := parts(t, build(t, testLayout()))
	for _, name := range []string{
		"[Content_Types].xml",
		"_rels/.rels",
		"docProps/core.xml",
		"word/document.xml",
		"word/styles.xml",
		"word/numbering.xml",
		"word/settings.xml",
		"word/_rels/document.xml.rels",
	} {
		if _, ok := p[name]; !ok {
			t.Errorf("missing package part %q", name)
		}
	}
}

func TestEveryPartIsWellFormedXML(t *testing.T) {
	for name, body := range parts(t, build(t, testLayout())) {
		dec := xml.NewDecoder(strings.NewReader(body))
		for {
			_, err := dec.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Errorf("%s is not well-formed XML: %v", name, err)
				break
			}
		}
	}
}

// TestNoATSForbiddenStructures is the regression guard for the whole point of
// this generator: an ATS reads document.xml sequentially, so tables, columns,
// text boxes, drawings, headers and footers destroy the reading order.
func TestNoATSForbiddenStructures(t *testing.T) {
	forbidden := []string{
		"<w:tbl", "<w:txbxContent", "<w:drawing", "<w:pict", "<w:object",
		"<w:headerReference", "<w:footerReference", "<w:hyperlink",
		"<w:fldSimple", "<w:instrText", "<w:sdt", "<w:framePr",
		`w:num="2"`, // a two column section
	}
	for name, body := range parts(t, build(t, testLayout())) {
		if !strings.HasSuffix(name, ".xml") {
			continue
		}
		for _, tag := range forbidden {
			if strings.Contains(body, tag) {
				t.Errorf("%s contains ATS-hostile markup %q", name, tag)
			}
		}
	}
}

func TestPageIsA4WithSingleColumnAndRealBullets(t *testing.T) {
	p := parts(t, build(t, testLayout()))
	doc := p["word/document.xml"]
	if !strings.Contains(doc, `w:pgSz w:w="11906" w:h="16838"`) {
		t.Error("page size is not A4 (11906x16838 twips)")
	}
	if !strings.Contains(doc, `<w:cols w:space="708"/>`) {
		t.Error("the single column section properties are missing")
	}
	if !strings.Contains(doc, `<w:pStyle w:val="`+StyleBullet+`"/>`) {
		t.Error("the bullet style is unused")
	}
	if !strings.Contains(p["word/numbering.xml"], `<w:num w:numId="1">`) {
		t.Error("the real Word bullet list definition is missing")
	}
	for _, id := range []string{StyleName, StyleHeadline, StyleContact, StyleSection,
		StyleEntry, StyleEntryMeta, StyleSummary, StyleBullet, StyleField} {
		if !strings.Contains(p["word/styles.xml"], `w:styleId="`+id+`"`) {
			t.Errorf("style %s is not declared", id)
		}
	}
}

// pPrOrder and rPrOrder are the child sequences the OOXML schema fixes for
// w:pPr and w:rPr. Word refuses a part whose children are out of order, and
// refuses it silently enough that it looks like a styling bug, so the order is
// asserted here rather than discovered in a recruiter's inbox.
var pPrOrder = []string{
	"pStyle", "keepNext", "keepLines", "pageBreakBefore", "framePr", "widowControl",
	"numPr", "suppressLineNumbers", "pBdr", "shd", "tabs", "suppressAutoHyphens",
	"kinsoku", "wordWrap", "overflowPunct", "topLinePunct", "autoSpaceDE",
	"autoSpaceDN", "bidi", "adjustRightInd", "snapToGrid", "spacing", "ind",
	"contextualSpacing", "mirrorIndents", "suppressOverlap", "jc", "textDirection",
	"textAlignment", "textboxTightWrap", "outlineLvl", "divId", "cnfStyle", "rPr",
	"sectPr", "pPrChange",
}

var rPrOrder = []string{
	"rStyle", "rFonts", "b", "bCs", "i", "iCs", "caps", "smallCaps", "strike",
	"dstrike", "outline", "shadow", "emboss", "imprint", "noProof", "snapToGrid",
	"vanish", "webHidden", "color", "spacing", "w", "kern", "position", "sz",
	"szCs", "highlight", "u", "effect", "bdr", "shd", "fitText", "vertAlign",
	"rtl", "cs", "em", "lang", "eastAsianLayout", "specVanish", "oMath",
}

func TestPropertyChildrenFollowTheSchemaOrder(t *testing.T) {
	for name, body := range parts(t, build(t, testLayout())) {
		if !strings.HasSuffix(name, ".xml") {
			continue
		}
		for _, check := range []struct {
			container string
			order     []string
		}{{"pPr", pPrOrder}, {"rPr", rPrOrder}} {
			for _, block := range between(body, "<w:"+check.container+">", "</w:"+check.container+">") {
				var kids []string
				for _, tag := range childTags(block) {
					if index(check.order, tag) >= 0 {
						kids = append(kids, tag)
					}
				}
				for i := 1; i < len(kids); i++ {
					if index(check.order, kids[i-1]) > index(check.order, kids[i]) {
						t.Errorf("%s: <w:%s> children out of schema order: %v", name, check.container, kids)
						break
					}
				}
			}
		}
	}
}

// between returns the content of every <open>...</open> block.
func between(s, open, close string) []string {
	var out []string
	rest := s
	for {
		start := strings.Index(rest, open)
		if start < 0 {
			return out
		}
		rest = rest[start+len(open):]
		end := strings.Index(rest, close)
		if end < 0 {
			return out
		}
		out = append(out, rest[:end])
		rest = rest[end+len(close):]
	}
}

// childTags returns the w: names of the direct children of an XML fragment.
func childTags(block string) []string {
	var tags []string
	depth := 0
	for i := 0; i < len(block); {
		switch {
		case strings.HasPrefix(block[i:], "<w:"):
			end := strings.IndexAny(block[i:], " />")
			if end < 0 {
				return tags
			}
			name := block[i+3 : i+end]
			selfClosing := block[i+end-1] == '/'
			if depth == 0 {
				tags = append(tags, name)
			}
			if selfClosing {
				i += end + 1
				continue
			}
			depth++
			i += end + 1
		case strings.HasPrefix(block[i:], "</w:"):
			depth--
			i += len("</w:") + 1
			for i < len(block) && block[i] != '>' {
				i++
			}
			i++
		case block[i] == '>':
			i++
		default:
			i++
		}
	}
	return tags
}

func index(list []string, want string) int {
	for i, v := range list {
		if v == want {
			return i
		}
	}
	return -1
}

// TestDesignIsMadeOfSafePrimitives checks that the visual identity is built
// only from properties a text extractor ignores: colors, sizes, paragraph
// borders and character spacing.
func TestDesignIsMadeOfSafePrimitives(t *testing.T) {
	p := parts(t, build(t, testLayout()))
	styles, doc := p["word/styles.xml"], p["word/document.xml"]

	// The accent reaches the name and the section headings; the field labels
	// get it per run, in document.xml.
	if n := strings.Count(styles, `<w:color w:val="1F3864"/>`); n < 2 {
		t.Errorf("the accent color appears %d times in styles.xml, expected at least 2", n)
	}
	// A hairline is a paragraph border, never a drawing.
	if !strings.Contains(styles, `<w:pBdr><w:bottom`) {
		t.Error("the section heading lost its hairline")
	}
	if strings.Contains(doc, "<w:drawing") || strings.Contains(doc, "<w:pict") {
		t.Error("a design element was drawn as a shape instead of a property")
	}
	// Only the first section carries the rule under the header block.
	if n := strings.Count(doc, "<w:pBdr>"); n != 1 {
		t.Errorf("%d top rules in document.xml, want exactly 1", n)
	}
}

func TestLabelRunCarriesTheAccentColor(t *testing.T) {
	var found bool
	for _, p := range FromLayout(testLayout()) {
		for _, r := range p.Runs {
			if r.Label && strings.HasSuffix(r.Text, ": ") {
				found = true
			}
		}
	}
	if !found {
		t.Error("no field label is marked as a label, the accent would be lost")
	}
	doc := parts(t, build(t, testLayout()))["word/document.xml"]
	if !strings.Contains(doc, `<w:color w:val="1F3864"/>`) {
		t.Error("the label color is missing from document.xml")
	}
}

func TestTextIsEscapedAndReadable(t *testing.T) {
	doc := parts(t, build(t, &layout.Layout{Blocks: []layout.Block{
		{Kind: layout.KindName, Text: "Ampersand & Co <Ltd>"},
	}}))["word/document.xml"]
	if !strings.Contains(doc, "Ampersand &amp; Co &lt;Ltd&gt;") {
		t.Errorf("text is not escaped in document.xml: %s", doc)
	}
	if got := Escape("a & b < c > d \"e\""); strings.Contains(got, "a & b") {
		t.Errorf("Escape left raw characters: %s", got)
	}
}

func TestEntryTitleKeepsOrganizationInTheReadingOrder(t *testing.T) {
	var text []string
	for _, p := range FromLayout(testLayout()) {
		for _, r := range p.Runs {
			text = append(text, strings.TrimSpace(r.Text))
		}
	}
	joined := strings.Join(text, " ")
	for _, want := range []string{"Jean Dupont", "PROFIL", "Entreprise, Paris | 2020 - 2024", "Technologies: Go, PostgreSQL"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in %q", want, joined)
		}
	}
	if i, j := strings.Index(joined, "PROFIL"), strings.Index(joined, "Entreprise, Paris"); i > j {
		t.Error("the section heading must precede its entries")
	}
}

// TestAnEntryIsKeptWholeButNotBoundToTheNext is the .docx half of the PDF
// pagination rules: an experience that fits on a page stays whole, and the
// entry that follows it is free to start on the next one. The second half is
// the one that bites: keepNext on the last paragraph of an entry would tie the
// two together, and a document of short entries would then push itself down
// the page to stay with itself.
func TestAnEntryIsKeptWholeButNotBoundToTheNext(t *testing.T) {
	l := &layout.Layout{Blocks: []layout.Block{
		{Kind: layout.KindSection, Text: "EXPERIENCE"},
		{Kind: layout.KindEntryTitle, Text: "Développeur", Dim: "Entreprise"},
		{Kind: layout.KindEntryMeta, Text: "Paris | 2020 - 2024"},
		{Kind: layout.KindBullet, Text: "Une réalisation"},
		{Kind: layout.KindBullet, Text: "Une autre"},
		{Kind: layout.KindEntryTitle, Text: "Ingénieur", Dim: "Autre"},
		{Kind: layout.KindEntryMeta, Text: "Lyon | 2018 - 2020"},
		{Kind: layout.KindField, Bold: "Technologies", Text: "Go"},
	}}
	var keepNext []bool
	for _, p := range FromLayout(l) {
		keepNext = append(keepNext, p.KeepNext)
	}
	// Section heading and first entry: the heading keeps with what follows, and
	// the first entry keeps whole, down to its last paragraph.
	want := []bool{false, true, true, true, false, true, true, false}
	if len(keepNext) != len(want) {
		t.Fatalf("got %d paragraphs, want %d", len(keepNext), len(want))
	}
	for i := range want {
		if keepNext[i] != want[i] {
			t.Errorf("paragraph %d (%s): keepNext = %v, want %v",
				i, styleOf(t, l, i), keepNext[i], want[i])
		}
	}
	doc := parts(t, build(t, l))["word/document.xml"]
	if n := strings.Count(doc, "<w:keepNext/>"); n != 5 {
		t.Errorf("document.xml carries %d keepNext, want 5", n)
	}
	// The last paragraph of the document must never hold the chain open, or
	// Word keeps looking for a next paragraph to stay with.
	if strings.Contains(doc, `<w:pStyle w:val="ATSField"/><w:keepNext/>`) {
		t.Error("the last paragraph of the last entry is bound to nothing")
	}
}

func styleOf(t *testing.T, l *layout.Layout, i int) string {
	t.Helper()
	return FromLayout(l)[i].Style
}
