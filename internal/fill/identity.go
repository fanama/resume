package fill

import (
	"regexp"
	"strings"

	"github.com/fanama/resume/atscv/internal/layout"
)

// readIdentity takes the name and the title from the lines that precede the
// first heading. Both are guesses about position: in every template that has
// them, the name is at the top and the title is under it.
func (f *filler) readIdentity(head []string) []string {
	i := 0
	// Skip the decorative lines some templates print: a rule of dashes, or a
	// page number left in the text.
	for i < len(head) && isDecorative(head[i]) {
		i++
	}
	if i < len(head) {
		if name := cleanName(head[i]); name != "" {
			f.resume.Name = name
			i++
		}
	}
	// The title is the next line, if it reads like one: short, no date, and
	// either ends in a job family or is a single noun phrase.
	if i < len(head) {
		if h := cleanHeadline(head[i]); h != "" {
			f.resume.Headline = h
			i++
		}
	}
	rest := head[i:]
	if len(rest) > 12 {
		// A head that never ends is a document with no heading in it; keeping
		// all of it as a summary would swallow the CV.
		rest = rest[:12]
	}
	return rest
}

var (
	// A name is two to four capitalised words, and carries no digits, no @
	// and no sentence punctuation. Anything looser and a job title qualifies.
	nameRe = regexp.MustCompile(`^(?:[A-ZÀ-ÖØ-Þ][\p{L}\p{M}'’\-]{1,20}(?:\s+(?:de|du|des|van|von|der|den|la|le|el|al|bin|da))?\s*){1,4}$`)
	// A job family is what a title ends in. Used only to tell a title from a
	// sentence, never to invent one.
	titleWords = []string{
		"engineer", "developer", "manager", "analyst", "consultant", "architect",
		"designer", "scientist", "researcher", "lead", "head", "director", "officer",
		"specialist", "technician", "administrator", "coordinator", "assistant",
		"professor", "teacher", "student", "intern", "freelance", "associate", "partner",
		"ingénieur", "developpeur", "développeur", "responsable", "analyste", "consultant",
		"architecte", "designer", "chercheur", "directeur", "responsable", "gestionnaire",
		"professeur", "enseignant", "étudiant", "stagiaire", "freelance", "associé",
	}
)

// cleanName accepts a line as a name or returns "".
func cleanName(line string) string {
	s := strings.TrimSpace(strings.Trim(line, " |•-–—"))
	// A name never carries a digit, a colon, a date or an address marker.
	if s == "" || len([]rune(s)) > 48 || strings.ContainsAny(s, "0123456789@:/") {
		return ""
	}
	if _, isHeading := sectionOf(s); isHeading {
		return ""
	}
	if !nameRe.MatchString(s) {
		return ""
	}
	// "Ingénieur logiciel" is two capitalised words too. The difference is
	// that a job title ends in a job word, so it is rejected here and picked
	// up as the headline instead.
	if endsWithTitleWord(s) {
		return ""
	}
	return s
}

func endsWithTitleWord(s string) bool {
	folded := layout.Fold(strings.ToLower(s))
	for _, w := range titleWords {
		if strings.HasSuffix(strings.TrimSpace(folded), " "+w) || strings.HasSuffix(strings.TrimSpace(folded), w) {
			return true
		}
	}
	return false
}

// cleanHeadline accepts a line as a title: it has to be short, and either carry
// a job word or be a couple of words with no sentence punctuation.
func cleanHeadline(line string) string {
	s := strings.TrimSpace(strings.Trim(line, " |•-–—"))
	if s == "" || len([]rune(s)) > 64 {
		return ""
	}
	if _, isHeading := sectionOf(s); isHeading {
		return ""
	}
	if strings.ContainsAny(s, "0123456789@") || hasDate(s) {
		return ""
	}
	if strings.Contains(s, ". ") || strings.HasSuffix(s, ".") {
		return ""
	}
	// A sentence of body text is not a title. Anything over ten words is prose.
	if len(strings.Fields(s)) > 10 {
		return ""
	}
	if endsWithTitleWord(s) || len(strings.Fields(s)) <= 6 {
		return s
	}
	return ""
}
