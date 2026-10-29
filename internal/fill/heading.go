package fill

import (
	"regexp"
	"strings"

	"github.com/fanama/resume/atscv/internal/layout"
)

// IsHeading reports whether a line is a section heading, for the packages that
// cut a document at its headings and read the lines between them.
func IsHeading(line string) bool {
	_, ok := sectionOf(line)
	return ok
}

// sectionOf reports the kind of a line that is a heading, and whether it is one.
// A heading is short, has no terminal punctuation and matches the table after
// folding: three conditions, because a line of body text that happens to start
// with the word "experience" is not a section.
func sectionOf(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || len([]rune(trimmed)) > 48 {
		return "", false
	}
	// A heading never ends a sentence, and never carries a colon: "Skills:" is
	// a heading, "Skills: Python, Go" is a list.
	cleaned := strings.TrimSpace(strings.Trim(trimmed, " .:•|-–—"))
	if cleaned == "" {
		return "", false
	}
	return sectionKind(cleaned)
}

// sectionKind matches a cleaned heading against the table of exact wordings,
// then against the patterns, then against the same heading without numbering.
func sectionKind(cleaned string) (string, bool) {
	if kind, ok := headings[foldKey(cleaned)]; ok {
		return kind, true
	}
	// The table holds the wordings that are written exactly; the patterns
	// hold the ones that are written freely. The profile section is the one
	// a candidate names in their own words, so it is matched on its shape.
	//
	// This runs before the numbering is stripped. trimNumbering removes a
	// leading enumerator by cutting up to three leading alphanumerics, which
	// also eats the first word of a short heading: "En bref" becomes "bref"
	// and "Qui suis-je" becomes "suis je". Matching the heading as printed
	// first keeps those intact.
	if kind, ok := patternKind(foldKey(cleaned)); ok {
		return kind, true
	}
	// A heading numbered "2. Experience" or "II. Formation" reads the same
	// once the numbering is gone.
	numbered := strings.TrimSpace(trimNumbering(cleaned))
	if kind, ok := headings[foldKey(numbered)]; ok {
		return kind, true
	}
	// The patterns are only tried on the stripped form when what was stripped
	// really was an enumerator. trimNumbering cuts up to three leading
	// alphanumerics followed by a separator, which on "Job Description"
	// removes "Job" and leaves a heading the profile pattern would accept. A
	// job ad's description is not the candidate's summary.
	if enum, isEnum := trimEnumerator(cleaned); isEnum {
		return patternKind(foldKey(enum))
	}
	return "", false
}

// summaryRe matches the headings that open a resume with a paragraph about the
// candidate. That section is named more freely than any other: "Profil", "À
// propos de moi", "Professional Summary", "Description", "En bref" are all the
// same block, and listing every wording is a losing game. The shape is what
// they share, so the shape is what is matched.
//
// The pattern is anchored and the qualifiers are enumerated rather than left
// open, because the risk here is the opposite of a miss: "Description du poste"
// and "Profil recherché" are job-ad wordings that appear inside a resume, and
// reading one as the candidate's summary would put the wrong paragraph at the
// top of the document. Only a qualifier that says "this is about me" is allowed
// to follow the noun.
var summaryRe = regexp.MustCompile(`^(?:` +
	// "profil", "profile", "mon profil", "personal profile", "profil pro"
	`(?:mon |my |personal |professional |short |brief |candidate )?profil(?:e)?` +
	`(?: (?:professionnel(?:le)?|professional|personnel(?:le)?|personal|summary|court|bref))?` +
	`|` +
	// "a propos", "a propos de moi", "about", "about me", "about myself"
	`(?:a propos(?: de (?:moi|mon parcours))?|about(?: (?:me|myself))?)` +
	`|` +
	// "summary", "professional summary", "career summary", "summary of qualifications"
	`(?:(?:professional|career|executive|personal|profile|short|brief) )?summar(?:y|ies)` +
	`(?: of qualifications)?` +
	`|` +
	// "description", but only on its own or said of the person
	`description(?: (?:personnelle|professionnelle|personal|professional))?` +
	`|` +
	// the short French and English openers that mean the same block
	`(?:presentation|accroche|en bref|bref apercu|apercu|overview|bio|biographie)` +
	`(?: (?:personnelle|professionnelle|general|generale))?` +
	`|` +
	`qui suis je` +
	`|` +
	// "objectif", "objectif professionnel", "career objective"
	`(?:(?:career|professional) )?objectif?(?:ve)?` +
	`(?: (?:professionnel(?:le)?|de carriere|professional))?` +
	`|` +
	`(?:personal|professional) statement` +
	`)$`)

// patternKind matches a folded heading against the shapes that are not worth
// enumerating. It takes the already folded key so that the patterns can be
// written in plain ASCII and still match "À PROPOS" or "Aperçu".
func patternKind(key string) (string, bool) {
	if key == "" {
		return "", false
	}
	if summaryRe.MatchString(key) {
		return kindSummary, true
	}
	return "", false
}

// foldKey reduces a heading to the form used as a table key: no accents, no
// case, no punctuation around, single spaces.
func foldKey(s string) string {
	s = layout.Fold(strings.ToLower(s))
	var b strings.Builder
	space := false
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '-' || r == '_' || r == '.' || r == ':' || r == '\'' || r == '\u2019' {
			space = true
			continue
		}
		if space && b.Len() > 0 {
			b.WriteByte(' ')
		}
		space = false
		b.WriteRune(r)
	}
	return b.String()
}
