package fill

import (
	"strings"

	"github.com/fanama/resume/atscv/internal/layout"
	"github.com/fanama/resume/atscv/internal/model"
)

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
