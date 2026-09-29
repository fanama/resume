package fill

import (
	"regexp"
	"strings"

	"github.com/fanama/resume/atscv/internal/model"
)

// A bullet is a line that starts with a mark or a dash. Whether a line is one
// tells the entry builder what to do with it: a title and a company are placed
// in fields, a bullet becomes a highlight.
var (
	bulletRe = regexp.MustCompile(`^\s*[•▪◦‣·*\-–—]\s+`)
	// A skill list is a run of items separated by commas, pipes, middle dots or
	// semicolons. The French uses a comma and a middot, the English a comma.
	skillSepRe = regexp.MustCompile(`\s*[,;|·•]\s*|\s+/\s+|\s+et\s+|\s+and\s+`)
)

// readExperience fills the experience section. The shape it looks for is a title
// line carrying a period, then free lines under it, then the next title line.
func (f *filler) readExperience(s section) {
	for _, e := range f.entries(s.lines) {
		exp := model.Experience{
			Title:      e.title,
			Company:    e.company,
			Location:   e.location,
			Start:      e.start,
			End:        e.end,
			Highlights: e.bullets,
			Team:       e.team,
			Stack:      e.stack,
		}
		if s := description(e.notes); s != "" && exp.Summary == "" {
			exp.Summary = truncate(s, 400)
		}
		if exp.Title == "" && exp.Company == "" && len(exp.Highlights) == 0 {
			continue
		}
		f.resume.Experience = append(f.resume.Experience, exp)
	}
	if len(f.resume.Experience) == 0 {
		f.noteUnmapped(s)
	}
}

// readProjects fills the projects section. A project has a name and a period,
// and the same split as a job.
func (f *filler) readProjects(s section) {
	for _, e := range f.entries(s.lines) {
		p := model.Project{
			Name:         firstNonEmpty(e.title, e.company),
			Start:        e.start,
			End:          e.end,
			Summary:      truncate(description(e.notes), 400),
			Highlights:   e.bullets,
			Technologies: e.stack,
		}
		if p.Name == "" && p.Summary == "" && len(p.Highlights) == 0 {
			continue
		}
		f.resume.Projects = append(f.resume.Projects, p)
	}
	if len(f.resume.Projects) == 0 {
		f.noteUnmapped(s)
	}
}

// readActivities fills the association section, which is an experience with an
// organisation instead of a company.
func (f *filler) readActivities(s section) {
	for _, e := range f.entries(s.lines) {
		a := model.Activity{
			Name:         firstNonEmpty(e.title, e.company),
			Organization: e.company,
			Start:        e.start,
			End:          e.end,
			Summary:      truncate(description(e.notes), 400),
			Highlights:   e.bullets,
		}
		if a.Name == "" && a.Organization == "" {
			continue
		}
		f.resume.Activities = append(f.resume.Activities, a)
	}
	if len(f.resume.Activities) == 0 {
		f.noteUnmapped(s)
	}
}

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

// readSkills fills the skills section. A line of the form "Languages: Go, Rust"
// becomes a group with a category; lines that name no category are collected
// into one group instead of one group each.
//
// The grouping is what makes the section readable. A skills block is usually a
// handful of comma separated lines with no category at all, and appending a
// group per line produced a form with a dozen cards all headed "Compétences" —
// the same heading repeated is not a classification, it is the absence of one.
// A PDF also wraps a long line, so consecutive uncategorised lines are often
// one list that the page broke in two, and joining them puts it back together.
func (f *filler) readSkills(s section) {
	// loose holds the items from every line that named no category. They land
	// in a single group at the position of the first such line, so the order of
	// the section is kept.
	var loose []string
	looseAt := -1

	for _, line := range s.lines {
		items := splitItems(line)
		if len(items) == 0 {
			continue
		}
		category := ""
		// "Category: a, b, c" only when the part before the colon is short
		// enough to be a category and not a sentence.
		if head, rest, ok := strings.Cut(line, ":"); ok && len(rest) > 0 {
			if len([]rune(strings.TrimSpace(head))) <= 40 && !hasDate(head) {
				category = strings.TrimSpace(head)
				items = splitItems(rest)
			}
		}
		// A category carried in the same line, "Languages - Go, Rust".
		if category == "" {
			for _, sep := range []string{" - ", " – ", " — "} {
				if head, rest, ok := strings.Cut(line, sep); ok {
					if len([]rune(strings.TrimSpace(head))) <= 40 && len(splitItems(rest)) > 1 {
						category = strings.TrimSpace(head)
						items = splitItems(rest)
						break
					}
				}
			}
		}
		if len(items) == 0 {
			continue
		}
		if category == "" {
			if looseAt < 0 {
				looseAt = len(f.resume.Skills)
				// The slot is reserved now and filled once every line is read,
				// so that a category stated after a bare list keeps its place.
				f.resume.Skills = append(f.resume.Skills, model.SkillGroup{})
			}
			loose = append(loose, items...)
			continue
		}
		// A category can be stated twice, once per line, when the extraction
		// split it. The items join the group that already carries the name
		// rather than opening a second one under the same heading.
		if i := indexOfCategory(f.resume.Skills, category); i >= 0 {
			f.resume.Skills[i].Items = appendUnique(f.resume.Skills[i].Items, items)
			continue
		}
		f.resume.Skills = append(f.resume.Skills, model.SkillGroup{Category: category, Items: items})
	}

	if looseAt >= 0 {
		f.resume.Skills[looseAt] = model.SkillGroup{
			Category: skillGroupLabel(f.lang),
			Items:    appendUnique(nil, loose),
		}
	}
	if len(f.resume.Skills) == 0 {
		f.noteUnmapped(s)
	}
}

// indexOfCategory finds a group already headed by name, compared the way a
// heading is compared: case and accents are what an extraction damages first.
func indexOfCategory(groups []model.SkillGroup, name string) int {
	key := foldKey(name)
	for i := range groups {
		if groups[i].Category != "" && foldKey(groups[i].Category) == key {
			return i
		}
	}
	return -1
}

// appendUnique adds the items that are not already in the list. A skill named
// twice is a duplicate the candidate would have to delete by hand, and the
// wrapping of a PDF line is a common way to produce one.
func appendUnique(dst []string, items []string) []string {
	seen := make(map[string]bool, len(dst)+len(items))
	for _, s := range dst {
		seen[foldKey(s)] = true
	}
	for _, s := range items {
		k := foldKey(s)
		if k == "" || seen[k] {
			continue
		}
		seen[k] = true
		dst = append(dst, s)
	}
	return dst
}

func skillGroupLabel(lang model.Lang) string {
	if lang == model.LangEN {
		return "Skills"
	}
	return "Compétences"
}

// splitItems breaks a skill line into items, dropping a leading category and
// the noise an extraction leaves on the last one.
func splitItems(line string) []string {
	line = strings.TrimSpace(bulletRe.ReplaceAllString(line, ""))
	if line == "" {
		return nil
	}
	parts := skillSepRe.Split(line, -1)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = cleanItem(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

// cleanItem drops the punctuation an extraction leaves around an item and
// rejects what cannot be a skill: a date, a very long phrase, a line of prose.
func cleanItem(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, " \t•·|-–—,;:/")
	s = strings.TrimSpace(s)
	if s == "" || len([]rune(s)) > 40 {
		return ""
	}
	if hasDate(s) {
		return ""
	}
	// A phrase of six words or more is a sentence, not a skill.
	if len(strings.Fields(s)) > 6 {
		return ""
	}
	return s
}

// readCertifications fills the certifications section. A line is an
// organisation and a date, which is most of what the schema holds.
func (f *filler) readCertifications(s section) {
	for _, line := range s.lines {
		name, issuer, date := parseCertification(line)
		if name == "" {
			continue
		}
		f.resume.Certifications = append(f.resume.Certifications, model.Certification{
			Name:   name,
			Issuer: issuer,
			Date:   date,
		})
	}
	if len(f.resume.Certifications) == 0 {
		f.noteUnmapped(s)
	}
}

// parseCertification reads "Name, Issuer, 2020" and its variants. The issuer is
// only what sits after a comma, never a guess: a certification without an
// issuer is incomplete but honest.
func parseCertification(line string) (name, issuer, date string) {
	s := strings.TrimSpace(bulletRe.ReplaceAllString(line, ""))
	if s == "" {
		return "", "", ""
	}
	if d, _, ok := splitPeriod(s); ok {
		date = d
		s = strings.TrimSpace(stripPeriod(s))
	}
	parts := strings.Split(s, ",")
	name = cleanItem(parts[0])
	if len(parts) > 1 {
		issuer = cleanItem(parts[1])
	}
	if name == "" || len([]rune(name)) > 60 {
		return "", "", ""
	}
	return name, issuer, date
}

// stripPeriod removes everything a period is made of, leaving the text around
// it: the dates, the words that say the period is still open, and the connector
// between a start and an end. All three have to go together, or the " - " that
// surrounded the dates is left behind and reads as a field separator.
func stripPeriod(s string) string {
	// Decided before anything is removed: once the dates are gone there is
	// nothing left to tell a connector from an ordinary word.
	hadDate := len(dateSpans(s)) > 0
	for _, span := range dateSpans(s) {
		if i := strings.Index(s, span); i >= 0 {
			s = s[:i] + " " + s[i+len(span):]
		}
	}
	// The connectors are only removed once a date was found, so that a word
	// like "to" inside a job title survives.
	if hadDate {
		s = periodConnector.ReplaceAllString(s, " ")
	}
	return strings.TrimSpace(strings.Join(strings.Fields(s), " "))
}

// periodConnector matches the words that join a start to an end, in both
// languages, including the open ended wording.
var periodConnector = regexp.MustCompile(`(?i)\b(?:de|a|à|au|from|to|until|through|depuis|jusqu|aujourd'hui|aujourdhui|present|presentement?|en cours|actuel|actuelle|current|now|ongoing)\b`)

// description joins a note block into the paragraph the schema holds.
//
// It used to return the first line and drop the rest, which lost the body of
// every description longer than one line: the candidate wrote three sentences
// about a job and the form showed one. The block is the description, so the
// block is what is kept, with the lines the page broke rejoined.
func description(lines []string) string {
	var out []string
	for _, l := range lines {
		l = strings.TrimSpace(bulletRe.ReplaceAllString(l, ""))
		if l == "" {
			continue
		}
		// A line the previous one runs into is the rest of that sentence.
		if n := len(out); n > 0 && continuesSentence(out[n-1]) {
			out[n-1] = joinWrapped(out[n-1], l)
			continue
		}
		out = append(out, l)
	}
	return strings.Join(out, " ")
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
