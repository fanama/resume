package fill

import (
	"regexp"
	"strings"

	"github.com/fanama/resume/atscv/internal/layout"
)

// A section is a heading and the lines under it. Reading a resume is mostly
// this: finding where a block starts and ends, which is a question about lines
// and not about geometry, because the extraction has already flattened the page.
type section struct {
	kind  string // one of the kinds below
	title string // the heading as printed, kept for the unmapped report
	lines []string
}

const (
	kindSummary     = "summary"
	kindExperience  = "experience"
	kindEducation   = "education"
	kindSkills      = "skills"
	kindLanguages   = "languages"
	kindProjects    = "projects"
	kindCerts       = "certifications"
	kindActivities  = "activities"
	kindPublication = "publications"
)

// headings maps a folded heading to a section kind. Both languages are listed
// for every kind, and the plurals and the two most common spellings, because a
// heading is the one line of a resume that is written freely.
//
// The keys are folded: EXPÉRIENCE, Expérience,经验和 "EXPERIENCE" all reach the
// same entry, since a PDF loses accents more often than a template does.
var headings = map[string]string{
	// profile
	"profil": kindSummary, "profile": kindSummary, "summary": kindSummary,
	"resume": kindSummary, "a propos": kindSummary, "about": kindSummary,
	"about me": kindSummary, "objectif": kindSummary, "objectif professionnel": kindSummary,
	"professional summary": kindSummary, "personal statement": kindSummary,
	"presentation": kindSummary, "accroche": kindSummary,

	// experience
	"experience": kindExperience, "experiences": kindExperience, "experience professionnelle": kindExperience,
	"professional experience": kindExperience, "work experience": kindExperience,
	"employment": kindExperience, "employment history": kindExperience,
	"career": kindExperience, "parcours": kindExperience, "parcours professionnel": kindExperience,
	"experiences professionnelles": kindExperience, "activite": kindActivities,
	"activites": kindActivities, "activity": kindActivities, "activities": kindActivities,
	"professional activities": kindActivities, "responsibilities": kindExperience,
	"poste": kindExperience, "postes": kindExperience, "jobs": kindExperience,

	// education
	"education": kindEducation, "formations": kindEducation, "formation": kindEducation,
	"diplomes": kindEducation, "diplome": kindEducation, "etudes": kindEducation,
	"etude": kindEducation, "scolarite": kindEducation,
	"academic background": kindEducation, "academic": kindEducation,
	"qualifications": kindEducation, "degre": kindEducation, "degree": kindEducation,
	"formation initiale": kindEducation, "parcours academique": kindEducation,

	// skills
	"skills": kindSkills, "competences": kindSkills, "competence": kindSkills,
	"technical skills": kindSkills, "competences techniques": kindSkills,
	"competences professionnelles": kindSkills, "expertise": kindSkills,
	"technologies": kindSkills, "technology": kindSkills, "stack": kindSkills,
	"tech stack": kindSkills, "outils": kindSkills, "tools": kindSkills,
	"mots cles": kindSkills, "hard skills": kindSkills, "professional skills": kindSkills,
	"langages": kindSkills, "programmation": kindSkills,

	// languages
	"languages": kindLanguages, "langues": kindLanguages, "langue": kindLanguages,
	"spoken languages": kindLanguages, "language": kindLanguages,

	// projects
	"projects": kindProjects, "projets": kindProjects, "projet": kindProjects,
	"personal projects": kindProjects, "side projects": kindProjects,
	"realisations": kindProjects, "realisation": kindProjects, "achievements": kindProjects,
	"portfolio": kindProjects, "side project": kindProjects,

	// certifications
	"certifications": kindCerts, "certification": kindCerts, "certifications et diplomes": kindCerts,
	"certificates": kindCerts, "licences": kindCerts, "licenses": kindCerts,
	"accreditations": kindCerts, "formations et certifications": kindCerts,

	// activities
	"activites associatives": kindActivities, "benevolat": kindActivities, "volunteering": kindActivities, "volunteer": kindActivities,
	"associations": kindActivities, "engagement": kindActivities,

	// publications, which the schema has no place for
	"publications": kindPublication, "publication": kindPublication,
	"papers": kindPublication, "articles": kindPublication,
	"references":      kindPublication,
	"recommandations": kindPublication, "interets": kindPublication,
	"interests": kindPublication, "hobbies": kindPublication, "centre d interet": kindPublication,
	"distinctions": kindPublication, "awards": kindPublication, "recompenses": kindPublication,
	"honors": kindPublication,
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
	kind, ok := headings[foldKey(cleaned)]
	if !ok {
		// The table holds the wordings that are written exactly; the patterns
		// hold the ones that are written freely. The profile section is the one
		// a candidate names in their own words, so it is matched on its shape.
		//
		// This runs before the numbering is stripped. trimNumbering removes a
		// leading enumerator by cutting up to three leading alphanumerics, which
		// also eats the first word of a short heading: "En bref" becomes "bref"
		// and "Qui suis-je" becomes "suis je". Matching the heading as printed
		// first keeps those intact.
		if kind, ok = patternKind(foldKey(cleaned)); ok {
			return kind, true
		}
		// A heading numbered "2. Experience" or "II. Formation" reads the same
		// once the numbering is gone.
		numbered := strings.TrimSpace(trimNumbering(cleaned))
		if kind, ok = headings[foldKey(numbered)]; !ok {
			// The patterns are only tried on the stripped form when what was
			// stripped really was an enumerator. trimNumbering cuts up to three
			// leading alphanumerics followed by a separator, which on "Job
			// Description" removes "Job" and leaves a heading the profile
			// pattern would accept. A job ad's description is not the
			// candidate's summary.
			if enum, isEnum := trimEnumerator(cleaned); isEnum {
				kind, ok = patternKind(foldKey(enum))
			}
		}
	}
	return kind, ok
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

// trimNumbering drops a leading enumerator: "1.", "1)", "2 -", "IV.", "• ".
// It walks runes rather than bytes, because the separators are multi-byte and a
// byte index would slice a character in half.
func trimNumbering(s string) string {
	s = strings.TrimSpace(s)
	for _, prefix := range []string{"•", "-", "–", "—", "*"} {
		s = strings.TrimSpace(strings.TrimPrefix(s, prefix))
	}
	r := []rune(s)
	i := 0
	for i < len(r) && i < 3 && isAlnum(r[i]) {
		i++
	}
	if i == 0 || i >= len(r) {
		return s
	}
	switch r[i] {
	case '.', ')', ':', '-', '–', '—', ' ':
		return strings.TrimSpace(string(r[i+1:]))
	}
	return s
}

func isAlnum(r rune) bool {
	return r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
}

// enumeratorRe matches a leading enumerator and captures what follows. Unlike
// trimNumbering it insists the prefix be digits or roman numerals, so an
// ordinary first word is not mistaken for a number: "1. Profil" is numbered,
// "Job Description" is not.
var enumeratorRe = regexp.MustCompile(`^(?:[•\-–—*]\s*)?(?:\d{1,2}|[ivxIVX]{1,4})[.)\:\-–—]\s*(.+)$`)

// trimEnumerator returns the heading without its enumerator, and whether it
// carried one.
func trimEnumerator(s string) (string, bool) {
	m := enumeratorRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return s, false
	}
	return strings.TrimSpace(m[1]), true
}

// splitSections cuts the document at its headings. Lines before the first
// heading are the identity block and are left to the caller; a heading with no
// lines under it still counts, so that "Références" is reported rather than lost.
func splitSections(lines []string) []section {
	var out []section
	var cur *section
	for _, line := range lines {
		kind, ok := sectionOf(line)
		if !ok {
			if cur != nil {
				cur.lines = append(cur.lines, line)
			}
			continue
		}
		if cur != nil {
			out = append(out, *cur)
		}
		cur = &section{kind: kind, title: strings.TrimSpace(line)}
	}
	if cur != nil {
		out = append(out, *cur)
	}
	return out
}

// firstHeadingIndex reports where the identity block ends, or len(lines) when
// the document has no heading at all.
func firstHeadingIndex(lines []string) int {
	for i, l := range lines {
		if _, ok := sectionOf(l); ok {
			return i
		}
	}
	return len(lines)
}
