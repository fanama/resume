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
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
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

// fontDirs are the usual places a TrueType file lives, macOS first.
var fontDirs = []string{
	"/System/Library/Fonts/Supplemental",
	"/Library/Fonts",
	"/Library/Fonts/Microsoft",
	"/usr/share/fonts/truetype/dejavu",
	"/usr/share/fonts/truetype/liberation",
	"/usr/share/fonts/TTF",
	"/usr/share/fonts",
	"/usr/local/share/fonts",
}

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

func addTTF(pdf *fpdf.Fpdf, family, style, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("pdf: read font %s: %w", path, err)
	}
	pdf.AddUTF8FontFromBytes(family, style, data)
	return nil
}

// resolveFontFiles looks for a regular and a bold TrueType file. An explicit
// path always wins.
func resolveFontFiles(opts Options) (regular, bold string) {
	if opts.FontFile != "" {
		regular = opts.FontFile
	}
	if opts.BoldFontFile != "" {
		bold = opts.BoldFontFile
	}
	dirs := append([]string{}, fontDirs...)
	if opts.FontDir != "" {
		dirs = append([]string{opts.FontDir}, dirs...)
	}
	if regular == "" {
		regular = lookup(dirs, opts.FontFamily, false)
	}
	if bold == "" {
		bold = lookup(dirs, opts.FontFamily, true)
	}
	return regular, bold
}

// lookup walks the font directories looking for a family, in the "Arial.ttf",
// "Arial Bold.ttf", "arial-bold.ttf" spellings the platforms use.
func lookup(dirs []string, family string, bold bool) string {
	bases := []string{family}
	if tight := strings.ReplaceAll(family, " ", ""); tight != family {
		bases = append(bases, tight)
	}
	styles := []string{""}
	if bold {
		styles = []string{" Bold", "-Bold", "Bold", " bold", "-bold", "bd"}
	}
	for _, dir := range dirs {
		for _, base := range bases {
			for _, style := range styles {
				for _, name := range []string{base + style + ".ttf", strings.ToLower(base + style + ".ttf")} {
					path := filepath.Join(dir, name)
					if info, err := os.Stat(path); err == nil && !info.IsDir() {
						return path
					}
				}
			}
		}
	}
	return ""
}

// CP1252 converts a UTF-8 string to the single byte encoding of the standard
// PDF fonts. Characters outside the encoding become a question mark, which is
// the honest outcome: there is no glyph to draw. A tabulation becomes a space
// because a tab is a column for a text extractor; a line feed is kept, fpdf
// needs it to break a paragraph.
func CP1252(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == '\t':
			b.WriteByte(' ')
		case r < 0x80, r >= 0xA0 && r <= 0xFF:
			// ASCII, the line feed fpdf needs, or Latin-1, which cp1252
			// shares.
			b.WriteByte(byte(r))
		default:
			if c, ok := cp1252High[r]; ok {
				b.WriteByte(c)
			} else {
				b.WriteByte('?')
			}
		}
	}
	return b.String()
}

// cp1252High is the 0x80 to 0x9F range of cp1252, the only part that differs
// from Latin-1.
var cp1252High = map[rune]byte{
	0x20AC: 0x80, 0x201A: 0x82, 0x0192: 0x83, 0x201E: 0x84, 0x2026: 0x85,
	0x2020: 0x86, 0x2021: 0x87, 0x02C6: 0x88, 0x2030: 0x89, 0x0160: 0x8A,
	0x2039: 0x8B, 0x0152: 0x8C, 0x017D: 0x8E, 0x2018: 0x91, 0x2019: 0x92,
	0x201C: 0x93, 0x201D: 0x94, 0x2022: 0x95, 0x2013: 0x96, 0x2014: 0x97,
	0x02DC: 0x98, 0x2122: 0x99, 0x0161: 0x9A, 0x203A: 0x9B, 0x0153: 0x9C,
	0x017E: 0x9E, 0x0178: 0x9F,
}

func (r *renderer) contentWidth() float64 {
	w, _ := r.f.GetPageSize()
	return w - 2*pageMargin
}

// lineHeight is the body leading: 1.32 times the point size, which matches the
// 1.1 line spacing of the .docx closely enough for both to hold two pages.
func (r *renderer) lineHeight() float64 {
	return r.opts.FontSize * 0.44
}

func (r *renderer) font(style string, size float64) {
	r.f.SetFont(r.family, style, size)
}

func (r *renderer) ink() {
	r.f.SetTextColor(0, 0, 0)
}

func (r *renderer) dim() {
	r.f.SetTextColor(r.opts.DimColor.R, r.opts.DimColor.G, r.opts.DimColor.B)
}

// faint paints the marks that must recede: the bullet dash, the contact lines.
func (r *renderer) faint() {
	r.f.SetTextColor(0x59, 0x59, 0x59)
}

func (r *renderer) heading() {
	r.f.SetTextColor(r.opts.SectionColor.R, r.opts.SectionColor.G, r.opts.SectionColor.B)
}

// item is a block with the vertical space it needs, measured before it is
// drawn. A page break taken after the fact is the orphan the reader sees, so the
// renderer measures first and breaks where it decided to, not where the text ran
// out.
type item struct {
	block  layout.Block
	gap    float64 // the space a spacer block left for this one
	height float64
	// entryEnd and entryHeight describe the entry a title opens, so an entry
	// can be kept on one page as a whole.
	entryEnd    int
	entryHeight float64
	// sectionEnd and sectionHeight do the same for a section heading: the whole
	// section, so a rubric that fits on a page is never split, which is what
	// keeps a heading off the bottom of a page.
	sectionEnd    int
	sectionHeight float64
}

// plan turns the block stream into measured items. A spacer is not an item: its
// height becomes the gap of the block that follows, which is how the vertical
// rhythm survives a page break instead of leaving a hole at the top of a page.
func (r *renderer) plan(l *layout.Layout) []item {
	items := make([]item, 0, len(l.Blocks))
	gap := 0.0
	seenSection := false
	for _, b := range l.Blocks {
		if b.Kind == layout.KindSummary && b.Text == "" {
			gap += spaceBeforeEntry
			continue
		}
		it := item{block: b, gap: gap, entryEnd: -1, sectionEnd: -1, height: gap}
		gap = 0
		r.fontOf(b.Kind)
		switch b.Kind {
		case layout.KindSection:
			it.height += spaceBeforeSection + r.lineHeight()
			if r.opts.SectionRules {
				it.height += ruleBelow + spaceAfterSection
				if !seenSection {
					// The first heading also carries the rule that separates
					// the identity from the content.
					it.height += ruleAbove + gapBelowRule
				}
			}
			seenSection = true
		case layout.KindBullet:
			it.height += r.lines(b.Text, r.bulletWidth()) * r.lineHeight()
		case layout.KindField:
			it.height += r.fieldLines(b)
		default:
			it.height += r.lines(b.Text, r.contentWidth()) * r.lineHeight()
		}
		items = append(items, it)
	}
	r.markEntries(items)
	r.markSections(items)
	return items
}

// markEntries records where each entry ends, so the pagination can keep a whole
// job description, degree or activity on one page.
func (r *renderer) markEntries(items []item) {
	for i := range items {
		if items[i].block.Kind != layout.KindEntryTitle {
			continue
		}
		h := items[i].height
		end := i + 1
		for j := i + 1; j < len(items); j++ {
			if items[j].block.Kind == layout.KindEntryTitle || items[j].block.Kind == layout.KindSection {
				break
			}
			h += items[j].height
			end = j + 1
		}
		items[i].entryHeight = h
		items[i].entryEnd = end
	}
}

// markSections records the height of each section, heading included. A section
// that fits on a page of its own is then moved there whole, instead of leaving
// its heading at the bottom of a page with two of its entries and pushing the
// rest overleaf.
func (r *renderer) markSections(items []item) {
	for i := range items {
		if items[i].block.Kind != layout.KindSection {
			continue
		}
		h := items[i].height
		end := i + 1
		for j := i + 1; j < len(items); j++ {
			if items[j].block.Kind == layout.KindSection {
				break
			}
			h += items[j].height
			end = j + 1
		}
		items[i].sectionHeight = h
		items[i].sectionEnd = end
	}
}

// lines returns the number of printed lines a string takes at a given width.
// fpdf only discovers it while printing, which is too late to decide a break;
// this asks the same wrapping in advance, with the same font already set.
func (r *renderer) lines(text string, width float64) float64 {
	if text == "" || width <= 0 {
		return 0
	}
	return float64(len(r.f.SplitLines([]byte(r.encode(text)), width)))
}

// fieldLines returns the height of a "Label: value" block. A value that does not
// fit next to its label hangs under it, which costs a line: the measurement has
// to make the same choice as the drawing, or the last line of a page is the one
// that decides wrongly.
func (r *renderer) fieldLines(b layout.Block) float64 {
	if b.Bold == "" {
		return r.lines(b.Text, r.contentWidth()) * r.lineHeight()
	}
	r.font("B", r.opts.FontSize)
	labelWidth := r.f.GetStringWidth(r.encode(b.Bold + ": "))
	if labelWidth+r.f.GetStringWidth(r.encode(b.Text)) > r.contentWidth() {
		r.font("", r.opts.FontSize)
		return r.lineHeight() + r.lines(b.Text, r.contentWidth()-fieldIndent)*r.lineHeight()
	}
	r.font("", r.opts.FontSize)
	return r.lines(" "+b.Text, r.contentWidth()-labelWidth) * r.lineHeight()
}

// bulletWidth is the width an achievement line wraps at, once the hanging
// indent and the dash are taken out.
func (r *renderer) bulletWidth() float64 {
	r.font("", r.opts.FontSize)
	indent := r.f.GetStringWidth(r.encode("- ")) + bulletIndent
	return r.contentWidth() - hanging - indent
}

// usableHeight is the height a page offers between the two margins.
func (r *renderer) usableHeight() float64 {
	_, height := r.f.GetPageSize()
	return height - 2*pageMargin
}

// run lays the document out.
func (r *renderer) run(l *layout.Layout) {
	items := r.plan(l)
	first := true
	for i := range items {
		it := items[i]
		switch it.block.Kind {
		case layout.KindName:
			// The identity is a unit: a name alone at the top of a second page,
			// away from the contact details, is worse than a short page.
			r.breakIfNeeded(r.untilSection(items, i))
		case layout.KindSection:
			// A rubric that fits on a page of its own is never split: a heading
			// stranded at the bottom of a page is the title at the end of the
			// page, whether a line follows it or not. A section taller than a
			// page has to split, and then it keeps its rule and its first line.
			if it.sectionHeight > 0 && it.sectionHeight <= r.usableHeight() {
				r.breakIfTail(it.sectionHeight, it.sectionEnd < len(items))
			} else {
				r.breakIfNeeded(it.height + r.nextLine(items, i+1))
			}
			first = false
		case layout.KindEntryTitle:
			// Keep a whole entry together when it fits on a page. When it is
			// taller than one, it has to split, but its header stays with the
			// first line of its body: "Ingénieur, Acme, 2021" followed by an
			// unrelated page is how a resume loses a reader.
			more := it.entryEnd < len(items)
			if it.entryHeight > 0 && it.entryHeight <= r.usableHeight() {
				r.breakIfTail(it.entryHeight, more)
			} else {
				r.breakIfTail(r.withFirstLine(items, i), more)
			}
		}
		r.draw(it, first && it.block.Kind == layout.KindSection)
	}
}

// draw prints one measured item.
func (r *renderer) draw(it item, firstSection bool) {
	b := it.block
	r.fontOf(b.Kind)
	switch b.Kind {
	case layout.KindName:
		r.heading()
		r.text(b.Text, it.gap)
	case layout.KindHeadline:
		r.dim()
		r.text(b.Text, it.gap)
	case layout.KindContact:
		r.faint()
		r.text(b.Text, it.gap)
	case layout.KindSection:
		r.section(b.Text, it.gap, firstSection)
	case layout.KindEntryTitle:
		r.ink()
		r.text(b.Text, it.gap)
	case layout.KindEntryMeta:
		r.dim()
		r.text(b.Text, it.gap)
	case layout.KindSummary:
		r.ink()
		r.text(b.Text, it.gap)
	case layout.KindBullet:
		r.ink()
		r.bullet(b.Text, it.gap)
	case layout.KindField:
		r.ink()
		r.field(b.Bold, b.Text, it.gap)
	}
	if r.trace != nil {
		*r.trace = append(*r.trace, drawn{
			page:        r.f.PageCount(),
			kind:        b.Kind,
			text:        b.Text,
			y:           r.f.GetY(),
			height:      it.height,
			entryHeight: it.entryHeight,
		})
	}
}

// fontOf sets the typeface and the size a block is printed with, so that the
// measurement and the drawing agree.
func (r *renderer) fontOf(kind layout.Kind) {
	switch kind {
	case layout.KindName:
		r.font("B", r.opts.FontSize+4.5)
	case layout.KindHeadline:
		r.font("", r.opts.FontSize+1)
	case layout.KindContact:
		r.font("", r.opts.FontSize-1.5)
	case layout.KindSection:
		r.font("B", r.opts.FontSize+1)
	case layout.KindEntryTitle:
		r.font("B", r.opts.FontSize)
	case layout.KindEntryMeta:
		r.font("", r.opts.FontSize-1)
	default:
		r.font("", r.opts.FontSize)
	}
}

// untilSection is the height of everything from i up to the next section
// heading: the identity block, for the only call there is.
func (r *renderer) untilSection(items []item, i int) float64 {
	h := 0.0
	for j := i; j < len(items); j++ {
		if items[j].block.Kind == layout.KindSection {
			break
		}
		h += items[j].height
	}
	return h
}

// nextLine is the height of the first printed line of the item at i, or a single
// line when the section turns out to be empty.
func (r *renderer) nextLine(items []item, i int) float64 {
	if i < len(items) && items[i].height > 0 {
		return items[i].height
	}
	return r.lineHeight()
}

// withFirstLine is the height of an entry header plus the first line of its
// body.
func (r *renderer) withFirstLine(items []item, i int) float64 {
	h := items[i].height
	for j := i + 1; j < len(items); j++ {
		if items[j].block.Kind == layout.KindEntryTitle || items[j].block.Kind == layout.KindSection {
			break
		}
		h += items[j].height
		if items[j].block.Kind != layout.KindEntryMeta {
			break
		}
	}
	return h
}

// breakIfNeeded starts a new page when the next h millimetres do not fit.
func (r *renderer) breakIfNeeded(h float64) {
	_, height := r.f.GetPageSize()
	if r.f.GetY()+h > height-pageMargin {
		r.f.AddPage()
	}
}

// breakIfTail is breakIfNeeded plus the bottom band. When the document carries
// on, a page may not end inside the last two lines: a break there leaves a
// title under one line of content, or one line of content under a title, and
// both read as an accident rather than as a page. The last block of the
// document is exempt, since nothing follows it and no reader can see a break
// that is not there.
func (r *renderer) breakIfTail(h float64, more bool) {
	_, height := r.f.GetPageSize()
	limit := height - pageMargin
	if r.f.GetY()+h > limit || (more && r.f.GetY()+h > limit-r.tail()) {
		r.f.AddPage()
	}
}

// tail is the height of the bottom band, in lines.
func (r *renderer) tail() float64 {
	return r.lineHeight() * tailLines
}

// text prints a wrapped paragraph flush with the left margin.
func (r *renderer) text(text string, gap float64) {
	if gap > 0 {
		r.f.Ln(gap)
	}
	r.f.MultiCell(0, r.lineHeight(), r.encode(text), "", "L", false)
}

// bullet prints an achievement line with a hanging indent, so that a wrapped
// line stays aligned under the text and not under the dash. The dash itself is
// painted in a light gray: the sentence is what the reader, and the parser,
// come for.
func (r *renderer) bullet(text string, gap float64) {
	if gap > 0 {
		r.f.Ln(gap)
	}
	r.f.SetX(pageMargin + hanging)
	dash := "- "
	r.f.SetTextColor(0x8A, 0x8A, 0x8A)
	indent := r.f.GetStringWidth(r.encode(dash)) + bulletIndent
	r.f.CellFormat(indent, r.lineHeight(), r.encode(dash), "", 0, "L", false, 0, "")
	r.ink()
	r.f.MultiCell(r.contentWidth()-hanging-indent, r.lineHeight(), r.encode(text), "", "L", false)
}

// field prints a "Label: value" line. The label carries the accent color, the
// value is plain, which keeps the eye on the category while the parser reads one
// clean line.
func (r *renderer) field(label, value string, gap float64) {
	if gap > 0 {
		r.f.Ln(gap)
	}
	if label == "" {
		r.f.MultiCell(0, r.lineHeight(), r.encode(value), "", "L", false)
		return
	}
	r.font("B", r.opts.FontSize)
	r.heading()
	labelWidth := r.f.GetStringWidth(r.encode(label + ": "))
	if labelWidth+r.f.GetStringWidth(r.encode(value)) > r.contentWidth() {
		// The value does not fit next to its label: print the label on its own
		// line and hang the value under it, so the reader still sees that the
		// list belongs to the label. The parser reads "Label:" then the value.
		r.f.MultiCell(0, r.lineHeight(), r.encode(label+":"), "", "L", false)
		r.font("", r.opts.FontSize)
		r.ink()
		r.f.SetX(pageMargin + fieldIndent)
		r.f.MultiCell(r.contentWidth()-fieldIndent, r.lineHeight(), r.encode(value), "", "L", false)
		return
	}
	// The space belongs to the second run: a trailing space is dropped by
	// most text extractors.
	r.f.CellFormat(labelWidth, r.lineHeight(), r.encode(label+":"), "", 0, "L", false, 0, "")
	r.font("", r.opts.FontSize)
	r.ink()
	r.f.MultiCell(r.contentWidth()-labelWidth, r.lineHeight(), r.encode(" "+value), "", "L", false)
}

// section prints a heading and the hairline under it. The first one also gets a
// hairline above, which separates the identity from the content.
func (r *renderer) section(title string, gap float64, first bool) {
	if first && r.opts.SectionRules {
		y := r.f.GetY() + ruleAbove
		r.hairline(y)
		r.f.SetY(y + gapBelowRule)
	}
	r.font("B", r.opts.FontSize+1)
	r.heading()
	r.f.Ln(spaceBeforeSection + gap)
	r.f.MultiCell(0, r.lineHeight(), r.encode(strings.ToUpper(title)), "", "L", false)
	if r.opts.SectionRules {
		y := r.f.GetY() + ruleBelow
		r.hairline(y)
		r.f.SetY(y + spaceAfterSection)
	}
}

// hairline draws a full width rule in the accent color.
func (r *renderer) hairline(y float64) {
	r.f.SetDrawColor(r.opts.SectionColor.R, r.opts.SectionColor.G, r.opts.SectionColor.B)
	r.f.SetLineWidth(0.25)
	r.f.Line(pageMargin, y, pageMargin+r.contentWidth(), y)
}

// Report is what Measure found about a layout.
type Report struct {
	// Pages is the number of pages the renderer produced.
	Pages int
	// LastPageFill is the share of the usable height the last page uses, 0 to
	// 100. It is the difference between a second page that holds a real section
	// and one that holds three lines.
	LastPageFill float64
	// LastPageBlocks counts the blocks on the last page.
	LastPageBlocks int
	// Sections counts the section headings, the unit a reader would cut.
	Sections int
}

// Measure reports how many pages a layout takes. It runs the real renderer with
// its real font, its real leading and its real page breaks: a character count
// divided by a constant is a guess, and a page count is the one number of this
// document that is checked rather than estimated.
func Measure(l *layout.Layout, opts Options) (Report, error) {
	var trace []drawn
	if err := render(io.Discard, l, opts, &trace); err != nil {
		return Report{}, err
	}
	rep := Report{Pages: 1}
	if len(trace) == 0 {
		return rep, nil
	}
	rep.Pages = trace[len(trace)-1].page
	rep.Sections = 0
	first := -1
	for i, d := range trace {
		if d.kind == layout.KindSection {
			rep.Sections++
		}
		if d.page == rep.Pages && first < 0 {
			first = i
		}
	}
	if first < 0 {
		first = len(trace) - 1
	}
	rep.LastPageBlocks = len(trace) - first

	// The fill is measured from the y the last block reached. The renderer has
	// already run, so the page size is the one it used.
	pdf := fpdf.NewCustom(&fpdf.InitType{UnitStr: "mm", SizeStr: "A4"})
	_, height := pdf.GetPageSize()
	used := trace[len(trace)-1].y - pageMargin
	if usable := height - 2*pageMargin; usable > 0 && used > 0 {
		rep.LastPageFill = used / usable * 100
	}
	return rep, nil
}

// Bytes renders the layout into memory.
func Bytes(l *layout.Layout, opts Options) ([]byte, error) {
	var buf bytes.Buffer
	if err := Render(&buf, l, opts); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
