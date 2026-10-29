package fill

import (
	"regexp"
	"strings"

	"github.com/fanama/resume/atscv/internal/layout"
	"github.com/fanama/resume/atscv/internal/model"
)

// buildEntry reads one line into a title and a company. The formats in the wild
// are: "Title | Company | 2019", "Title, Company (2019)", "2019 - 2021 · Title
// · Company", "Title at Company, 2019". The date is taken out first, then the
// rest is split on the separators, and the longest part is the title because a
// company is usually the shorter of the two.
func (f *filler) buildEntry(line, start, end string) entry {
	e := entry{start: start, end: end}
	rest := entryText(line)
	if rest == "" {
		// A line that is only a period: the title is on the next line, which
		// the caller stores, and this entry waits for it.
		return e
	}
	// "Title chez Company" and its English form, tried before the separators,
	// because the word is a stronger signal than a pipe.
	if title, company, ok := splitCompany(rest); ok {
		e.title, e.company = title, company
		return e
	}
	parts := splitOnSeparators(rest)
	// "Acme, Paris | 2021 - 2024" is a company and a place, not a title and a
	// company, and the second part is the one that names a place. The only
	// reliable way to tell is the document itself: the contact block of a resume
	// states where the candidate is, and a part that repeats it is a location.
	if n := f.placeIndex(parts); n >= 0 {
		e.location = clean(parts[n])
		e.company = firstOutside(parts, n)
		return e
	}
	e.title, e.company = titleAndCompany(rest, parts)
	return e
}

// entryText removes the period and the punctuation around a title line.
func entryText(line string) string {
	// Both halves of the period are removed before the line is split: leaving
	// the wording of an open end in place makes the " - " that surrounded the
	// dates look like a separator, and the word "Present" look like a company.
	rest := strings.TrimSpace(stripPeriod(line))
	rest = strings.Trim(strings.TrimSpace(rest), "|·•-–—,;:()[]")
	return strings.TrimSpace(rest)
}

// splitCompany reads a title followed by an explicit company separator.
func splitCompany(rest string) (title, company string, ok bool) {
	for _, sep := range companySplitters {
		if a, b, ok := strings.Cut(rest, sep); ok {
			if isTitleish(a) && isTitleish(b) {
				return clean(a), clean(b), true
			}
		}
	}
	return "", "", false
}

// firstOutside is the first part that is not the place at index skip.
func firstOutside(parts []string, skip int) string {
	for i, p := range parts {
		if i != skip {
			return clean(p)
		}
	}
	return ""
}

// titleAndCompany splits the remaining parts, recognising a school or a degree
// when the words say so.
func titleAndCompany(rest string, parts []string) (title, company string) {
	switch len(parts) {
	case 0:
		return clean(rest), ""
	case 1:
		return clean(parts[0]), ""
	}
	// Two parts: the one that names a school or a degree is the school, the
	// other is the title. With two neutral parts, the first is the title.
	if hasWord(parts[0], schoolWords) || hasWord(parts[1], degreeWords) {
		return clean(parts[1]), clean(parts[0])
	}
	return clean(parts[0]), clean(parts[1])
}

var (
	sepRun = regexp.MustCompile(`\s*(?:\||·|•|—|–|\s+-\s+|\s*[,;]\s+|\s+/\s+|\s{2,})\s*`)
	// Trailing punctuation an extraction leaves on the last field of a line.
	trailing = regexp.MustCompile(`^[\s|·•\-–—,;:()\[\]]+|[\s|·•\-–—,;:()\[\]]+$`)
)

// splitOnSeparators breaks a line on the separators a resume uses between
// fields, and drops the parts that carry nothing.
func splitOnSeparators(s string) []string {
	var out []string
	for _, p := range sepRun.Split(s, -1) {
		p = clean(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// clean trims the punctuation around a field and collapses its spaces.
func clean(s string) string {
	return collapseSpaces(strings.TrimSpace(trailing.ReplaceAllString(strings.TrimSpace(s), "")))
}

func layoutFold(s string) string { return layout.Fold(strings.ToLower(s)) }

// cleanBullet turns a line of prose into a highlight: trimmed of its trailing
// stop, which a bullet never carries, and kept whole even when it is long,
// because a highlight is allowed to be a sentence.
func cleanBullet(s string) string {
	s = collapseSpaces(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	return strings.TrimSuffix(s, ".")
}

// The fields an entry can carry besides its title and its period.
const (
	fieldTeam  = "team"
	fieldStack = "stack"
)

// fieldLine reads a "Label: value" line into the field it names. Both languages
// are accepted whatever the resume is in, because a French CV that lists an
// English stack is common and the label costs nothing to recognise.
func fieldLine(line string) (label, value string, ok bool) {
	head, rest, cut := strings.Cut(line, ":")
	if !cut {
		return "", "", false
	}
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return "", "", false
	}
	switch layoutFold(strings.TrimSpace(head)) {
	case layoutFold(model.LangFR.Labels().Team), layoutFold(model.LangEN.Labels().Team):
		return fieldTeam, rest, true
	case layoutFold(model.LangFR.Labels().Technologies), layoutFold(model.LangEN.Labels().Technologies):
		return fieldStack, rest, true
	}
	return "", "", false
}
