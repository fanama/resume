package pdf

import (
	"bytes"
	"io"

	"github.com/fanama/resume/atscv/internal/layout"
	fpdf "github.com/go-pdf/fpdf"
)

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
	return reportFrom(trace), nil
}

// RenderReport writes the layout as a PDF and answers what Measure would have
// found, in one pass. Measuring after rendering would run the whole document a
// second time, fonts included, to discover a number the first pass already
// knew: the trace is what the pagination produced anyway.
func RenderReport(w io.Writer, l *layout.Layout, opts Options) (Report, error) {
	var trace []drawn
	if err := render(w, l, opts, &trace); err != nil {
		return Report{}, err
	}
	return reportFrom(trace), nil
}

// reportFrom reads a report off the trace of a completed render. An empty
// trace means nothing was drawn, which is one empty page rather than none.
func reportFrom(trace []drawn) Report {
	rep := Report{Pages: 1}
	if len(trace) == 0 {
		return rep
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
	return rep
}

// Bytes renders the layout into memory.
func Bytes(l *layout.Layout, opts Options) ([]byte, error) {
	var buf bytes.Buffer
	if err := Render(&buf, l, opts); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
