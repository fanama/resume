package fill

import (
	"strings"

	"github.com/fanama/resume/atscv/internal/model"
)

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
