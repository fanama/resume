// Package tex renders a layout as a LaTeX document.
//
// The source is written here, in pure Go: no TeX distribution is needed to
// produce the .tex file, no external tool is executed, and the binary keeps
// its single dependency. The document holds the ATS contract of the other
// renderers — one column, no table, no image, no box, native list items, and
// no header, footer or page number. Only compiling the file into a PDF needs
// a TeX installation, and that decision belongs to whoever reads it:
//
//	pdflatex resume.tex
//
// The packages the source asks for (geometry, xcolor, babel, lmodern) ship
// with any complete TeX installation.
package tex

import (
	"github.com/fanama/resume/atscv/internal/layout"
	"github.com/fanama/resume/atscv/internal/model"
)

// Options configures the LaTeX output.
type Options struct {
	// Lang is the document language: it picks the babel option, and with it
	// the hyphenation patterns and the typographic conventions.
	Lang model.Lang
	// SectionRules draws a hairline under every section heading, plus one
	// under the identity block, like the pdf and the docx do.
	SectionRules bool
	// Accent is the RRGGBB color of the headings and the labels. An empty
	// or malformed value falls back to the default navy, because a value
	// that is not a color would fail the compile.
	Accent string
	// FontSize is the body size in points. Zero means 10.5, the body of the
	// other renderers, and the whole scale follows it.
	FontSize float64
	// Header is the optional comment printed at the top of the file.
	Header string
}

// Render returns the LaTeX source of a layout.
func Render(l *layout.Layout, opts Options) string {
	body, lang := normalize(opts)
	d := &doc{opts: opts, body: body, accent: hexColor(opts.Accent, defaultAccent)}
	d.preamble(lang)

	firstSection := true
	inList := false
	for i, blk := range l.Blocks {
		if blk.Kind != layout.KindBullet && inList {
			d.line(`\end{itemize}`)
			inList = false
		}
		hasNext := i+1 < len(l.Blocks)
		var next layout.Block
		if hasNext {
			next = l.Blocks[i+1]
		}
		if blk.Kind == layout.KindBullet {
			d.bullet(blk.Text, &inList)
			continue
		}
		if d.block(blk, firstSection, next, hasNext) {
			firstSection = false
		}
	}
	if inList {
		d.line(`\end{itemize}`)
	}
	d.line(`\end{document}`)
	return d.b.String()
}

// normalize fills the defaults of the body size and the language.
func normalize(opts Options) (float64, model.Lang) {
	body := opts.FontSize
	if body <= 0 {
		body = 10.5
	}
	lang := opts.Lang
	if lang == "" {
		lang = model.LangFR
	}
	return body, lang
}
