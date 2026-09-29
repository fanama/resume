package fill

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/fanama/resume/atscv/internal/layout"
	"github.com/fanama/resume/atscv/internal/model"
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

// Heading counts decide the language, so the two lists must hold only headings
// that are decisive in one direction. A heading both languages share, such as
// "experience" or "projects", is in neither: counting it in both cancels out and
// makes the document look French or English at random.
var (
	frenchOnly = map[string]int{
		"parcours": 1, "parcours professionnel": 1, "poste": 1, "postes": 1,
		"formation": 1, "formations": 1, "diplome": 1, "diplomes": 1, "etudes": 1,
		"etude": 1, "competences": 1, "competence": 1, "competences techniques": 1,
		"competences professionnelles": 1, "langues": 1, "langue": 1,
		"realisations": 1, "realisation": 1, "profil": 1, "a propos": 1,
		"benevolat": 1, "activites": 1, "activites associatives": 1,
		"scolarite": 1, "mots cles": 1, "technologies": 1, "outils": 1,
		"parcours academique": 1, "formation initiale": 1, "centre d interet": 1,
		"distinctions": 1, "recompenses": 1, "accroche": 1,
		"experience professionnelle": 1, "experiences professionnelles": 1,
		"formations et certifications": 1, "certifications et diplomes": 1,
	}
	englishOnly = map[string]int{
		"work experience": 1, "professional experience": 1, "employment": 1,
		"employment history": 1, "career": 1, "education": 1, "skills": 1,
		"technical skills": 1, "hard skills": 1, "languages": 1,
		"professional skills": 1, "side projects": 1, "personal projects": 1,
		"academic background": 1, "academic": 1, "qualifications": 1,
		"volunteering": 1, "volunteer": 1, "activities": 1,
		"publications": 1, "papers": 1, "articles": 1, "references": 1,
		"interests": 1, "hobbies": 1, "awards": 1, "honors": 1,
		"professional summary": 1, "personal statement": 1, "about": 1,
		"about me": 1, "profile": 1, "summary": 1, "tech stack": 1,
		"stack": 1, "tools": 1, "degree": 1, "spoken languages": 1,
	}
	// Words that carry the language on their own, used when the headings tie.
	// They are counted on the whole document because a CV states its work in
	// prose, and prose is long.
	frenchWords  = []string{" et ", " avec ", " chez ", " dont ", " pour ", " dans ", " une ", " sur "}
	englishWords = []string{" and ", " with ", " at ", " for ", " from ", " the ", " of "}
)

// guessLang decides which language the headings are written in. The caller may
// already know it, from the editor's own setting, and then nothing is guessed.
//
// Three passes, cheapest first: the headings that only one language uses, then
// the function words of the prose, and only then the tool's default. A single
// "EXPERIENCE" heading decides nothing, which is why the second pass exists.
func guessLang(lines []string, want model.Lang) model.Lang {
	if want != "" {
		return want
	}
	var fr, en int
	for _, l := range lines {
		// Only headings count here: body text mentions both languages at once,
		// and one English word in a French CV is not a language signal.
		key := foldKey(strings.Trim(strings.TrimSpace(l), " .:•|-–—"))
		if _, ok := headings[key]; !ok {
			key = foldKey(trimNumbering(strings.Trim(strings.TrimSpace(l), " .:•|-–—")))
		}
		fr += frenchOnly[key]
		en += englishOnly[key]
	}
	switch {
	case fr > en:
		return model.LangFR
	case en > fr:
		return model.LangEN
	}

	joined := " " + layoutFold(strings.ToLower(strings.Join(lines, " "))) + " "
	for _, w := range frenchWords {
		fr += strings.Count(joined, w)
	}
	for _, w := range englishWords {
		en += strings.Count(joined, w)
	}
	switch {
	case fr > en:
		return model.LangFR
	case en > fr:
		return model.LangEN
	}
	// Nothing to go on. French is the tool's default and the language of the
	// documentation, so an undecidable document is read as French.
	return model.LangFR
}

// readSection fills one section from the lines under its heading.
func (f *filler) readSection(s section) {
	switch s.kind {
	case kindSummary:
		f.readSummary(s)
	case kindExperience:
		f.readExperience(s)
	case kindEducation:
		f.readEducation(s)
	case kindSkills:
		f.readSkills(s)
	case kindLanguages:
		f.readLanguages(s)
	case kindProjects:
		f.readProjects(s)
	case kindCerts:
		f.readCertifications(s)
	case kindActivities:
		f.readActivities(s)
	case kindPublication:
		// The schema has no field for a publication list or a reference list.
		// The heading is reported so the candidate knows the content exists
		// somewhere and did not make it into the draft.
		f.noteUnmapped(s)
	}
}

func (f *filler) noteUnmapped(s section) {
	if len(s.lines) == 0 && s.title == "" {
		return
	}
	for _, u := range f.unmapped {
		if u == s.title {
			return
		}
	}
	f.unmapped = append(f.unmapped, s.title)
}

func (f *filler) readSummary(s section) {
	if len(s.lines) == 0 {
		f.noteUnmapped(s)
		return
	}
	if f.resume.Summary == "" {
		f.resume.Summary = truncate(joinSentences(s.lines), 1200)
	}
}

func joinSentences(lines []string) string {
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		l = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(l), "•-–—*"))
		if l != "" {
			out = append(out, l)
		}
	}
	return strings.Join(out, " ")
}

// readLanguages reads "Français : natif" pairs, the one section whose shape is
// regular enough to be read without guessing a value.
func (f *filler) readLanguages(s section) {
	for _, line := range s.lines {
		name, level, ok := strings.Cut(line, ":")
		if !ok {
			name, level, ok = strings.Cut(line, " - ")
		}
		if !ok {
			continue
		}
		n := strings.TrimSpace(name)
		l := strings.TrimSpace(level)
		if n == "" || l == "" || len([]rune(n)) > 24 || isDecorative(l) {
			continue
		}
		if !looksLikeLanguage(n) {
			continue
		}
		f.resume.Languages = append(f.resume.Languages, model.SpokenLanguage{Name: n, Level: l})
	}
	if len(f.resume.Languages) == 0 {
		f.noteUnmapped(s)
	}
}

var languageNames = []string{
	"francais", "français", "anglais", "allemand", "espagnol", "italien", "portugais",
	"neerlandais", "neerlan", "russe", "chinois", "japonais", "arabe", "polonais",
	"hongrois", "tchèque", "grec", "suédois", "danois", "norvégien", "finnois",
	"ukrainien", "roumain", "bulgare", "catalan", "basque", "breton", "latin",
	"french", "english", "german", "spanish", "italian", "portuguese", "dutch",
	"russian", "chinese", "japanese", "arabic", "polish", "hindi", "korean",
}

func looksLikeLanguage(s string) bool {
	f := layout.Fold(strings.ToLower(strings.TrimSpace(s)))
	for _, n := range languageNames {
		if f == n || strings.HasPrefix(f, n) {
			return true
		}
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

// dateSpans lists the date-like fragments of a line, in order, with the
// overlap resolved so that "01/2020" is not also read as "2020".
// span is a date-like fragment located in a line, so that two regexes matching
// the same text do not both count.
type span struct {
	start, end int
	text       string
}

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
