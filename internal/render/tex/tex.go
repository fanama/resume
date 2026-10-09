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
	"strconv"
	"strings"

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

// defaultAccent is the navy of the headings when -accent is not given.
const defaultAccent = "1F3864"

// leading is the line spacing of the document, the same 1.25 as the pdf: the
// body size is the reference of one typographic scale, and every size below
// carries its own leading so the rhythm survives a size change.
const leading = 1.25

// The vertical rhythm, in millimetres, copied from the pdf renderer so the
// two documents read alike.
const (
	spaceSection     = "6mm"   // before a section heading
	spaceFirstRule   = "1.5mm" // the hairline under the identity block
	spaceRuleHeading = "8mm"   // that hairline, then the first heading
	spaceHeadingBody = "2.2mm" // a heading and the block it announces
	spaceUnderRule   = "1.2mm" // a heading and the hairline under it
	spaceBetween     = "1.2mm" // a title and its dates, the dates and the body
	spaceEntry       = "3mm"   // between two entries of a section
	ruleThickness    = "0.25mm"
)

// Render returns the LaTeX source of a layout.
func Render(l *layout.Layout, opts Options) string {
	body := opts.FontSize
	if body <= 0 {
		body = 10.5
	}
	lang := opts.Lang
	if lang == "" {
		lang = model.LangFR
	}
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
		switch blk.Kind {
		case layout.KindName:
			d.group(font(body+4.5) + `\bfseries\color{accent} ` + escape(blk.Text) + `\par`)
		case layout.KindHeadline:
			d.group(font(body+1) + `\color{dim} ` + escape(blk.Text) + `\par`)
		case layout.KindContact:
			d.group(font(body-1.5) + `\color{faint} ` + escape(blk.Text) + `\par`)
		case layout.KindSection:
			d.section(blk.Text, firstSection, next, hasNext)
			firstSection = false
		case layout.KindEntryTitle:
			// The secondary run shares the title's line, as in the docx, and
			// the paragraph is made unbreakable so a title never sits alone
			// at the bottom of a page.
			if blk.Dim != "" {
				d.group(`\interlinepenalty=10000 \bfseries ` + escape(blk.Text) +
					` {\color{dim}| ` + escape(blk.Dim) + `\par}`)
			} else {
				d.group(`\interlinepenalty=10000 \bfseries ` + escape(blk.Text) + `\par`)
			}
			d.bond(spaceBetween, next, hasNext)
		case layout.KindEntryMeta:
			d.group(font(body-1) + `\interlinepenalty=10000 \color{dim} ` +
				escape(blk.Text) + `\par`)
			d.bond(spaceBetween, next, hasNext)
		case layout.KindSummary:
			if blk.Text == "" {
				// The spacer between two entries: a page may break here.
				d.line(`\vspace{` + spaceEntry + `}`)
				continue
			}
			d.line(escape(blk.Text) + `\par`)
		case layout.KindBullet:
			if !inList {
				d.line(`\begin{itemize}`)
				inList = true
			}
			d.line(`\item ` + escape(blk.Text))
		case layout.KindField:
			// The label carries the accent, the value stays plain: the parser
			// reads one clean line either way.
			if blk.Bold != "" {
				d.line(`{\color{accent}\bfseries ` + escape(blk.Bold) + `:} ` +
					escape(blk.Text) + `\par`)
			} else {
				d.line(escape(blk.Text) + `\par`)
			}
		}
	}
	if inList {
		d.line(`\end{itemize}`)
	}
	d.line(`\end{document}`)
	return d.b.String()
}

// doc accumulates the source one line at a time.
type doc struct {
	b      strings.Builder
	opts   Options
	body   float64
	accent string
}

func (d *doc) line(s string) { d.b.WriteString(s); d.b.WriteByte('\n') }

// group writes a braced group: the font, the color and the \par of a block
// all live inside it, so nothing leaks into the block that follows.
func (d *doc) group(s string) { d.line(`{` + s + `}`) }

// keep forbids a page break at the point it is written.
func (d *doc) keep() { d.line(`\nopagebreak[4]`) }

// preamble writes the header comments and the fixed part of the document.
func (d *doc) preamble(lang model.Lang) {
	if d.opts.Header != "" {
		for _, line := range strings.Split(strings.TrimRight(d.opts.Header, "\n"), "\n") {
			d.line(`% ` + line)
		}
	}
	d.line(`% Single column, no table, no image: the same block stream as the docx and the pdf.`)
	d.line(`\documentclass[10pt,a4paper]{article}`)
	d.line(`\usepackage[T1]{fontenc}`)
	d.line(`\usepackage[utf8]{inputenc}`)
	d.line(`\usepackage{lmodern}`)
	babel := "french"
	if lang == model.LangEN {
		babel = "english"
	}
	d.line(`\usepackage[` + babel + `]{babel}`)
	d.line(`\usepackage[a4paper,margin=14mm]{geometry}`)
	d.line(`\usepackage{xcolor}`)
	d.line(`\definecolor{accent}{HTML}{` + d.accent + `}`)
	d.line(`\definecolor{dim}{HTML}{404040}`)
	d.line(`\definecolor{faint}{HTML}{595959}`)
	d.line(`\definecolor{dash}{HTML}{8A8A8A}`)
	// No header, no footer, no page number: an ATS and a recruiter both read
	// the flow, and a number is noise in it.
	d.line(`\pagestyle{empty}`)
	d.line(`\setlength{\parindent}{0pt}`)
	d.line(`\setlength{\partopsep}{0pt}`)
	d.line(`\setlength{\leftmargini}{6mm}`)
	d.line(`\setlength{\labelsep}{1.5mm}`)
	d.line(`\renewcommand{\labelitemi}{\textcolor{dash}{\textbullet}}`)
	// The list parameters of the class would put four points of air around
	// every list; the achievements of a resume follow the rhythm of the
	// text instead, so \@listi is rewritten with the gaps the pdf uses.
	d.line(`\makeatletter`)
	d.line(`\renewcommand{\@listi}{\leftmargin\leftmargini \topsep ` + spaceBetween +
		` \parsep 0pt \itemsep ` + spaceBetween + `}`)
	d.line(`\makeatother`)
	d.line(`\begin{document}`)
	d.line(font(d.body))
}

// section writes a heading with the rhythm of the pdf: six millimetres
// before it, the hairline under it when the option is on, and no page break
// between the heading, its rule and the first line it announces.
func (d *doc) section(title string, first bool, next layout.Block, hasNext bool) {
	rules := d.opts.SectionRules
	d.line(`\par`)
	if first && rules {
		d.line(`\vspace{` + spaceFirstRule + `}`)
		d.keep()
		d.line(d.rule())
		d.keep()
		d.line(`\vspace{` + spaceRuleHeading + `}`)
	} else {
		d.line(`\vspace{` + spaceSection + `}`)
	}
	d.group(font(d.body+1) + `\interlinepenalty=10000 \bfseries\color{accent} ` +
		escape(strings.ToUpper(title)) + `\par`)
	if rules {
		d.keep()
		d.line(`\vspace{` + spaceUnderRule + `}`)
		d.keep()
		d.line(d.rule())
	}
	d.bond(spaceHeadingBody, next, hasNext)
}

// rule is a hairline in the accent color, drawn as a box: a box is never a
// place a page can break, so the rule can never be stranded from its heading.
func (d *doc) rule() string {
	return `{\color{accent}\rule{\linewidth}{` + ruleThickness + `}}`
}

// bond writes what keeps the block just written on the same page as the one
// that follows. Everywhere a break is welcome — before a section, before the
// spacer between two entries, at the end of the document — it writes nothing.
//
// The pattern is always a penalty before the glue, never after it: a penalty
// that immediately precedes a glue is the penalty of the break at that glue,
// so the pair forbids the break while the gap stays exactly what it says.
// A bullet list already brings its own spacing, the topsep of the list, and
// a second glue right after it would be a break no penalty can guard.
func (d *doc) bond(space string, next layout.Block, hasNext bool) {
	if !hasNext {
		return
	}
	switch {
	case next.Kind == layout.KindBullet:
		d.keep()
	case next.Kind == layout.KindSection:
	case next.Kind == layout.KindSummary && next.Text == "":
	default:
		d.keep()
		d.line(`\vspace{` + space + `}`)
	}
}

// hexColor reads an RRGGBB color for \definecolor, falling back when the
// value is not a color: a crafted accent must not reach the file, where it
// would fail the compile instead of the request.
func hexColor(s, fallback string) string {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) != 6 {
		return fallback
	}
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'f', r >= 'A' && r <= 'F':
		default:
			return fallback
		}
	}
	return strings.ToUpper(s)
}

// num formats a size without trailing zeros, so 10.5 is "10.5" and 15 is "15".
func num(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

// font is the \fontsize pair of one step of the scale: the body is the
// reference and the leading follows it, exactly like the pdf renderer.
func font(size float64) string {
	return `\fontsize{` + num(size) + `}{` + num(size*leading) + `}\selectfont`
}

// escape turns a piece of resume text into LaTeX source. Every character the
// language reads specially is rewritten, so a name like "Rakoto & fils_2"
// typesets instead of breaking the compile.
func escape(s string) string {
	if !strings.ContainsAny(s, `\{}#$%&~^_`) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\textbackslash{}`)
		case '{':
			b.WriteString(`\{`)
		case '}':
			b.WriteString(`\}`)
		case '#':
			b.WriteString(`\#`)
		case '$':
			b.WriteString(`\$`)
		case '%':
			b.WriteString(`\%`)
		case '&':
			b.WriteString(`\&`)
		case '_':
			b.WriteString(`\_`)
		case '~':
			b.WriteString(`\textasciitilde{}`)
		case '^':
			b.WriteString(`\textasciicircum{}`)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
