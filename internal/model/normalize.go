package model

import "strings"

// Normalize trims every string and drops empty entries so that renderers and
// the linter never have to test for blank fields.
func (r *Resume) Normalize() {
	r.normalizeHeader()

	r.Experience = cleanSlice(r.Experience, normalizeExperience)
	r.Education = cleanSlice(r.Education, normalizeEducation)
	r.Skills = cleanSlice(r.Skills, normalizeSkillGroup)
	r.Certifications = cleanSlice(r.Certifications, normalizeCertification)
	r.Projects = cleanSlice(r.Projects, normalizeProject)
	r.Activities = cleanSlice(r.Activities, normalizeActivity)
	r.Languages = cleanSlice(r.Languages, normalizeSpokenLanguage)
}

// normalizeHeader trims the identity block and its links.
func (r *Resume) normalizeHeader() {
	r.Name = clean(r.Name)
	r.Headline = clean(r.Headline)
	r.Summary = clean(r.Summary)

	r.Contact.Email = clean(r.Contact.Email)
	r.Contact.Phone = clean(r.Contact.Phone)
	r.Contact.City = clean(r.Contact.City)
	r.Contact.Region = clean(r.Contact.Region)
	r.Contact.Country = clean(r.Contact.Country)
	r.Contact.Note = clean(r.Contact.Note)
	r.Contact.Links = cleanLinks(r.Contact.Links)
}

func normalizeExperience(e *Experience) {
	e.Title = clean(e.Title)
	e.Company = clean(e.Company)
	e.Location = clean(e.Location)
	e.Start = clean(e.Start)
	e.End = clean(e.End)
	e.Summary = clean(e.Summary)
	e.Team = clean(e.Team)
	e.Highlights = cleanList(e.Highlights)
	e.Stack = cleanList(e.Stack)
}

func normalizeEducation(e *Education) {
	e.Degree = clean(e.Degree)
	e.School = clean(e.School)
	e.Location = clean(e.Location)
	e.Start = clean(e.Start)
	e.End = clean(e.End)
	e.Summary = clean(e.Summary)
	e.Coursework = cleanList(e.Coursework)
}

func normalizeSkillGroup(s *SkillGroup) {
	s.Category = clean(s.Category)
	s.Items = cleanList(s.Items)
}

func normalizeCertification(c *Certification) {
	c.Name = clean(c.Name)
	c.Issuer = clean(c.Issuer)
	c.Date = clean(c.Date)
	c.ID = clean(c.ID)
}

func normalizeProject(p *Project) {
	p.Name = clean(p.Name)
	p.URL = clean(p.URL)
	p.Start = clean(p.Start)
	p.End = clean(p.End)
	p.Summary = clean(p.Summary)
	p.Highlights = cleanList(p.Highlights)
	p.Technologies = cleanList(p.Technologies)
}

func normalizeActivity(a *Activity) {
	a.Name = clean(a.Name)
	a.Organization = clean(a.Organization)
	a.Location = clean(a.Location)
	a.Start = clean(a.Start)
	a.End = clean(a.End)
	a.Summary = clean(a.Summary)
	a.Highlights = cleanList(a.Highlights)
}

func normalizeSpokenLanguage(l *SpokenLanguage) {
	l.Name = clean(l.Name)
	l.Level = clean(l.Level)
}

func clean(s string) string { return strings.TrimSpace(s) }

func cleanList(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, v := range in {
		if v = clean(v); v != "" {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func cleanLinks(in []Link) []Link {
	if len(in) == 0 {
		return nil
	}
	out := make([]Link, 0, len(in))
	for _, l := range in {
		l.Label = clean(l.Label)
		l.URL = clean(l.URL)
		if l.URL == "" && l.Label == "" {
			continue
		}
		out = append(out, l)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// cleanSlice normalizes each element and drops the ones that end up empty, so
// that a section with only blank rows is not rendered at all.
func cleanSlice[T any](in []T, fn func(*T)) []T {
	if len(in) == 0 {
		return nil
	}
	out := make([]T, 0, len(in))
	for i := range in {
		fn(&in[i])
		if !isEmpty(in[i]) {
			out = append(out, in[i])
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
