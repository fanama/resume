package tex

import (
	"strings"

	"github.com/fanama/resume/atscv/internal/layout"
	"github.com/fanama/resume/atscv/internal/model"
)

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
	d.packages(lang)
	d.pageLayout()
	d.line(`\begin{document}`)
	d.line(font(d.body))
}

// packages declares the TeX packages the document needs, with babel set to
// the language of the resume.
func (d *doc) packages(lang model.Lang) {
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
}

// pageLayout writes the colors, the margins and the list parameters of the
// document.
func (d *doc) pageLayout() {
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
}

// block writes one block and reports whether it was a section heading.
func (d *doc) block(blk layout.Block, first bool, next layout.Block, hasNext bool) bool {
	switch blk.Kind {
	case layout.KindName:
		d.group(font(d.body+4.5) + `\bfseries\color{accent} ` + escape(blk.Text) + `\par`)
	case layout.KindHeadline:
		d.group(font(d.body+1) + `\color{dim} ` + escape(blk.Text) + `\par`)
	case layout.KindContact:
		d.group(font(d.body-1.5) + `\color{faint} ` + escape(blk.Text) + `\par`)
	case layout.KindSection:
		d.section(blk.Text, first, next, hasNext)
		return true
	case layout.KindEntryTitle:
		d.entryTitle(blk, next, hasNext)
	case layout.KindEntryMeta:
		d.group(font(d.body-1) + `\interlinepenalty=10000 \color{dim} ` +
			escape(blk.Text) + `\par`)
		d.bond(spaceBetween, next, hasNext)
	case layout.KindSummary:
		d.summary(blk)
	case layout.KindField:
		d.field(blk)
	}
	return false
}

// entryTitle writes the title and its optional secondary run on one line, as
// in the docx, and makes the paragraph unbreakable so a title never sits alone
// at the bottom of a page.
func (d *doc) entryTitle(blk layout.Block, next layout.Block, hasNext bool) {
	if blk.Dim != "" {
		d.group(`\interlinepenalty=10000 \bfseries ` + escape(blk.Text) +
			` {\color{dim}| ` + escape(blk.Dim) + `\par}`)
	} else {
		d.group(`\interlinepenalty=10000 \bfseries ` + escape(blk.Text) + `\par`)
	}
	d.bond(spaceBetween, next, hasNext)
}

// summary writes a paragraph, or the spacer between two entries.
func (d *doc) summary(blk layout.Block) {
	if blk.Text == "" {
		// The spacer between two entries: a page may break here.
		d.line(`\vspace{` + spaceEntry + `}`)
		return
	}
	d.line(escape(blk.Text) + `\par`)
}

// field writes one labelled line: the label carries the accent, the value
// stays plain, so the parser reads one clean line either way.
func (d *doc) field(blk layout.Block) {
	if blk.Bold != "" {
		d.line(`{\color{accent}\bfseries ` + escape(blk.Bold) + `:} ` +
			escape(blk.Text) + `\par`)
		return
	}
	d.line(escape(blk.Text) + `\par`)
}

// bullet writes one achievement, opening the list on the first one.
func (d *doc) bullet(text string, inList *bool) {
	if !*inList {
		d.line(`\begin{itemize}`)
		*inList = true
	}
	d.line(`\item ` + escape(text))
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
