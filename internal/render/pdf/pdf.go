// Package pdf renders a layout as a text-based PDF.
//
// The document is a single column of selectable text, with no image, no
// vector shape other than an optional hairline, and no footer: any text
// extractor reads the resume top to bottom, in the same order as the .docx
// output.
//
// When a TrueType font is available the text is embedded with a ToUnicode
// map, so extraction returns the accented characters as they were typed. With
// the standard PDF fonts the text is written in cp1252 instead, which still
// covers French, Spanish, German and Portuguese.
package pdf

import (
	"fmt"
	"io"
	"strings"

	"github.com/fanama/resume/atscv/internal/layout"
	fpdf "github.com/go-pdf/fpdf"
)

// Options configures the PDF output.
type Options struct {
	// FontFamily is the typeface to look for, "Arial" by default. It is
	// resolved against FontDir and the usual system font directories; when no
	// TrueType file is found the standard "Helvetica" is used instead.
	FontFamily string
	// FontDir is an extra directory searched for the font files.
	FontDir string
	// FontFile and BoldFontFile force a specific TrueType file.
	FontFile     string
	BoldFontFile string
	// FontSize is the body size in points.
	FontSize float64
	// SectionColor is the RGB color of the section headings.
	SectionColor RGB
	// SectionRules draws a hairline under each section heading.
	SectionRules bool
	// DimColor is the RGB color of the secondary lines.
	DimColor RGB
	// Title and Author fill the document information dictionary.
	Title  string
	Author string
}

// RGB is a color.
type RGB struct{ R, G, B int }

// DefaultOptions returns the options used when the CLI passes none.
func DefaultOptions() Options {
	return Options{
		FontFamily:   "Arial",
		FontSize:     10.5,
		SectionColor: RGB{0x1F, 0x38, 0x64},
		SectionRules: true,
		DimColor:     RGB{0x40, 0x40, 0x40},
		Title:        "Resume",
	}
}

const (
	pageMargin = 14.0 // mm
	hanging    = 6.0  // mm, the hanging indent of an achievement line
	// The vertical rhythm, in millimetres. The body leading comes from
	// lineHeight; these are the gaps between the blocks.
	spaceBeforeSection = 6.0
	spaceAfterSection  = 2.2
	spaceBeforeEntry   = 3.0
	bulletIndent       = 2.0 // the extra space between the dash and the text
	fieldIndent        = 2.5 // the hang of a value that did not fit next to its label
	ruleAbove          = 1.5 // the rule between the identity and the first section
	ruleBelow          = 1.2 // the gap between a heading and its rule
	gapBelowRule       = 2.0 // the gap between that rule and the first entry
	tailLines          = 2.0 // the bottom band of a page, in lines, where no break may fall while the document carries on
)

type renderer struct {
	f    *fpdf.Fpdf
	opts Options
	// encode translates a string to the encoding of the current font. It is
	// the identity for a TrueType font, and a cp1252 conversion for a
	// standard one.
	encode func(string) string
	// family is the font family name actually in use.
	family string
	// trace, when set, records every block with the page it landed on. The
	// pagination rules are invariants about orphans and split entries, and an
	// invariant is only worth anything if a test can see it.
	trace *[]drawn
}

// drawn is one block written, with the page it landed on, the y it reached and
// the space it had been measured to need.
type drawn struct {
	page        int
	kind        layout.Kind
	text        string
	y           float64
	height      float64
	entryHeight float64 // the whole entry, for a title that opens one
}

// Render writes the layout as a PDF file.
func Render(w io.Writer, l *layout.Layout, opts Options) error {
	return render(w, l, opts, nil)
}

func render(w io.Writer, l *layout.Layout, opts Options, trace *[]drawn) error {
	if opts.FontFamily == "" {
		opts.FontFamily = "Arial"
	}
	if opts.FontSize <= 0 {
		opts.FontSize = 10.5
	}
	if opts.Title == "" {
		opts.Title = "Resume"
	}
	pdf := fpdf.NewCustom(&fpdf.InitType{
		OrientationStr: "P",
		UnitStr:        "mm",
		SizeStr:        "A4",
		FontDirStr:     opts.FontDir,
	})
	pdf.SetMargins(pageMargin, pageMargin, pageMargin)
	pdf.SetAutoPageBreak(true, pageMargin)
	pdf.SetTitle(opts.Title, true)
	pdf.SetAuthor(opts.Author, true)
	pdf.SetCreator("atscv", true)
	// No footer and no page number: a stray "Page 1 of 2" is noise for a
	// parser and for a recruiter.
	pdf.AddPage()

	r, err := newRenderer(pdf, opts)
	if err != nil {
		return err
	}
	r.trace = trace
	r.run(l)
	if err := pdf.Error(); err != nil {
		return fmt.Errorf("pdf: %w", err)
	}
	if err := pdf.Output(w); err != nil {
		return fmt.Errorf("pdf: write: %w", err)
	}
	return nil
}

func newRenderer(pdf *fpdf.Fpdf, opts Options) (*renderer, error) {
	r := &renderer{f: pdf, opts: opts, encode: func(s string) string { return s }}

	regular, bold := resolveFontFiles(opts)
	if regular == "" && bold == "" {
		// No TrueType file: fall back to a standard font, which requires the
		// text to be written in cp1252.
		r.family = "Helvetica"
		r.encode = CP1252
		return r, nil
	}
	if regular == "" {
		regular = bold
	}
	if bold == "" {
		bold = regular
	}
	family := "atscv-" + strings.ReplaceAll(strings.ToLower(opts.FontFamily), " ", "-")
	if err := addTTF(pdf, family, "", regular); err != nil {
		return nil, err
	}
	if err := addTTF(pdf, family, "B", bold); err != nil {
		return nil, err
	}
	r.family = family
	return r, nil
}
