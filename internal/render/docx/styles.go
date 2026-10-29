package docx

import (
	"strconv"
	"strings"
)

// Style identifiers used by the renderers. They are declared once in
// styles.xml so a recruiter can restyle the document from Word's style pane.
const (
	StyleName      = "ATSName"
	StyleHeadline  = "ATSHeadline"
	StyleContact   = "ATSContact"
	StyleSection   = "ATSSection"
	StyleEntry     = "ATSEntryTitle"
	StyleEntryMeta = "ATSEntryMeta"
	StyleSummary   = "ATSSummary"
	StyleBullet    = "ATSBullet"
	StyleField     = "ATSField"
)

// stylesXML builds the styles part. Every style is a plain paragraph style
// inheriting from Normal: no text boxes, no frames, no tabs, and no border
// other than the hairlines of the design.
//
// The values below are the whole visual identity of the document: twentieths of
// a point for the spacing, half-points for the sizes. They follow a single
// scale, the body being the reference, so -size rescales the whole design
// instead of only the text.
func stylesXML(opts Options) string {
	accent := opts.SectionColor
	if accent == "" {
		accent = defaultAccent
	}

	var b strings.Builder
	b.WriteString(xmlHeader)
	b.WriteString(`<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">`)

	writeDefaults(&b, opts)
	writeNormal(&b)
	for _, s := range stylesFor(opts, accent) {
		writeStyle(&b, s)
	}

	b.WriteString(`</w:styles>`)
	return b.String()
}

// spacing builds a w:spacing element. OOXML measures in twentieths of a
// point, so 5 pt of air under a heading is "100".
func spacing(beforePt, afterPt float64) string {
	return `<w:spacing w:before="` + hp20(beforePt) + `" w:after="` + hp20(afterPt) + `"` +
		` w:line="264" w:lineRule="auto"/>`
}

// tight is the body leading, 264/240 = 1.1: dense enough to stay on two
// pages, loose enough to read.
const tight = `<w:spacing w:before="0" w:after="0" w:line="240" w:lineRule="auto"/>`

// sizes is the type scale of the document, derived from the body size.
type sizes struct {
	name     float64
	headline float64
	meta     float64
	contact  float64
}

func styleSizes(opts Options) sizes {
	return sizes{
		name:     opts.FontSize + 4.5,
		headline: opts.FontSize + 1,
		meta:     opts.FontSize - 1,
		contact:  opts.FontSize - 1.5,
	}
}

// stylesFor returns every declared style, in the order they are written.
func stylesFor(opts Options, accent string) []style {
	s := styleSizes(opts)
	styles := headingStyles(s, accent)
	styles = append(styles, sectionStyles(opts, s, accent)...)
	return append(styles, bodyStyles(s)...)
}

// headingStyles covers the identity block: the name, the headline and the
// contact line.
func headingStyles(s sizes, accent string) []style {
	return []style{
		{
			// The name is the first thing read and the first thing seen. A
			// twentieth of a point of letter spacing keeps it from looking
			// like a shouting paragraph, and a parser never sees it.
			id:      StyleName,
			name:    "ATS Name",
			primary: true,
			pPr:     `<w:keepNext/>` + spacing(0, 2),
			rPr: `<w:b/><w:color w:val="` + accent + `"/>` +
				`<w:spacing w:val="20"/><w:sz w:val="` + hp(s.name) + `"/><w:szCs w:val="` + hp(s.name) + `"/>`,
		},
		{
			id:      StyleHeadline,
			name:    "ATS Headline",
			primary: true,
			pPr:     `<w:keepNext/>` + spacing(0, 5),
			rPr:     `<w:color w:val="404040"/><w:sz w:val="` + hp(s.headline) + `"/><w:szCs w:val="` + hp(s.headline) + `"/>`,
		},
		{
			id:   StyleContact,
			name: "ATS Contact",
			pPr:  tight,
			rPr:  `<w:color w:val="595959"/><w:sz w:val="` + hp(s.contact) + `"/><w:szCs w:val="` + hp(s.contact) + `"/>`,
		},
	}
}

// sectionStyles covers the structure of the resume: headings, entry titles
// and their metadata line.
func sectionStyles(opts Options, s sizes, accent string) []style {
	return []style{
		{
			// A section heading is the only element allowed a border, and only
			// a bottom one: it is a paragraph border, so the extracted text
			// stays exactly the same.
			id:      StyleSection,
			name:    "ATS Section",
			primary: true,
			pPr: `<w:keepNext/><w:keepLines/>` +
				sectionRule(opts.SectionRules, accent) +
				spacing(14, 5) +
				`<w:outlineLvl w:val="0"/>`,
			rPr: `<w:b/><w:caps/><w:color w:val="` + accent + `"/><w:spacing w:val="10"/>` +
				`<w:sz w:val="` + hp(s.headline) + `"/><w:szCs w:val="` + hp(s.headline) + `"/>`,
		},
		{
			// An entry title keeps its lines with it, so a job never starts at
			// the very bottom of a page.
			id:      StyleEntry,
			name:    "ATS Entry Title",
			primary: true,
			pPr:     `<w:keepNext/><w:keepLines/>` + spacing(7, 0),
			rPr:     `<w:b/>`,
		},
		{
			id:   StyleEntryMeta,
			name: "ATS Entry Meta",
			pPr:  `<w:keepNext/>` + spacing(0, 3),
			rPr:  `<w:color w:val="404040"/><w:sz w:val="` + hp(s.meta) + `"/><w:szCs w:val="` + hp(s.meta) + `"/>`,
		},
	}
}

// bodyStyles covers the content paragraphs: summary, achievements and
// labelled fields.
func bodyStyles(s sizes) []style {
	return []style{
		{
			id:   StyleSummary,
			name: "ATS Summary",
			pPr:  spacing(0, 4),
		},
		{
			// A real Word list, hanging dash at 0.6 cm, and contextualSpacing
			// so two consecutive lines do not accumulate space.
			id:   StyleBullet,
			name: "ATS Bullet",
			pPr: `<w:numPr><w:ilvl w:val="0"/><w:numId w:val="1"/></w:numPr>` +
				spacing(0, 1.5) +
				`<w:ind w:left="340" w:hanging="227"/>` +
				`<w:contextualSpacing/>`,
		},
		{
			id:   StyleField,
			name: "ATS Field",
			pPr:  spacing(0, 1.5),
		},
	}
}

// writeDefaults declares the body font, its size and the proofing language.
func writeDefaults(b *strings.Builder, opts Options) {
	b.WriteString(`<w:docDefaults><w:rPrDefault><w:rPr>`)
	b.WriteString(`<w:rFonts w:ascii="` + Escape(opts.FontFamily) + `" w:hAnsi="` + Escape(opts.FontFamily) +
		`" w:eastAsia="` + Escape(opts.FontFamily) + `" w:cs="` + Escape(opts.FontFamily) + `"/>`)
	b.WriteString(`<w:sz w:val="` + hp(opts.FontSize) + `"/><w:szCs w:val="` + hp(opts.FontSize) + `"/>`)
	b.WriteString(`<w:lang w:val="` + Escape(opts.LangTag) + `" w:eastAsia="` + Escape(opts.LangTag) + `" w:bidi="ar-SA"/>`)
	b.WriteString(`</w:rPr></w:rPrDefault>`)
	b.WriteString(`<w:pPrDefault><w:pPr>`)
	b.WriteString(`<w:spacing w:before="0" w:after="0" w:line="264" w:lineRule="auto"/>`)
	b.WriteString(`</w:pPr></w:pPrDefault></w:docDefaults>`)
}

// writeNormal declares the default paragraph style every other style builds
// on.
func writeNormal(b *strings.Builder) {
	b.WriteString(`<w:style w:type="paragraph" w:default="1" w:styleId="Normal">`)
	b.WriteString(`<w:name w:val="Normal"/><w:qFormat/>`)
	b.WriteString(`</w:style>`)
}

// style is one paragraph style of the document.
type style struct {
	id      string
	name    string
	pPr     string
	rPr     string
	primary bool
}

// writeStyle writes one w:style element, omitting the empty property blocks.
func writeStyle(b *strings.Builder, s style) {
	b.WriteString(`<w:style w:type="paragraph" w:styleId="` + s.id + `">`)
	b.WriteString(`<w:name w:val="` + Escape(s.name) + `"/>`)
	b.WriteString(`<w:basedOn w:val="Normal"/>`)
	b.WriteString(`<w:next w:val="Normal"/>`)
	if s.primary {
		b.WriteString(`<w:qFormat/>`)
	}
	if s.pPr != "" {
		b.WriteString(`<w:pPr>` + s.pPr + `</w:pPr>`)
	}
	if s.rPr != "" {
		b.WriteString(`<w:rPr>` + s.rPr + `</w:rPr>`)
	}
	b.WriteString(`</w:style>`)
}

// sectionRule draws the optional hairline under a section heading. It is a
// paragraph border, never a table or a drawing, so it does not disturb the
// extracted text.
func sectionRule(enabled bool, color string) string {
	if !enabled {
		return ""
	}
	return `<w:pBdr><w:bottom w:val="single" w:sz="4" w:space="2" w:color="` + color + `"/></w:pBdr>`
}

// hp20 converts a size in points to the twentieths of a point used by w:spacing.
func hp20(points float64) string {
	return strconv.Itoa(int(points*20 + 0.5))
}
