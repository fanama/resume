// Package docx writes an ATS-safe .docx file.
//
// The document is deliberately built by hand: the package is a plain zip
// archive of a handful of OOXML parts, with no third-party dependency. It
// contains no table, no text box, no column, no image, no header and no
// footer, so that the reading order an Applicant Tracking System extracts is
// exactly the reading order of the resume.
package docx

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// Options configures the .docx output.
type Options struct {
	// FontFamily is the typeface of the whole document. Calibri and Arial are
	// the two safest choices for parsers and for recruiters' machines.
	FontFamily string
	// FontSize is the body size in points.
	FontSize float64
	// SectionColor is the accent color (RRGGBB) used for the section headings,
	// the headline and the field labels. It is a color, not a layout change: a
	// text extractor never sees it.
	SectionColor string
	// SectionRules draws a hairline under the section headings and one under
	// the header block. They are paragraph borders, which no ATS mistakes for
	// content.
	SectionRules bool
	// LangTag is the document language, "fr-FR" or "en-US".
	LangTag string
	// Author, Title and Keywords fill the document properties, which some
	// parsers read instead of the visible text.
	Author   string
	Title    string
	Keywords string
}

// DefaultOptions returns the options used when the CLI passes none.
func DefaultOptions() Options {
	return Options{
		FontFamily:   "Calibri",
		FontSize:     10.5,
		SectionColor: defaultAccent,
		SectionRules: true,
		LangTag:      "fr-FR",
		Title:        "Resume",
	}
}

// defaultAccent is the navy used for the headings when no color is given.
const defaultAccent = "1F3864"

// Run is a styled piece of text inside a paragraph.
type Run struct {
	Text string
	Bold bool
	Dim  bool
	// Label marks a "Label: " prefix. It is bold and painted with the accent
	// color, which is what makes a dense list of technologies scannable.
	Label bool
}

// Paragraph is one paragraph of the document, addressed by style.
type Paragraph struct {
	Style string
	Runs  []Run
	// RuleTop draws a hairline above the paragraph. It is used once, under the
	// header block, to separate the identity from the content.
	RuleTop bool
	// KeepNext keeps the paragraph on the same page as the one that follows
	// it. An entry that fits on a page must not be split across two, and the
	// last paragraph of an entry must not carry it, or two entries would be
	// bound to each other and a page would empty itself to keep them together.
	KeepNext bool
}

// Line builds a single-run paragraph.
func Line(style, text string) Paragraph {
	return Paragraph{Style: style, Runs: []Run{{Text: text}}}
}

// Labeled builds a "Label: value" paragraph where only the label is accented.
func Labeled(style, label, value string) Paragraph {
	return Paragraph{Style: style, Runs: []Run{{Text: label + ": ", Label: true}, {Text: value}}}
}

// Render writes a .docx file made of the given paragraphs.
func Render(w io.Writer, paragraphs []Paragraph, opts Options) error {
	if opts.FontFamily == "" {
		opts.FontFamily = "Calibri"
	}
	if opts.FontSize <= 0 {
		opts.FontSize = 10.5
	}
	if opts.LangTag == "" {
		opts.LangTag = "fr-FR"
	}
	if opts.SectionColor == "" {
		opts.SectionColor = defaultAccent
	}
	if opts.Title == "" {
		opts.Title = "Resume"
	}

	parts := []struct {
		name string
		body string
	}{
		{"[Content_Types].xml", contentTypes()},
		{"_rels/.rels", rootRels()},
		{"docProps/core.xml", coreProps(opts)},
		{"docProps/app.xml", appProps(opts)},
		{"word/_rels/document.xml.rels", documentRels()},
		{"word/document.xml", documentXML(paragraphs, opts)}, {"word/styles.xml", stylesXML(opts)},
		{"word/numbering.xml", numberingXML()},
		{"word/settings.xml", settingsXML(opts)},
		{"word/fontTable.xml", fontTableXML(opts.FontFamily)},
	}

	// A fixed timestamp keeps the archive byte-for-byte reproducible, which
	// makes it reviewable in git.
	modTime := time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC)

	zw := zip.NewWriter(w)
	for _, p := range parts {
		hdr := &zip.FileHeader{Name: p.name, Method: zip.Deflate, Modified: modTime}
		fw, err := zw.CreateHeader(hdr)
		if err != nil {
			return fmt.Errorf("docx: create %s: %w", p.name, err)
		}
		if _, err := io.WriteString(fw, p.body); err != nil {
			return fmt.Errorf("docx: write %s: %w", p.name, err)
		}
	}
	if err := zw.Close(); err != nil {
		return fmt.Errorf("docx: close archive: %w", err)
	}
	return nil
}

// Bytes renders a .docx into memory.
func Bytes(paragraphs []Paragraph, opts Options) ([]byte, error) {
	var buf bytes.Buffer
	if err := Render(&buf, paragraphs, opts); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

const xmlHeader = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n"

// Escape makes a string safe for an XML text node and drops the control
// characters that XML 1.0 forbids.
func Escape(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		case '\'':
			b.WriteString("&apos;")
		case '\t', '\n', '\r':
			b.WriteRune(' ')
		default:
			if r < 0x20 {
				continue
			}
			b.WriteRune(r)
		}
	}
	return b.String()
}

func hp(points float64) string {
	// OOXML stores font sizes in half-points.
	return strconv.Itoa(int(points*2 + 0.5))
}

func contentTypes() string {
	return xmlHeader +
		`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
		`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
		`<Default Extension="xml" ContentType="application/xml"/>` +
		`<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>` +
		`<Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/>` +
		`<Override PartName="/word/numbering.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.numbering+xml"/>` +
		`<Override PartName="/word/settings.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.settings+xml"/>` +
		`<Override PartName="/word/fontTable.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.fontTable+xml"/>` +
		`<Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/>` +
		`<Override PartName="/docProps/app.xml" ContentType="application/vnd.openxmlformats-officedocument.extended-properties+xml"/>` +
		`</Types>`
}

func rootRels() string {
	return xmlHeader +
		`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
		`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>` +
		`<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/>` +
		`<Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/extended-properties" Target="docProps/app.xml"/>` +
		`</Relationships>`
}

func documentRels() string {
	return xmlHeader +
		`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
		`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>` +
		`<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/numbering" Target="numbering.xml"/>` +
		`<Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/settings" Target="settings.xml"/>` +
		`<Relationship Id="rId4" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/fontTable" Target="fontTable.xml"/>` +
		`</Relationships>`
}

func coreProps(opts Options) string {
	stamp := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	var b strings.Builder
	b.WriteString(xmlHeader)
	b.WriteString(`<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties"`)
	b.WriteString(` xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:dcterms="http://purl.org/dc/terms/"`)
	b.WriteString(` xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">`)
	b.WriteString(`<dc:title>` + Escape(opts.Title) + `</dc:title>`)
	b.WriteString(`<dc:creator>` + Escape(opts.Author) + `</dc:creator>`)
	if opts.Keywords != "" {
		b.WriteString(`<cp:keywords>` + Escape(opts.Keywords) + `</cp:keywords>`)
	}
	b.WriteString(`<dc:language>` + Escape(opts.LangTag) + `</dc:language>`)
	b.WriteString(`<cp:lastModifiedBy>` + Escape(opts.Author) + `</cp:lastModifiedBy>`)
	b.WriteString(`<dcterms:created xsi:type="dcterms:W3CDTF">` + stamp + `</dcterms:created>`)
	b.WriteString(`<dcterms:modified xsi:type="dcterms:W3CDTF">` + stamp + `</dcterms:modified>`)
	b.WriteString(`</cp:coreProperties>`)
	return b.String()
}

func appProps(opts Options) string {
	return xmlHeader +
		`<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties"` +
		` xmlns:vt="http://schemas.openxmlformats.org/officeDocument/2006/docPropsVTypes">` +
		`<Application>atscv</Application>` +
		`<Company>` + Escape(opts.Author) + `</Company>` +
		`<DocSecurity>0</DocSecurity>` +
		`<ScaleCrop>false</ScaleCrop>` +
		`<LinksUpToDate>false</LinksUpToDate>` +
		`<SharedDoc>false</SharedDoc>` +
		`<HyperlinksChanged>false</HyperlinksChanged>` +
		`</Properties>`
}

func documentXML(paragraphs []Paragraph, opts Options) string {
	var b strings.Builder
	b.WriteString(xmlHeader)
	b.WriteString(`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"`)
	b.WriteString(` xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">`)
	b.WriteString(`<w:body>`)
	for _, p := range paragraphs {
		b.WriteString(paragraphXML(p, opts))
	}
	// A4, 2 cm margins, a single text column and no header or footer: every
	// element an ATS could stumble upon is absent on purpose.
	b.WriteString(`<w:sectPr>`)
	b.WriteString(`<w:pgSz w:w="11906" w:h="16838"/>`)
	b.WriteString(`<w:pgMar w:top="794" w:right="794" w:bottom="794" w:left="794" w:header="0" w:footer="0" w:gutter="0"/>`)
	b.WriteString(`<w:cols w:space="708"/>`)
	b.WriteString(`<w:docGrid w:linePitch="360"/>`)
	b.WriteString(`</w:sectPr>`)
	b.WriteString(`</w:body></w:document>`)
	return b.String()
}

func paragraphXML(p Paragraph, opts Options) string {
	var b strings.Builder
	b.WriteString(`<w:p><w:pPr>`)
	b.WriteString(`<w:pStyle w:val="` + Escape(p.Style) + `"/>`)
	if p.KeepNext {
		// The schema puts keepNext right after pStyle and before the border,
		// and the same test that checks this order runs on every part.
		b.WriteString(`<w:keepNext/>`)
	}
	if p.RuleTop && opts.SectionRules {
		// w:pBdr comes right after the numbering, before the spacing: the
		// schema fixes that order.
		b.WriteString(`<w:pBdr><w:top w:val="single" w:sz="4" w:space="6" w:color="` + opts.SectionColor + `"/></w:pBdr>`)
	}
	if rpr := runsProps(p.Runs, opts); rpr != "" {
		b.WriteString(`<w:rPr>` + rpr + `</w:rPr>`)
	}
	b.WriteString(`</w:pPr>`)
	for _, r := range p.Runs {
		if r.Text == "" {
			continue
		}
		b.WriteString(`<w:r>`)
		if rpr := runProps(r, opts); rpr != "" {
			b.WriteString(`<w:rPr>` + rpr + `</w:rPr>`)
		}
		b.WriteString(`<w:t xml:space="preserve">` + Escape(r.Text) + `</w:t>`)
		b.WriteString(`</w:r>`)
	}
	b.WriteString(`</w:p>`)
	return b.String()
}

// runsProps merges the formatting of a paragraph's runs so that the paragraph
// mark itself keeps the same look in Word.
func runsProps(runs []Run, opts Options) string {
	var b strings.Builder
	for _, r := range runs {
		b.WriteString(runProps(r, opts))
	}
	return b.String()
}

// runProps builds the run properties of a run. The child order of w:rPr is
// fixed by the schema: b, i, caps, color, spacing, sz.
func runProps(r Run, opts Options) string {
	var b strings.Builder
	if r.Bold || r.Label {
		b.WriteString(`<w:b/>`)
	}
	if r.Dim {
		b.WriteString(`<w:color w:val="404040"/>`)
	}
	if r.Label {
		b.WriteString(`<w:color w:val="` + opts.SectionColor + `"/>`)
	}
	return b.String()
}

func fontTableXML(family string) string {
	return xmlHeader +
		`<w:fonts xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
		`<w:font w:name="` + Escape(family) + `">` +
		`<w:charset w:val="00"/><w:family w:val="swiss"/><w:pitch w:val="variable"/>` +
		`<w:sig w:usb0="E4002EFF" w:usb1="C000247B" w:usb2="00000009" w:usb3="00000000" w:csb0="000001FF" w:csb1="00000000"/>` +
		`</w:font></w:fonts>`
}

func settingsXML(opts Options) string {
	return xmlHeader +
		`<w:settings xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
		`<w:zoom w:percent="100"/>` +
		`<w:defaultTabStop w:val="708"/>` +
		`<w:characterSpacingControl w:val="doNotCompress"/>` +
		`<w:themeFontLang w:val="` + Escape(opts.LangTag) + `"/>` +
		`<w:compat><w:compatSetting w:name="compatibilityMode" w:uri="http://schemas.microsoft.com/office/word" w:val="15"/></w:compat>` +
		`</w:settings>`
}

// numberingXML declares a single bullet list. A real Word list is used rather
// than a typed "- " so that the paragraphs stay clean text for the parser.
func numberingXML() string {
	var b strings.Builder
	b.WriteString(xmlHeader)
	b.WriteString(`<w:numbering xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">`)
	b.WriteString(`<w:abstractNum w:abstractNumId="0"><w:multiLevelType w:val="hybridMultilevel"/>`)
	indents := []string{"360", "720", "1080"}
	for i, left := range indents {
		b.WriteString(`<w:lvl w:ilvl="` + strconv.Itoa(i) + `">`)
		b.WriteString(`<w:start w:val="1"/>`)
		b.WriteString(`<w:numFmt w:val="bullet"/>`)
		b.WriteString(`<w:lvlText w:val="•"/>`)
		b.WriteString(`<w:lvlJc w:val="left"/>`)
		b.WriteString(`<w:pPr><w:ind w:left="` + left + `" w:hanging="360"/></w:pPr>`)
		b.WriteString(`<w:rPr><w:rFonts w:ascii="Arial" w:hAnsi="Arial" w:cs="Arial" w:hint="default"/></w:rPr>`)
		b.WriteString(`</w:lvl>`)
	}
	b.WriteString(`</w:abstractNum>`)
	b.WriteString(`<w:num w:numId="1"><w:abstractNumId w:val="0"/></w:num>`)
	b.WriteString(`</w:numbering>`)
	return b.String()
}
