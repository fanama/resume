package pdf

import (
	"strings"

	"github.com/fanama/resume/atscv/internal/layout"
)

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

// draw prints one measured item.
func (r *renderer) draw(it item, firstSection bool) {
	b := it.block
	r.fontOf(b.Kind)
	switch b.Kind {
	case layout.KindName:
		r.heading()
	case layout.KindHeadline, layout.KindEntryMeta:
		r.dim()
	case layout.KindContact:
		r.faint()
	case layout.KindEntryTitle, layout.KindSummary, layout.KindBullet, layout.KindField:
		r.ink()
	}
	switch b.Kind {
	case layout.KindSection:
		r.section(b.Text, it.gap, firstSection)
	case layout.KindBullet:
		r.bullet(b.Text, it.gap)
	case layout.KindField:
		r.field(b.Bold, b.Text, it.gap)
	default:
		r.text(b.Text, it.gap)
	}
	r.traceDraw(b, it)
}

// traceDraw records where a block landed and how tall it was, for the tests
// that check the pagination.
func (r *renderer) traceDraw(b layout.Block, it item) {
	if r.trace == nil {
		return
	}
	*r.trace = append(*r.trace, drawn{
		page:        r.f.PageCount(),
		kind:        b.Kind,
		text:        b.Text,
		y:           r.f.GetY(),
		height:      it.height,
		entryHeight: it.entryHeight,
	})
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
