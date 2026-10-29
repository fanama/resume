package fill

import (
	"regexp"
	"strings"
)

// proseLead matches the openings of a sentence that describes a role rather
// than claims a result. A highlight starts with a verb in the past — "Livré une
// API", "Reduced latency" — while a description sets a scene first.
var proseLead = regexp.MustCompile(`^(?:` +
	`au sein d|dans le cadre|en tant que|responsable de la|mission principale|` +
	`mon role|mes missions|contexte|l ?equipe|j ?ai |nous avons |` +
	`as part of|within a|in charge of|my role|the team|i was |we were |` +
	`working (?:as|with|on)|responsible for` +
	`)`)

// isProse reports whether a line reads as part of a description rather than as
// a highlight. The test is deliberately conservative: a line only counts as
// prose when it says so clearly, because the cost of the two mistakes is not
// the same. Reading a highlight as prose merges it into a paragraph, which is
// ugly but keeps every word; reading a paragraph as highlights cuts it into
// fragments that stop mid-clause, which is what the form used to show.
func isProse(line string) bool {
	s := strings.TrimSpace(line)
	if s == "" || bulletRe.MatchString(s) {
		return false
	}
	// A line long enough to wrap in the source is a paragraph. The threshold is
	// above the length of a written highlight and below that of a full line of
	// body text in a one column layout.
	if len([]rune(s)) > 120 {
		return true
	}
	if proseLead.MatchString(layoutFold(s)) {
		return true
	}
	// Two sentences on one line is prose by construction: a highlight is one
	// claim, and the mark that separates them is the evidence.
	if strings.Count(s, ". ") >= 1 && len([]rune(s)) > 60 {
		return true
	}
	return false
}

// continuesField reports whether a line is the tail of a field list that ran
// past the width of the page. The renderer breaks such a list on a separator,
// so the tail is short, carries no sentence and names no new field: "OpenAI,
// GCP," then "Elasticsearch". A line that reads as a sentence is not a tail,
// it is the next thing the entry says.
func continuesField(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" || bulletRe.MatchString(s) {
		return false
	}
	if _, _, ok := fieldLine(s); ok {
		return false
	}
	if hasDate(s) || len([]rune(s)) > 90 {
		return false
	}
	// A short line under a field is not automatically its tail: the title of
	// the next job is short too, and reading it as the end of the stack lost
	// the title of every post but the first. The caller decides, by asking
	// whether the field line it came after was actually cut mid-list.
	return true
}

// continuesSentence reports whether a line was broken by the page rather than
// ended by the writer. A line that stops without a terminal mark, and without
// the punctuation a list uses, is the first half of a sentence.
func continuesSentence(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	r := []rune(s)
	switch r[len(r)-1] {
	case '.', '!', '?', ':', ';', '•', ')', ']':
		return false
	}
	// Only a line that is long enough to have been wrapped: a short line that
	// happens to lack a full stop is a heading or a field, not half a sentence.
	return len(r) >= 40
}

// joinWrapped puts back together a sentence the page broke in two. A word split
// across the break carries a hyphen, which is dropped with the space; anything
// else is joined with one space.
func joinWrapped(prev, next string) string {
	prev = strings.TrimSpace(prev)
	next = strings.TrimSpace(next)
	if strings.HasSuffix(prev, "-") {
		return strings.TrimSuffix(prev, "-") + next
	}
	return prev + " " + next
}

// strayTitle reports whether a line that was filed as content is really the
// title of the entry that follows it.
//
// isTitleish carries the whole test. continuesSentence is deliberately not
// consulted: it calls any line of forty characters or more a wrapped sentence,
// and a degree like "Classe préparatoire aux grandes écoles, TSI Maths Sup et
// Spé" is sixty. Asking it here left every long title stuck in the description
// of the entry above it.
func strayTitle(s string) bool { return isTitleish(strings.TrimSpace(s)) }

// lastTitleLine returns the index of the last line of loose that reads as a
// title, or -1. A title is short and carries no mark of its own; a line that
// was marked, or that is long, is content and stays content.
func lastTitleLine(loose []string) int {
	for i := len(loose) - 1; i >= 0; i-- {
		l := strings.TrimSpace(loose[i])
		if l == "" || isDecorative(l) || bulletRe.MatchString(l) {
			continue
		}
		if len([]rune(l)) > 60 {
			return -1
		}
		return i
	}
	return -1
}
