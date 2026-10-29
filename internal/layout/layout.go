// Package layout turns a model.Resume into a flat, single-column stream of
// blocks. Every renderer (docx, pdf, asciidoc, text) consumes that same stream,
// which guarantees the four outputs expose an identical reading order.
package layout

import (
	"strings"

	"github.com/fanama/resume/atscv/internal/model"
)

// Kind enumerates the block types of the ATS-safe layout.
type Kind int

const (
	// KindName is the candidate name, the first thing an ATS reads.
	KindName Kind = iota
	// KindHeadline is the job title / seniority line under the name.
	KindHeadline
	// KindContact is one line of plain contact details.
	KindContact
	// KindSection is an uppercase section heading.
	KindSection
	// KindEntryTitle is the job title, degree, project or activity name.
	KindEntryTitle
	// KindEntryMeta carries the organization, the location and the dates.
	KindEntryMeta
	// KindSummary is a paragraph of prose.
	KindSummary
	// KindBullet is a single achievement line.
	KindBullet
	// KindField is a "Label: value" line (Technologies, Team, Coursework...).
	KindField
)

// Block is one rendered unit. Bold marks the primary run of the block, Dim the
// secondary one; both are optional.
type Block struct {
	Kind Kind
	Text string
	Bold string
	Dim  string
	// Bullet is true for KindBullet. The DOCX renderer turns it into a real
	// Word list; the text renderers prefix it with a dash.
	Bullet bool
}

// DateFormat selects how periods are written.
type DateFormat string

const (
	// DateNumeric writes 03/2022 - 08/2022. The most robust form for parsers.
	DateNumeric DateFormat = "numeric"
	// DateMonthName writes mars 2022 - août 2022.
	DateMonthName DateFormat = "month"
	// DateYear writes 2022 - 2022, which loses granularity: use with care.
	DateYear DateFormat = "year"
)

// Options configures the layout.
type Options struct {
	Lang            model.Lang
	DateFormat      DateFormat
	StripDiacritics bool
	SectionRules    bool
}

// DefaultOptions returns the options used when the CLI passes none.
func DefaultOptions() Options {
	return Options{Lang: model.LangFR, DateFormat: DateNumeric}
}

// OptsLang returns the language the layout was built with.
func (l *Layout) OptsLang() model.Lang { return l.opts.Lang }

// DateStyle returns the date format of the layout.
func (l *Layout) DateStyle() DateFormat { return l.opts.DateFormat }

// Layout is the ordered block stream plus a few page-fitting heuristics.
type Layout struct {
	Blocks []Block
	opts   Options
}

// New builds the layout of a resume.
func New(r *model.Resume, opts Options) *Layout {
	if opts.Lang == "" {
		opts.Lang = r.Lang
	}
	if opts.DateFormat == "" {
		opts.DateFormat = DateNumeric
	}
	l := &Layout{opts: opts}
	l.build(r)
	if opts.StripDiacritics {
		l.foldDiacritics()
	}
	return l
}

func (l *Layout) labels() model.Labels { return l.opts.Lang.Labels() }

func (l *Layout) add(b Block) { l.Blocks = append(l.Blocks, b) }

func (l *Layout) section(title string) {
	l.add(Block{Kind: KindSection, Text: title})
}

// spacer separates two entries inside a section.
func (l *Layout) spacer() {
	l.add(Block{Kind: KindSummary, Text: ""})
}

// foldDiacritics rewrites accented characters to their ASCII base so that
// keyword matching never fails on a diacritic.
func (l *Layout) foldDiacritics() {
	for i := range l.Blocks {
		b := &l.Blocks[i]
		b.Text = Fold(b.Text)
		b.Bold = Fold(b.Bold)
		b.Dim = Fold(b.Dim)
	}
}

// joinLocation joins the non-empty parts of a location with commas.
func joinLocation(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, ", ")
}

// trimScheme removes the scheme and the trailing slash of a printed link.
func trimScheme(url string) string {
	for _, scheme := range []string{"https://", "http://"} {
		if strings.HasPrefix(url, scheme) {
			return strings.TrimSuffix(url[len(scheme):], "/")
		}
	}
	return url
}
