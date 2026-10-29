package fill

import (
	"strings"

	"github.com/fanama/resume/atscv/internal/model"
)

// readEducation fills the education section. A school is recognised by the words
// that name one, and a degree by the words that name one; neither is guessed
// from the shape of the line alone.
var (
	schoolWords = []string{
		"university", "universite", "école", "ecole", "school", "college", "institute",
		"lycée", "lycee", "faculte", "faculté", "académie", "academie", "hochschule",
		"universidad", "universita", "sup", "iut", "ge", "ens", "polytechnique",
		"sorbonne", "campus", "academy", "bootcamp", "formation",
	}
	degreeWords = []string{
		"master", "mba", "bachelor", "licence", "license", "diplome", "diplôme", "doctorat",
		"phd", "degree", "bsc", "msc", "eng", "btech", "mtech", "certificat", "certificate",
		"baccalauréat", "baccalaureat", "deug", "dut", "duts", "tsvt", "classes preparees",
	}
)

func (f *filler) readEducation(s section) {
	for _, e := range f.entries(s.lines) {
		ed := model.Education{
			Degree: firstNonEmpty(degreeOf(e), e.title),
			School: firstNonEmpty(schoolOf(e), e.company),
			Start:  e.start,
			End:    e.end,
			// What sits under a degree is the coursework: the modules, the
			// thesis, the specialisation. It used to be dropped on the floor,
			// because only the notes were read and the bullets were not, so a
			// degree with four listed modules imported as a bare line.
			Summary:    truncate(description(e.notes), 400),
			Coursework: e.bullets,
		}
		if ed.Degree == "" && ed.School == "" {
			continue
		}
		f.resume.Education = append(f.resume.Education, ed)
	}
	if len(f.resume.Education) == 0 {
		f.noteUnmapped(s)
	}
}

// degreeOf returns the part of a title line that names a degree.
func degreeOf(e entry) string {
	for _, part := range []string{e.title, e.company} {
		if hasWord(part, degreeWords) {
			return strings.TrimSpace(part)
		}
	}
	return ""
}

// schoolOf returns the part of a title line that names a school.
//
// The company is tried first, unlike degreeOf, because a degree often contains
// a school word itself: "Classe préparatoire aux grandes écoles" names no
// school, and scanning the title first returned it as both the degree and the
// school of the same entry.
func schoolOf(e entry) string {
	for _, part := range []string{e.company, e.title} {
		if hasWord(part, schoolWords) {
			return strings.TrimSpace(part)
		}
	}
	return ""
}

func hasWord(s string, words []string) bool {
	if s == "" {
		return false
	}
	f := " " + layoutFold(s) + " "
	for _, w := range words {
		if strings.Contains(f, " "+layoutFold(w)) {
			return true
		}
	}
	return false
}
