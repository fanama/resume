package fill

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/fanama/resume/atscv/internal/layout"
)

// A date is read with layout.ParsePeriod rather than with a grammar of its own.
// layout is the package that decides what a period means, it already knows the
// "present" wording of both languages, and a value it accepts is a value the
// rest of the tool can format and lint. A fragment it rejects is left empty for
// the candidate to type, which is better than a plausible wrong date.
var (
	yearRe    = regexp.MustCompile(`\b(?:19|20)\d{2}\b`)
	monthRe   = regexp.MustCompile(`(?i)\b(?:0?[1-9]|1[0-2])\s*[/\-.]\s*(?:19|20)\d{2}\b`)
	ymRe      = regexp.MustCompile(`\b(?:19|20)\d{2}\s*[/\-.]\s*(?:0?[1-9]|1[0-2])\b`)
	monthWord = regexp.MustCompile(`(?i)\b(?:jan(?:v|uary)?|f[eé]v(?:r|rier)?|mars?|avr(?:il)?|mai|juin|juil(?:let)?|ao[uû]t|sept?(?:embre)?|oct(?:obre)?|nov(?:embre)?|d[eé]c(?:embre)?)\.?`)
	monthNum  = map[string]int{
		"jan": 1, "janv": 1, "january": 1, "fev": 2, "fevr": 2, "fevrier": 2, "février": 2,
		"mar": 3, "mars": 3, "avr": 4, "avril": 4, "mai": 5, "jun": 6, "juin": 6,
		"jul": 7, "juil": 7, "juillet": 7, "aou": 8, "aout": 8, "août": 8, "aug": 8, "august": 8,
		"sep": 9, "sept": 9, "septembre": 9, "oct": 10, "octobre": 10, "october": 10,
		"nov": 11, "novembre": 11, "november": 11, "dec": 12, "decembre": 12, "décembre": 12, "december": 12,
	}
	// The wording of an open ended period, in both languages. The French ones
	// that layout knows are covered there; these are the ones written in a CV
	// rather than in a form field.
	openEnded = []string{
		"aujourd'hui", "aujourd hui", "présent", "present", "en cours", "actuel", "actuelle",
		"actuellement", "aujourd", "maintenant", "current", "presently", "ongoing", "now",
		"à ce jour", "a ce jour", "ce jour", "aujourd'hui", "至今",
	}
	// Words that introduce a company, in the order the split tries them.
	companySplitters = []string{" chez ", " @ ", " at ", " für ", " - ", " — ", " – "}
	fieldSplitters   = []string{" | ", " - ", " — ", " – ", " · ", " / ", ", ", " • "}
)

func isDecorative(line string) bool {
	s := strings.TrimSpace(line)
	if s == "" {
		return true
	}
	// A row of dashes or underscores used as a rule.
	if strings.Trim(s, "-_=*·. ") == "" {
		return true
	}
	// A lone page number.
	if n, err := regexp.MatchString(`^\d{1,3}$`, s); err == nil && n {
		return true
	}
	return false
}

// hasDate reports whether a line carries something layout can read as a period.
// It is the cheapest test available and the most useful one: a line with a date
// is a job title line, a school line or a bullet that mentions a year, and the
// first two are what an entry is built from.
func hasDate(line string) bool {
	_, _, ok := splitPeriod(line)
	return ok
}

// splitPeriod finds the period in a line and returns the start and end strings
// as the schema stores them. The returned strings are normalised to what
// layout.ParsePeriod accepts, so a value it can format follows.
func splitPeriod(line string) (start, end string, ok bool) {
	spans := dateSpans(line)
	switch len(spans) {
	case 0:
		return "", "", false
	case 1:
		// One date and an open ended word: "since 2019", "2019 - present".
		if hasOpenEnded(line) {
			return normaliseDate(spans[0]), "present", true
		}
		// A single date alone is a start with no end. It is kept, because a
		// year is the most common thing a CV states.
		return normaliseDate(spans[0]), "", true
	default:
		// Two or more: the first is the start, the last is the end. A middle
		// date is part of the text, not a bound.
		s := normaliseDate(spans[0])
		e := normaliseDate(spans[len(spans)-1])
		if !hasOpenEnded(line) {
			// "Jan 2019 - 2021" mixes a month and a year; both are fine as long
			// as layout can read them.
			return s, e, true
		}
		return s, "present", true
	}
}

// span is a date-like fragment located in a line, so that two regexes matching
// the same text do not both count.
type span struct {
	start, end int
	text       string
}

// dateSpans lists the date-like fragments of a line, in order, with the
// overlap resolved so that "01/2020" is not also read as "2020".
func dateSpans(line string) []string {
	var spans []span
	add := func(re *regexp.Regexp) {
		for _, m := range re.FindAllStringIndex(line, -1) {
			spans = append(spans, span{m[0], m[1], line[m[0]:m[1]]})
		}
	}
	// The two-part numeric forms are looked for before the bare year, and any
	// span that overlaps an earlier one is dropped afterwards.
	add(monthRe)
	add(ymRe)
	add(monthWordWithYear)
	add(yearRe)

	// Drop the spans contained in another one, keeping the longest at each spot.
	sortSpans(spans)
	var kept []span
	for _, s := range spans {
		overlaps := false
		for _, k := range kept {
			if s.start < k.end && k.start < s.end {
				overlaps = true
				break
			}
		}
		if !overlaps {
			kept = append(kept, s)
		}
	}
	out := make([]string, 0, len(kept))
	for _, k := range kept {
		out = append(out, k.text)
	}
	return out
}

// monthWordWithYear catches "Jan 2019" and "mars 2020", which are one date but
// two matches for the single token regexes above.
var monthWordWithYear = regexp.MustCompile(`(?i)\b(?:jan|f[eé]v|mars?|avr|mai|juin|juil|ao[uû]t|sept?|oct|nov|d[eé]c)\w*\.?\s*(?:19|20)\d{2}\b`)

func sortSpans(spans []span) {
	for i := 1; i < len(spans); i++ {
		for j := i; j > 0 && spans[j].start < spans[j-1].start; j-- {
			spans[j], spans[j-1] = spans[j-1], spans[j]
		}
	}
}

// hasOpenEnded reports whether the line says the period is still running.
func hasOpenEnded(line string) bool {
	folded := layout.Fold(strings.ToLower(line))
	for _, w := range openEnded {
		if strings.Contains(folded, w) {
			return true
		}
	}
	return false
}

// normaliseDate turns a fragment into a string layout can read: "2019", "01/2019"
// and "Jan 2019" all survive, and a two digit year gets its century from the
// only one that is plausible.
func normaliseDate(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// "01-2019" and "01.2019" are the same date as "01/2019".
	s = strings.NewReplacer("-", "/", ".", "/").Replace(s)
	// A bare two digit year is read as 20xx, which is the only century a CV
	// uses for a working life.
	if m := regexp.MustCompile(`\b(\d{2})\b`).FindString(s); m != "" && yearRe.MatchString(s) == false {
		s = strings.Replace(s, m, "20"+m, 1)
	}
	return s
}

// isTitleish reports whether a line reads like a role or a school rather than
// like a sentence: short, and not a full stop of prose.
func isTitleish(line string) bool {
	fields := strings.Fields(line)
	if len(fields) == 0 || len(fields) > 12 {
		return false
	}
	if len([]rune(line)) > 90 {
		return false
	}
	// A sentence has a verb and a stop; a title does not.
	if strings.HasSuffix(line, ".") || strings.Contains(line, ". ") {
		return false
	}
	capitals := 0
	for _, f := range fields {
		if r := []rune(f); len(r) > 0 && unicode.IsUpper(r[0]) {
			capitals++
		}
	}
	return capitals > 0
}
