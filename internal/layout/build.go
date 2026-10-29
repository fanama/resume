package layout

import (
	"strings"

	"github.com/fanama/resume/atscv/internal/model"
)

func (l *Layout) build(r *model.Resume) {
	lab := l.labels()

	l.add(Block{Kind: KindName, Text: r.Name})
	if r.Headline != "" {
		l.add(Block{Kind: KindHeadline, Text: r.Headline})
	}
	for _, line := range contactLines(r.Contact) {
		l.add(Block{Kind: KindContact, Text: line})
	}

	if r.Summary != "" {
		l.section(lab.Summary)
		l.add(Block{Kind: KindSummary, Text: r.Summary})
	}

	l.buildExperience(r.Experience, lab)
	l.buildEducation(r.Education, lab)
	l.buildSkills(r.Skills, lab)
	l.buildCertifications(r.Certifications, lab)
	l.buildProjects(r.Projects, lab)
	l.buildActivities(r.Activities, lab)
	l.buildLanguages(r.Languages, lab)
}

// buildExperience writes the job entries.
func (l *Layout) buildExperience(entries []model.Experience, lab model.Labels) {
	if len(entries) == 0 {
		return
	}
	l.section(lab.Experience)
	for i, e := range entries {
		l.add(Block{Kind: KindEntryTitle, Text: e.Title, Bold: e.Title})
		if m := l.metaLine(e.Company, e.Location, e.Start, e.End); m != "" {
			l.add(Block{Kind: KindEntryMeta, Text: m})
		}
		if e.Summary != "" {
			l.add(Block{Kind: KindSummary, Text: e.Summary})
		}
		for _, h := range e.Highlights {
			l.add(Block{Kind: KindBullet, Text: h, Bullet: true})
		}
		if e.Team != "" {
			l.add(Block{Kind: KindField, Bold: lab.Team, Text: e.Team})
		}
		if len(e.Stack) > 0 {
			l.add(Block{Kind: KindField, Bold: lab.Technologies, Text: strings.Join(e.Stack, ", ")})
		}
		if i < len(entries)-1 {
			l.spacer()
		}
	}
}

// buildEducation writes the education entries.
func (l *Layout) buildEducation(entries []model.Education, lab model.Labels) {
	if len(entries) == 0 {
		return
	}
	l.section(lab.Education)
	for i, e := range entries {
		l.add(Block{Kind: KindEntryTitle, Text: e.Degree, Bold: e.Degree})
		if m := l.metaLine(e.School, e.Location, e.Start, e.End); m != "" {
			l.add(Block{Kind: KindEntryMeta, Text: m})
		}
		if e.Summary != "" {
			l.add(Block{Kind: KindSummary, Text: e.Summary})
		}
		if len(e.Coursework) > 0 {
			l.add(Block{Kind: KindField, Bold: lab.CourseworkLabel, Text: strings.Join(e.Coursework, ", ")})
		}
		if i < len(entries)-1 {
			l.spacer()
		}
	}
}

// buildSkills writes one labelled line per skill group.
func (l *Layout) buildSkills(groups []model.SkillGroup, lab model.Labels) {
	if len(groups) == 0 {
		return
	}
	l.section(lab.Skills)
	for _, g := range groups {
		text := strings.Join(g.Items, ", ")
		if g.Category != "" {
			l.add(Block{Kind: KindField, Bold: g.Category, Text: text})
		} else {
			l.add(Block{Kind: KindField, Text: text})
		}
	}
}

// buildCertifications writes the certifications, with their issuer and date.
func (l *Layout) buildCertifications(entries []model.Certification, lab model.Labels) {
	if len(entries) == 0 {
		return
	}
	l.section(lab.Certifications)
	for _, c := range entries {
		bits := []string{}
		if c.Issuer != "" {
			bits = append(bits, c.Issuer)
		}
		if c.Date != "" {
			bits = append(bits, c.Date)
		}
		if c.ID != "" {
			bits = append(bits, c.ID)
		}
		l.add(Block{Kind: KindEntryTitle, Text: c.Name, Bold: c.Name, Dim: strings.Join(bits, " | ")})
	}
}

// buildProjects writes the project entries.
func (l *Layout) buildProjects(entries []model.Project, lab model.Labels) {
	if len(entries) == 0 {
		return
	}
	l.section(lab.Projects)
	for i, p := range entries {
		l.add(Block{Kind: KindEntryTitle, Text: p.Name, Bold: p.Name})
		if m := l.metaLine(p.URL, "", p.Start, p.End); m != "" {
			l.add(Block{Kind: KindEntryMeta, Text: m})
		}
		if p.Summary != "" {
			l.add(Block{Kind: KindSummary, Text: p.Summary})
		}
		for _, h := range p.Highlights {
			l.add(Block{Kind: KindBullet, Text: h, Bullet: true})
		}
		if len(p.Technologies) > 0 {
			l.add(Block{Kind: KindField, Bold: lab.Technologies, Text: strings.Join(p.Technologies, ", ")})
		}
		if i < len(entries)-1 {
			l.spacer()
		}
	}
}

// buildActivities writes the association entries.
func (l *Layout) buildActivities(entries []model.Activity, lab model.Labels) {
	if len(entries) == 0 {
		return
	}
	l.section(lab.Activities)
	for i, a := range entries {
		l.add(Block{Kind: KindEntryTitle, Text: a.Name, Bold: a.Name})
		if m := l.metaLine(a.Organization, a.Location, a.Start, a.End); m != "" {
			l.add(Block{Kind: KindEntryMeta, Text: m})
		}
		if a.Summary != "" {
			l.add(Block{Kind: KindSummary, Text: a.Summary})
		}
		for _, h := range a.Highlights {
			l.add(Block{Kind: KindBullet, Text: h, Bullet: true})
		}
		if i < len(entries)-1 {
			l.spacer()
		}
	}
}

// buildLanguages writes the spoken languages as one labelled line.
func (l *Layout) buildLanguages(entries []model.SpokenLanguage, lab model.Labels) {
	if len(entries) == 0 {
		return
	}
	l.section(lab.Languages)
	items := make([]string, 0, len(entries))
	for _, s := range entries {
		if s.Level != "" {
			items = append(items, s.Name+" ("+s.Level+")")
		} else {
			items = append(items, s.Name)
		}
	}
	l.add(Block{Kind: KindField, Text: strings.Join(items, ", ")})
}

// contactLines builds the header block: one plain-text line per group of
// details, separated by pipes. No icons, no glyphs, no hidden labels.
func contactLines(c model.Contact) []string {
	var lines []string
	var identity []string
	if loc := joinLocation(c.City, c.Region, c.Country); loc != "" {
		identity = append(identity, loc)
	}
	if c.Email != "" {
		identity = append(identity, c.Email)
	}
	if c.Phone != "" {
		identity = append(identity, c.Phone)
	}
	if len(identity) > 0 {
		lines = append(lines, strings.Join(identity, " | "))
	}
	var links []string
	for _, lk := range c.Links {
		if lk.URL != "" {
			links = append(links, trimScheme(lk.URL))
		} else {
			links = append(links, lk.Label)
		}
	}
	// Split links over several lines so that no line becomes unreadably long.
	const perLine = 2
	for i := 0; i < len(links); i += perLine {
		end := i + perLine
		if end > len(links) {
			end = len(links)
		}
		lines = append(lines, strings.Join(links[i:end], " | "))
	}
	if c.Note != "" {
		lines = append(lines, c.Note)
	}
	return lines
}

// metaLine joins organization, location and the date range, the line every ATS
// expects under a job title.
func (l *Layout) metaLine(org, location, start, end string) string {
	var parts []string
	if org != "" {
		parts = append(parts, org)
	}
	if location != "" {
		parts = append(parts, location)
	}
	head := strings.Join(parts, ", ")
	if period := l.period(start, end); period != "" {
		if head == "" {
			return period
		}
		return head + " | " + period
	}
	return head
}
