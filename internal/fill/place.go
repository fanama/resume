package fill

import (
	"regexp"
	"strings"

	"github.com/fanama/resume/atscv/internal/layout"
)

// A place field: one to four words, each starting with a capital, no digit
// and no punctuation inside. Requiring the capital on every word is what
// keeps a sentence out, and a job title such as "Ingénieure logiciel" is
// already excluded because its second word is lower case.
var placeRe = regexp.MustCompile(`^[\p{Lu}][\p{L}\p{M}'’\-]*(?: [\p{Lu}][\p{L}\p{M}'’\-]*){0,3}$`)

// readPlace reads a city and a country. A gazetteer is the only reliable way
// and this tool has none, so the reading is deliberately narrow: a country has
// to be a country this list knows, and the city has to be the capitalised word
// just before it. Both are taken out of the line, since a city left in the text
// would be read as the first line of a job.
func (f *filler) readPlace(residual []string) {
	if f.resume.Contact.Country != "" {
		return
	}
	for i, line := range residual {
		parts := strings.FieldsFunc(line, func(r rune) bool {
			return r == '|' || r == ',' || r == ';' || r == '·'
		})
		for j, p := range parts {
			if !isCountry(p) {
				continue
			}
			country := p
			city := ""
			// The nearest preceding field that looks like a place name.
			for k := j - 1; k >= 0; k-- {
				if placeRe.MatchString(strings.TrimSpace(parts[k])) {
					city = strings.TrimSpace(parts[k])
					break
				}
			}
			if city == "" {
				continue
			}
			f.resume.Contact.Country = titleCase(country)
			f.resume.Contact.City = titleCase(city)
			residual[i] = strings.Replace(residual[i], city, " ", 1)
			residual[i] = strings.Replace(residual[i], country, " ", 1)
			return
		}
	}
}

func isCountry(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "france", "belgique", "belgium", "suisse", "switzerland", "luxembourg",
		"canada", "allemagne", "germany", "espagne", "spain", "italie", "italy",
		"portugal", "pays-bas", "netherlands", "royaume-uni", "united kingdom",
		"maroc", "morocco", "senegal", "tunisie", "cote d'ivoire",
		"etats-unis", "united states", "monde", "world", "europe",
		"international", "remote", "teletravail":
		return true
	}
	return false
}

func titleCase(s string) string {
	parts := strings.Fields(s)
	for i, p := range parts {
		r := []rune(p)
		if len(r) == 0 {
			continue
		}
		parts[i] = strings.ToUpper(string(r[0])) + string(r[1:])
	}
	return strings.Join(parts, " ")
}

/* placeIndex returns the index of the part that names a place, or -1.
 *
 * A gazetteer is the only complete answer to "is this word a city", and this
 * tool has none, so the document is asked instead. Three answers, in order of
 * how much they cost to be wrong:
 *
 *   - a word the header of this same document states is a place, because a
 *     resume repeats its author's city on the lines of the jobs;
 *   - a country this tool knows is a place wherever it appears.
 *
 * A lone capitalised word is deliberately not one of them. "Nexteo" and "Paris"
 * are spelled the same way, and reading every one of them as a city turns the
 * single word companies of half the job market into locations. */
func (f *filler) placeIndex(parts []string) int {
	for i, p := range parts {
		folded := layout.Fold(clean(p))
		if folded == "" {
			continue
		}
		if isCountry(clean(p)) {
			return i
		}
		for _, h := range f.headerFields {
			if folded == h {
				return i
			}
		}
	}
	return -1
}

// headerFields are the folded fields the header of the document states, read
// before the sections so that the entries can ask what the header claimed.
func headerFields(head []string) []string {
	var out []string
	for _, line := range head {
		for _, p := range strings.FieldsFunc(line, func(r rune) bool {
			return r == '|' || r == ',' || r == ';' || r == '·' || r == '\t'
		}) {
			p = strings.TrimSpace(p)
			// An address, a link and a line with digits in it are contact data,
			// not a place, and a long field is a sentence.
			if p == "" || len([]rune(p)) > 24 {
				continue
			}
			if strings.ContainsAny(p, "@/0123456789") {
				continue
			}
			out = append(out, layout.Fold(p))
		}
	}
	return out
}

// headerOf returns the lines before the first section heading, which is where a
// resume states who the author is and where they are.
func headerOf(lines []string) []string {
	n := firstHeadingIndex(lines)
	return lines[:n]
}
