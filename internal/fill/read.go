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

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
