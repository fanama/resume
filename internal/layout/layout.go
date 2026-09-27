// Package layout turns a model.Resume into a flat, single-column stream of
// blocks. Every renderer (docx, pdf, asciidoc, text) consumes that same stream,
// which guarantees the four outputs expose an identical reading order.
package layout

import (
	"fmt"
	"strconv"
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

func (l *Layout) build(r *model.Resume) {
	lab := l.labels()

	l.add(Block{Kind: KindName, Text: r.Name})
	if r.Headline != "" {
		l.add(Block{Kind: KindHeadline, Text: r.Headline})
	}
	for _, line := range contactLines(r.Contact) {
		l.add(Block{Kind: KindContact, Text: line})
	}

	if r.Summary != "" {
		l.section(lab.Summary)
		l.add(Block{Kind: KindSummary, Text: r.Summary})
	}

	if len(r.Experience) > 0 {
		l.section(lab.Experience)
		for i, e := range r.Experience {
			l.add(Block{Kind: KindEntryTitle, Text: e.Title, Bold: e.Title})
			if m := l.metaLine(e.Company, e.Location, e.Start, e.End); m != "" {
				l.add(Block{Kind: KindEntryMeta, Text: m})
			}
			if e.Summary != "" {
				l.add(Block{Kind: KindSummary, Text: e.Summary})
			}
			for _, h := range e.Highlights {
				l.add(Block{Kind: KindBullet, Text: h, Bullet: true})
			}
			if e.Team != "" {
				l.add(Block{Kind: KindField, Bold: lab.Team, Text: e.Team})
			}
			if len(e.Stack) > 0 {
				l.add(Block{Kind: KindField, Bold: lab.Technologies, Text: strings.Join(e.Stack, ", ")})
			}
			if i < len(r.Experience)-1 {
				l.spacer()
			}
		}
	}

	if len(r.Education) > 0 {
		l.section(lab.Education)
		for i, e := range r.Education {
			l.add(Block{Kind: KindEntryTitle, Text: e.Degree, Bold: e.Degree})
			if m := l.metaLine(e.School, e.Location, e.Start, e.End); m != "" {
				l.add(Block{Kind: KindEntryMeta, Text: m})
			}
			if e.Summary != "" {
				l.add(Block{Kind: KindSummary, Text: e.Summary})
			}
			if len(e.Coursework) > 0 {
				l.add(Block{Kind: KindField, Bold: lab.CourseworkLabel, Text: strings.Join(e.Coursework, ", ")})
			}
			if i < len(r.Education)-1 {
				l.spacer()
			}
		}
	}

	if len(r.Skills) > 0 {
		l.section(lab.Skills)
		for _, g := range r.Skills {
			text := strings.Join(g.Items, ", ")
			if g.Category != "" {
				l.add(Block{Kind: KindField, Bold: g.Category, Text: text})
			} else {
				l.add(Block{Kind: KindField, Text: text})
			}
		}
	}

	if len(r.Certifications) > 0 {
		l.section(lab.Certifications)
		for _, c := range r.Certifications {
			bits := []string{}
			if c.Issuer != "" {
				bits = append(bits, c.Issuer)
			}
			if c.Date != "" {
				bits = append(bits, c.Date)
			}
			if c.ID != "" {
				bits = append(bits, c.ID)
			}
			l.add(Block{Kind: KindEntryTitle, Text: c.Name, Bold: c.Name, Dim: strings.Join(bits, " | ")})
		}
	}

	if len(r.Projects) > 0 {
		l.section(lab.Projects)
		for i, p := range r.Projects {
			l.add(Block{Kind: KindEntryTitle, Text: p.Name, Bold: p.Name})
			if m := l.metaLine(p.URL, "", p.Start, p.End); m != "" {
				l.add(Block{Kind: KindEntryMeta, Text: m})
			}
			if p.Summary != "" {
				l.add(Block{Kind: KindSummary, Text: p.Summary})
			}
			for _, h := range p.Highlights {
				l.add(Block{Kind: KindBullet, Text: h, Bullet: true})
			}
			if len(p.Technologies) > 0 {
				l.add(Block{Kind: KindField, Bold: lab.Technologies, Text: strings.Join(p.Technologies, ", ")})
			}
			if i < len(r.Projects)-1 {
				l.spacer()
			}
		}
	}

	if len(r.Activities) > 0 {
		l.section(lab.Activities)
		for i, a := range r.Activities {
			l.add(Block{Kind: KindEntryTitle, Text: a.Name, Bold: a.Name})
			if m := l.metaLine(a.Organization, a.Location, a.Start, a.End); m != "" {
				l.add(Block{Kind: KindEntryMeta, Text: m})
			}
			if a.Summary != "" {
				l.add(Block{Kind: KindSummary, Text: a.Summary})
			}
			for _, h := range a.Highlights {
				l.add(Block{Kind: KindBullet, Text: h, Bullet: true})
			}
			if i < len(r.Activities)-1 {
				l.spacer()
			}
		}
	}

	if len(r.Languages) > 0 {
		l.section(lab.Languages)
		items := make([]string, 0, len(r.Languages))
		for _, s := range r.Languages {
			if s.Level != "" {
				items = append(items, s.Name+" ("+s.Level+")")
			} else {
				items = append(items, s.Name)
			}
		}
		l.add(Block{Kind: KindField, Text: strings.Join(items, ", ")})
	}
}

// spacer separates two entries inside a section.
func (l *Layout) spacer() {
	l.add(Block{Kind: KindSummary, Text: ""})
}

// contactLines builds the header block: one plain-text line per group of
// details, separated by pipes. No icons, no glyphs, no hidden labels.
func contactLines(c model.Contact) []string {
	var lines []string
	var identity []string
	if loc := joinLocation(c.City, c.Region, c.Country); loc != "" {
		identity = append(identity, loc)
	}
	if c.Email != "" {
		identity = append(identity, c.Email)
	}
	if c.Phone != "" {
		identity = append(identity, c.Phone)
	}
	if len(identity) > 0 {
		lines = append(lines, strings.Join(identity, " | "))
	}
	var links []string
	for _, lk := range c.Links {
		if lk.URL != "" {
			links = append(links, trimScheme(lk.URL))
		} else {
			links = append(links, lk.Label)
		}
	}
	// Split links over several lines so that no line becomes unreadably long.
	const perLine = 2
	for i := 0; i < len(links); i += perLine {
		end := i + perLine
		if end > len(links) {
			end = len(links)
		}
		lines = append(lines, strings.Join(links[i:end], " | "))
	}
	if c.Note != "" {
		lines = append(lines, c.Note)
	}
	return lines
}

func joinLocation(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, ", ")
}

func trimScheme(url string) string {
	for _, scheme := range []string{"https://", "http://"} {
		if strings.HasPrefix(url, scheme) {
			return strings.TrimSuffix(url[len(scheme):], "/")
		}
	}
	return url
}

// metaLine joins organization, location and the date range, the line every ATS
// expects under a job title.
func (l *Layout) metaLine(org, location, start, end string) string {
	var parts []string
	if org != "" {
		parts = append(parts, org)
	}
	if location != "" {
		parts = append(parts, location)
	}
	head := strings.Join(parts, ", ")
	if period := l.period(start, end); period != "" {
		if head == "" {
			return period
		}
		return head + " | " + period
	}
	return head
}

// Period is a parsed date range.
type Period struct {
	StartYear, StartMonth int // StartMonth is 0 when only a year is known
	EndYear, EndMonth     int
	Present               bool
	HasStart, HasEnd      bool
	RawStart, RawEnd      string
}

var presentWords = map[string]bool{
	"present": true, "now": true, "current": true, "ongoing": true,
	"aujourd'hui": true, "aujourd hui": true, "présent": true,
	"en cours": true, "actuel": true, "actuelle": true,
}

// ParsePeriod reads a start/end pair. Accepted inputs: "2023", "2023-01",
// "2023/01", "Jan 2023", "January 2023", "present".
func (l *Layout) ParsePeriod(start, end string) Period {
	p := Period{RawStart: start, RawEnd: end}
	if y, m, ok := parseDate(start); ok {
		p.StartYear, p.StartMonth, p.HasStart = y, m, true
	}
	if presentWords[strings.ToLower(strings.TrimSpace(end))] {
		p.Present = true
	} else if y, m, ok := parseDate(end); ok {
		p.EndYear, p.EndMonth, p.HasEnd = y, m, true
	}
	return p
}

func parseDate(s string) (year, month int, ok bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, 0, false
	}
	if y, m, ok := parseNumericDate(s); ok {
		return y, m, true
	}
	fields := strings.Fields(s)
	if len(fields) < 2 {
		return 0, 0, false
	}
	// "Jan 2023", "January 2023", "01 Jan 2023", "Jan. 2023"
	for _, f := range fields {
		if m, ok := monthNumber(f); ok {
			month = m
		} else if y, err := strconv.Atoi(f); err == nil && y > 1900 {
			year = y
		}
	}
	if year == 0 {
		return 0, 0, false
	}
	return year, month, true
}

func parseNumericDate(s string) (year, month int, ok bool) {
	clean := strings.NewReplacer("-", "/", ".", "/", " ", "/").Replace(s)
	parts := strings.Split(clean, "/")
	nums := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return 0, 0, false
		}
		nums = append(nums, n)
	}
	switch len(nums) {
	case 1:
		if nums[0] < 1900 {
			return 0, 0, false
		}
		return nums[0], 0, true
	case 2:
		a, b := nums[0], nums[1]
		switch {
		case a >= 1000: // 2023/01
			if b >= 1 && b <= 12 {
				return a, b, true
			}
		case b >= 1000: // 01/2023
			if a >= 1 && a <= 12 {
				return b, a, true
			}
		}
	}
	return 0, 0, false
}

var monthNames = map[string]int{
	"jan": 1, "january": 1, "janv": 1, "janvier": 1, "jan.": 1,
	"feb": 2, "february": 2, "fevr": 2, "février": 2, "fev": 2, "fev.": 2, "février.": 2,
	"mar": 3, "march": 3, "mars": 3,
	"apr": 4, "april": 4, "avr": 4, "avril": 4,
	"may": 5, "mai": 5,
	"jun": 6, "june": 6, "juin": 6,
	"jul": 7, "july": 7, "juil": 7, "juillet": 7,
	"aug": 8, "august": 8, "aout": 8, "août": 8,
	"sep": 9, "sept": 9, "september": 9, "septembre": 9,
	"oct": 10, "october": 10, "octobre": 10,
	"nov": 11, "november": 11, "novembre": 11,
	"dec": 12, "december": 12, "decembre": 12, "décembre": 12, "déc.": 12,
}

func monthNumber(token string) (int, bool) {
	t := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(token), "."))
	if t == "" {
		return 0, false
	}
	if n, err := strconv.Atoi(t); err == nil && n >= 1 && n <= 12 {
		return n, true
	}
	if m, ok := monthNames[t]; ok {
		return m, true
	}
	return 0, false
}

var monthLabels = map[model.Lang][]string{
	model.LangFR: {"", "janv.", "févr.", "mars", "avr.", "mai", "juin", "juil.", "août", "sept.", "oct.", "nov.", "déc."},
	model.LangEN: {"", "Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"},
}

// FormatMonth writes a month/year pair according to the date format. A date
// that only carries a year stays a year: printing 01/2023 for a 2023 start
// would invent a month the candidate never gave.
func (l *Layout) FormatMonth(year, month int) string {
	if month == 0 || l.opts.DateFormat == DateYear {
		return strconv.Itoa(year)
	}
	if l.opts.DateFormat == DateMonthName {
		return fmt.Sprintf("%s %d", monthLabels[l.opts.Lang][month], year)
	}
	return fmt.Sprintf("%02d/%d", month, year)
}

// period renders a date range such as "03/2022 - 08/2022" or "01/2023 - Present".
func (l *Layout) period(start, end string) string {
	p := l.ParsePeriod(start, end)
	var parts []string
	switch {
	case p.HasStart && p.Present:
		parts = []string{l.FormatMonth(p.StartYear, p.StartMonth), l.labels().Present}
	case p.HasStart && p.HasEnd:
		parts = []string{l.FormatMonth(p.StartYear, p.StartMonth), l.FormatMonth(p.EndYear, p.EndMonth)}
	case p.HasStart:
		parts = []string{l.FormatMonth(p.StartYear, p.StartMonth)}
	case p.Present:
		parts = []string{l.labels().Present}
	case p.HasEnd:
		parts = []string{l.FormatMonth(p.EndYear, p.EndMonth)}
	}
	if len(parts) == 0 {
		if start != "" && end != "" {
			return start + " - " + end
		}
		return strings.TrimSpace(start + " " + end)
	}
	if len(parts) == 1 {
		// A one sided range: an open start ("2020 - ") would look like a
		// mistake to a recruiter, so the known bound is printed alone.
		return parts[0]
	}
	return parts[0] + " - " + parts[1]
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

// Fold removes diacritics from a string, keeping non-latin scripts intact.
func Fold(s string) string {
	if s == "" {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if folded, ok := foldRune(r); ok {
			b.WriteRune(folded)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// foldTable maps a latin letter carrying a diacritic to its ASCII base letter.
// Letters without an ASCII equivalent (the German sharp s) are absent on
// purpose, and so is every non-latin script: they are copied unchanged.
var foldTable = map[rune]rune{
	'À': 'A', 'Á': 'A', 'Â': 'A', 'Ã': 'A', 'Ä': 'A', 'Å': 'A', 'Ā': 'A', 'Ă': 'A', 'Ą': 'A',
	'Ç': 'C', 'Ć': 'C', 'Č': 'C',
	'Ď': 'D', 'Đ': 'D',
	'È': 'E', 'É': 'E', 'Ê': 'E', 'Ë': 'E', 'Ē': 'E', 'Ĕ': 'E', 'Ė': 'E', 'Ę': 'E', 'Ě': 'E',
	'Ì': 'I', 'Í': 'I', 'Î': 'I', 'Ï': 'I', 'Ĩ': 'I', 'Ī': 'I', 'Ĭ': 'I', 'Į': 'I', 'İ': 'I',
	'Ł': 'L',
	'Ñ': 'N', 'Ń': 'N', 'Ņ': 'N',
	'Ò': 'O', 'Ó': 'O', 'Ô': 'O', 'Õ': 'O', 'Ö': 'O', 'Ø': 'O', 'Ō': 'O', 'Ŏ': 'O', 'Ő': 'O',
	'Œ': 'O', 'Æ': 'A',
	'Ŕ': 'R', 'Ř': 'R',
	'Ś': 'S', 'Ş': 'S', 'Š': 'S',
	'Ţ': 'T', 'Ť': 'T', 'Ŧ': 'T', 'Þ': 'T',
	'Ù': 'U', 'Ú': 'U', 'Û': 'U', 'Ü': 'U', 'Ũ': 'U', 'Ū': 'U', 'Ŭ': 'U', 'Ů': 'U', 'Ű': 'U', 'Ų': 'U',
	'Ŵ': 'W',
	'Ý': 'Y', 'Ŷ': 'Y', 'Ÿ': 'Y',
	'Ź': 'Z', 'Ż': 'Z', 'Ž': 'Z',

	'à': 'a', 'á': 'a', 'â': 'a', 'ã': 'a', 'ä': 'a', 'å': 'a', 'ā': 'a', 'ă': 'a', 'ą': 'a',
	'ç': 'c', 'ć': 'c', 'č': 'c',
	'ď': 'd', 'đ': 'd', 'ð': 'd',
	'è': 'e', 'é': 'e', 'ê': 'e', 'ë': 'e', 'ē': 'e', 'ĕ': 'e', 'ė': 'e', 'ę': 'e', 'ě': 'e',
	'ì': 'i', 'í': 'i', 'î': 'i', 'ï': 'i', 'ĩ': 'i', 'ī': 'i', 'ĭ': 'i', 'į': 'i', 'ı': 'i',
	'ł': 'l',
	'ñ': 'n', 'ń': 'n', 'ņ': 'n',
	'ò': 'o', 'ó': 'o', 'ô': 'o', 'õ': 'o', 'ö': 'o', 'ø': 'o', 'ō': 'o', 'ŏ': 'o', 'ő': 'o',
	'œ': 'o', 'æ': 'a',
	'ŕ': 'r', 'ř': 'r',
	'ś': 's', 'ş': 's', 'š': 's',
	'ţ': 't', 'ť': 't', 'ŧ': 't',
	'ù': 'u', 'ú': 'u', 'û': 'u', 'ü': 'u', 'ũ': 'u', 'ū': 'u', 'ŭ': 'u', 'ů': 'u', 'ű': 'u', 'ų': 'u',
	'ŵ': 'w',
	'ý': 'y', 'ŷ': 'y', 'ÿ': 'y',
	'ź': 'z', 'ż': 'z', 'ž': 'z',
}

// foldRune returns the ASCII base of a latin letter, if it has one.
func foldRune(r rune) (rune, bool) {
	folded, ok := foldTable[r]
	return folded, ok
}
