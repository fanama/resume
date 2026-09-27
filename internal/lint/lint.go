// Package lint checks a resume against the rules that Applicant Tracking
// Systems apply when they extract a profile: readable section titles, a
// complete contact block, consistent date ranges, quantified achievements and
// no layout element that would hide text.
package lint

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/fanama/resume/atscv/internal/layout"
	"github.com/fanama/resume/atscv/internal/model"
)

// Severity classifies a finding.
type Severity int

const (
	// Error marks a defect that will lose information during parsing.
	Error Severity = iota
	// Warn marks a weakness that costs points on a keyword match.
	Warn
	// Info marks a suggestion.
	Info
)

func (s Severity) String() string {
	switch s {
	case Error:
		return "error"
	case Warn:
		return "warning"
	default:
		return "info"
	}
}

// Finding is a single lint result.
type Finding struct {
	Severity Severity
	Rule     string
	Where    string
	Message  string
	Hint     string
}

func (f Finding) String() string {
	s := fmt.Sprintf("%-7s %-22s %-18s %s", f.Severity, f.Rule, f.Where, f.Message)
	if f.Hint != "" {
		s += "\n        → " + f.Hint
	}
	return s
}

// Report is the result of a lint run.
type Report struct {
	Findings []Finding
	Stats    Stats
}

// Stats holds the metrics a resume is judged on.
type Stats struct {
	Pages          int
	Words          int
	Bullets        int
	Metrics        int
	Entries        int
	Skills         int
	Chars          int
	ATSReadability int
}

// HasErrors reports whether the report contains at least one error.
func (r Report) HasErrors() bool {
	for _, f := range r.Findings {
		if f.Severity == Error {
			return true
		}
	}
	return false
}

// Score is a 0 to 100 readability estimate, the number a recruiter's tool
// would give to the extracted text. It is an estimate, not a guarantee.
func (r Report) Score() int {
	score := 100
	for _, f := range r.Findings {
		switch f.Severity {
		case Error:
			score -= 12
		case Warn:
			score -= 4
		default:
			score -= 1
		}
	}
	if score < 0 {
		return 0
	}
	return score
}

var (
	emailRe = regexp.MustCompile(`^[^@\s]+@[^@\s.]+\.[^@\s]{2,}$`)
	// Banned glyphs are the characters that a PDF text layer or an OCR pass
	// usually turns into noise.
	glyphRe = regexp.MustCompile(`[•▪‣◦·∙–—→⇒★☆✓✔✗✘⚑⚡☎✉➤»«]`)
	emojiRe = regexp.MustCompile(`[\x{1F000}-\x{1FAFF}\x{2600}-\x{27BF}\x{FE0F}]`)
	// MetricRe looks for a number, a percentage or a money amount.
	metricRe = regexp.MustCompile(`\d`)
	wordRe   = regexp.MustCompile(`[\p{L}\p{N}][\p{L}\p{N}'#+./-]*`)
	// French and English articles that must not open a bullet.
	articles = map[string]bool{
		"le": true, "la": true, "les": true, "un": true, "une": true, "des": true,
		"du": true, "de": true, "the": true, "a": true, "an": true,
	}
)

// Run checks a resume and its layout.
func Run(r *model.Resume, l *layout.Layout) Report {
	var rep Report
	c := &checker{resume: r, layout: l, rep: &rep}

	c.checkIdentity()
	c.checkContact()
	c.checkSections()
	c.checkExperience()
	c.checkDates()
	c.checkText()
	c.computeStats()
	return rep
}

type checker struct {
	resume *model.Resume
	layout *layout.Layout
	rep    *Report
}

func (c *checker) add(sev Severity, rule, where, msg, hint string) {
	c.rep.Findings = append(c.rep.Findings, Finding{
		Severity: sev, Rule: rule, Where: where, Message: msg, Hint: hint,
	})
}

// checkIdentity covers the header block.
func (c *checker) checkIdentity() {
	if c.resume.Name == "" {
		c.add(Error, "name", "header", "the resume has no name",
			"an ATS keys the whole profile on the name, put it in \"name\"")
		return
	}
	name := c.resume.Name
	if len([]rune(name)) > 60 {
		c.add(Info, "name", "header",
			fmt.Sprintf("the name is %d characters long", len([]rune(name))),
			"keep it under 60 characters: surnames with several particles are parsed better alone")
	}
	if low := strings.ToLower(name); strings.Contains(low, "cv") || strings.Contains(low, "curriculum") {
		c.add(Info, "name", "header", "the name contains \"CV\"",
			"the file name already says it, keep the name clean")
	}
	if c.resume.Headline == "" {
		c.add(Warn, "headline", "header", "no job title under the name",
			"one line with the exact title of the offer is the highest-return keyword of the whole file")
	}
	if !utf8.ValidString(name) {
		c.add(Error, "encoding", "header", "the name is not valid UTF-8",
			"export the JSON from an editor that writes UTF-8")
	}
}

// checkContact covers the fields a parser uses to reach the candidate.
func (c *checker) checkContact() {
	ct := c.resume.Contact
	switch {
	case ct.Email == "":
		c.add(Error, "contact.email", "header", "no email address",
			"it is often the only mandatory field of the parsed profile")
	case !emailRe.MatchString(ct.Email):
		c.add(Error, "contact.email", "header",
			fmt.Sprintf("%q is not a valid email address", ct.Email),
			"check for a missing dot in the domain or a trailing space")
	}
	if ct.Phone == "" {
		c.add(Warn, "contact.phone", "header", "no phone number",
			"many filters require a reachable number, write it in international format")
	}
	if ct.City == "" && ct.Country == "" {
		c.add(Warn, "contact.location", "header", "no city",
			"the location drives the eligibility filters of most job boards")
	}
	for i, link := range ct.Links {
		if link.URL == "" {
			c.add(Warn, "contact.links", "header",
				fmt.Sprintf("link %d has a label but no URL", i+1),
				"print the URL as text, never as a hidden hyperlink")
			continue
		}
		if !strings.Contains(link.URL, ".") {
			c.add(Warn, "contact.links", "header",
				fmt.Sprintf("link %q does not look like a URL", link.URL), "")
		}
	}
}

// checkSections covers the presence of the blocks a parser looks for.
func (c *checker) checkSections() {
	r := c.resume
	if strings.TrimSpace(r.Summary) == "" {
		c.add(Warn, "section.summary", "profile", "no profile paragraph",
			"three lines that repeat the offer's keywords are read by both the parser and the recruiter")
	} else {
		n := utf8.RuneCountInString(r.Summary)
		switch {
		case n < 150:
			c.add(Info, "section.summary", "profile",
				fmt.Sprintf("the profile is %d characters long", n),
				"aim for 300 to 800 characters of facts: years of experience, domain, stack")
		case n > 1200:
			c.add(Warn, "section.summary", "profile",
				fmt.Sprintf("the profile is %d characters long", n),
				"long paragraphs are skipped by a recruiter in six seconds")
		}
	}
	if len(r.Experience) == 0 {
		c.add(Error, "section.experience", "experience", "no professional experience",
			"it is the section an ATS weighs the most")
	}
	if len(r.Education) == 0 {
		c.add(Warn, "section.education", "education", "no education",
			"many filters still require a degree, even for a senior profile")
	}
	skills := 0
	for _, g := range r.Skills {
		skills += len(g.Items)
	}
	if skills < 5 {
		c.add(Warn, "section.skills", "skills",
			fmt.Sprintf("only %d skills listed", skills),
			"list at least a dozen: the keyword match is literal, so spell the technologies out")
	}
	if len(r.Languages) == 0 {
		c.add(Info, "section.languages", "languages", "no language",
			"several European portals filter on it")
	}
}

// checkExperience reviews the achievement lines.
func (c *checker) checkExperience() {
	for i, e := range c.resume.Experience {
		where := fmt.Sprintf("experience[%d]", i)
		if e.Title == "" {
			c.add(Error, "entry.title", where, "an entry has no job title",
				"the parser classifies the candidate on the title of each position")
		}
		if e.Company == "" {
			c.add(Warn, "entry.company", where, "the entry has no company name", "")
		}
		if len(e.Highlights) == 0 {
			c.add(Warn, "entry.bullets", where,
				fmt.Sprintf("%s has no achievement line", entryName(e)),
				"a job without a single quantified result is dropped by most screeners")
		}
		if len(e.Highlights) > 7 {
			c.add(Info, "entry.bullets", where,
				fmt.Sprintf("%s has %d achievement lines", entryName(e), len(e.Highlights)),
				"keep the five strongest, the rest belong to the interview")
		}
		quantified := 0
		for _, h := range e.Highlights {
			if metricRe.MatchString(h) {
				quantified++
			}
		}
		if len(e.Highlights) > 0 && quantified == 0 {
			c.add(Info, "entry.metrics", where,
				fmt.Sprintf("no line of %s carries a number", entryName(e)),
				"users, latency, coverage, revenue: one figure per line is what gets read")
		}
		if len(e.Stack) == 0 {
			c.add(Info, "entry.stack", where,
				fmt.Sprintf("%s lists no technology", entryName(e)), "")
		}
	}
	for i, e := range c.resume.Education {
		where := fmt.Sprintf("education[%d]", i)
		if e.Degree == "" && e.School == "" {
			c.add(Error, "entry.degree", where, "an education entry is empty", "")
		}
		if e.School == "" {
			c.add(Info, "entry.school", where, "the education entry has no school name", "")
		}
	}
}

func entryName(e model.Experience) string {
	switch {
	case e.Company != "":
		return e.Company
	case e.Title != "":
		return e.Title
	default:
		return "an entry"
	}
}

// checkDates verifies that every period is parseable and consistent.
func (c *checker) checkDates() {
	type period struct {
		where      string
		start, end string
	}
	var periods []period
	for i, e := range c.resume.Experience {
		periods = append(periods, period{
			where: fmt.Sprintf("experience[%d] %s", i, entryName(e)),
			start: e.Start,
			end:   e.End,
		})
	}
	for i, e := range c.resume.Education {
		periods = append(periods, period{
			where: fmt.Sprintf("education[%d] %s", i, e.School),
			start: e.Start,
			end:   e.End,
		})
	}
	for i, a := range c.resume.Activities {
		periods = append(periods, period{
			where: fmt.Sprintf("activities[%d] %s", i, a.Name),
			start: a.Start,
			end:   a.End,
		})
	}

	yearOnly, withMonth := 0, 0
	for _, p := range periods {
		if p.start == "" {
			c.add(Warn, "dates.start", p.where, "the period has no start date",
				"an undated entry is invisible to a chronological filter")
			continue
		}
		if p.end == "" {
			c.add(Warn, "dates.end", p.where, "the period has no end date",
				"write \"present\" for a job you are still in")
		}
		period := c.layout.ParsePeriod(p.start, p.end)
		if !period.HasStart {
			c.add(Error, "dates.parse", p.where,
				fmt.Sprintf("%q is not a date I can read", p.start),
				"use YYYY, YYYY-MM, MM/YYYY or \"Jan 2023\"")
			continue
		}
		if period.HasEnd {
			if months(period.EndYear, period.EndMonth) < months(period.StartYear, period.StartMonth) {
				c.add(Error, "dates.order", p.where,
					fmt.Sprintf("the period ends before it starts (%s → %s)", p.start, p.end), "")
			}
		}
		if period.Present && period.HasEnd {
			c.add(Info, "dates.present", p.where, "both a date and \"present\" are set", "")
		}
		if !period.Present && !period.HasEnd && p.end != "" {
			c.add(Error, "dates.parse", p.where,
				fmt.Sprintf("%q is not a date I can read", p.end),
				"use YYYY, YYYY-MM, MM/YYYY or \"present\"")
		}
		// Granularity comes from the parsed date, not from the raw string: a
		// month name such as "January 2023" carries a month even though it has
		// no separator.
		if period.StartMonth == 0 {
			yearOnly++
		} else {
			withMonth++
		}
	}
	if yearOnly > 0 && withMonth > 0 {
		c.add(Info, "dates.granularity", "dates",
			"some entries carry a month and others only a year",
			"keep the same granularity everywhere, a month is the safest")
	}
}

func months(year, month int) int {
	if month == 0 {
		month = 1
	}
	return year*12 + month
}

// checkText looks for the characters and shapes that break a text extraction.
func (c *checker) checkText() {
	for _, b := range c.layout.Blocks {
		where := blockWhere(b)
		text := strings.Join([]string{b.Text, b.Bold, b.Dim}, " ")
		if m := glyphRe.FindString(text); m != "" {
			c.add(Warn, "glyphs", where,
				fmt.Sprintf("the line holds the decorative glyph %q", m),
				"use a plain dash or nothing at all: a glyph is dropped or turned into a box")
		}
		if emojiRe.MatchString(text) {
			c.add(Error, "emoji", where, "the line holds an emoji",
				"an emoji has no glyph in the PDF core fonts, it becomes a black box")
		}
		if strings.Contains(text, "\t") {
			c.add(Warn, "tabs", where, "the line holds a tabulation",
				"a tab is a column for a parser, use spaces")
		}
		if b.Kind == layout.KindBullet {
			words := strings.Fields(b.Text)
			if len(words) == 0 {
				continue
			}
			if articles[strings.ToLower(strings.Trim(words[0], ".,;:!?\"'()"))] {
				c.add(Info, "bullet.verb", where,
					fmt.Sprintf("the line starts with the article %q", words[0]),
					"start with a verb in the imperative or the past tense: \"Reduced\", \"Deployed\"")
			}
			if n := utf8.RuneCountInString(b.Text); n > 240 {
				c.add(Info, "bullet.length", where,
					fmt.Sprintf("the line is %d characters long", n),
					"aim for a single line of about 120 characters")
			}
		}
	}
}

func blockWhere(b layout.Block) string {
	switch b.Kind {
	case layout.KindName:
		return "header"
	case layout.KindHeadline, layout.KindContact:
		return "header"
	case layout.KindSection:
		return "section " + b.Text
	default:
		if b.Text == "" {
			return "body"
		}
		head := b.Text
		if len([]rune(head)) > 28 {
			head = string([]rune(head)[:28]) + "…"
		}
		return "body " + head
	}
}

// computeStats fills the metrics of the report.
func (c *checker) computeStats() {
	s := &c.rep.Stats
	var text strings.Builder
	for _, b := range c.layout.Blocks {
		text.WriteString(b.Text)
		text.WriteString(" ")
		text.WriteString(b.Bold)
		text.WriteString(" ")
	}
	raw := text.String()
	s.Chars = len([]rune(raw))
	s.Words = len(wordRe.FindAllString(raw, -1))
	for _, b := range c.layout.Blocks {
		switch b.Kind {
		case layout.KindBullet:
			s.Bullets++
			if metricRe.MatchString(b.Text) {
				s.Metrics++
			}
		case layout.KindEntryTitle:
			s.Entries++
		}
	}
	for _, g := range c.resume.Skills {
		s.Skills += len(g.Items)
	}
	// One A4 page holds about 3 400 characters of body text at 10.5 pt.
	s.Pages = (s.Chars + 3399) / 3400
	if s.Pages == 0 {
		s.Pages = 1
	}

	// Readability: start at 100 and subtract the findings, the number of
	// quantified lines, then the length. The result is a comparable number
	// between two versions of the same resume, not an absolute grade.
	score := c.rep.Score()
	if s.Bullets > 0 {
		ratio := float64(s.Metrics) / float64(s.Bullets)
		score -= int(ratio * 30)
	}
	if s.Pages > 2 {
		score -= (s.Pages - 2) * 10
	}
	if s.Words < 200 {
		score -= 10
	}
	if score < 0 {
		score = 0
	}
	s.ATSReadability = score
}

// SortedFindings returns the findings by decreasing severity.
func (r Report) SortedFindings() []Finding {
	out := append([]Finding(nil), r.Findings...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Severity < out[j].Severity })
	return out
}
