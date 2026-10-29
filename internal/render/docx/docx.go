// Package docx writes an ATS-safe .docx file.
//
// The document is deliberately built by hand: the package is a plain zip
// archive of a handful of OOXML parts, with no third-party dependency. It
// contains no table, no text box, no column, no image, no header and no
// footer, so that the reading order an Applicant Tracking System extracts is
// exactly the reading order of the resume.
package docx

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
