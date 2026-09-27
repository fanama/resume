// Package model holds the resume data structure, its JSON schema and the
// localized labels used to render it.
package model

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Lang is a supported resume language.
type Lang string

const (
	LangFR Lang = "fr"
	LangEN Lang = "en"
)

// ParseLang validates and normalizes a language tag.
func ParseLang(s string) (Lang, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "fr", "fr-fr", "french", "francais":
		return LangFR, nil
	case "en", "en-us", "en-gb", "english", "anglais":
		return LangEN, nil
	case "":
		return "", fmt.Errorf("empty language")
	default:
		return "", fmt.Errorf("unsupported language %q (want fr or en)", s)
	}
}

// Labels returns the section and field labels for a language.
func (l Lang) Labels() Labels {
	if l == LangEN {
		return labelsEN
	}
	return labelsFR
}

// Labels are the translated strings written around the resume content.
type Labels struct {
	Summary         string
	Experience      string
	Education       string
	Skills          string
	Certifications  string
	Projects        string
	Activities      string
	Languages       string
	Present         string
	Technologies    string
	Team            string
	Website         string
	Issued          string
	Honors          string
	CourseworkLabel string
}

// Resume is the whole document. Every section is optional except Name; the
// renderer only emits sections that hold at least one entry, in a fixed order
// that mirrors what Applicant Tracking Systems expect.
type Resume struct {
	Lang           Lang             `json:"lang,omitempty"`
	Name           string           `json:"name"`
	Headline       string           `json:"headline"`
	Contact        Contact          `json:"contact"`
	Summary        string           `json:"summary"`
	Experience     []Experience     `json:"experience"`
	Education      []Education      `json:"education"`
	Skills         []SkillGroup     `json:"skills"`
	Certifications []Certification  `json:"certifications"`
	Projects       []Project        `json:"projects"`
	Activities     []Activity       `json:"activities"`
	Languages      []SpokenLanguage `json:"languages"`
}

// Contact holds the header contact block. Values are printed as plain text in
// reading order: never as icons, QR codes or hyperlinks with hidden labels.
type Contact struct {
	Email   string `json:"email"`
	Phone   string `json:"phone"`
	City    string `json:"city"`
	Region  string `json:"region"`
	Country string `json:"country"`
	Links   []Link `json:"links"`
	// Note is an optional free-form line printed after the links
	// (availability, work authorization, driving licence...).
	Note string `json:"note"`
}

// Link is a plain-text URL with an optional visible label.
type Link struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

// Experience is one job.
type Experience struct {
	Title      string   `json:"title"`
	Company    string   `json:"company"`
	Location   string   `json:"location"`
	Start      string   `json:"start"`
	End        string   `json:"end"`
	Summary    string   `json:"summary"`
	Highlights []string `json:"highlights"`
	Team       string   `json:"team"`
	Stack      []string `json:"stack"`
}

// Education is one degree or school track.
type Education struct {
	Degree     string   `json:"degree"`
	School     string   `json:"school"`
	Location   string   `json:"location"`
	Start      string   `json:"start"`
	End        string   `json:"end"`
	Summary    string   `json:"summary"`
	Coursework []string `json:"coursework"`
}

// SkillGroup is a labelled list of competencies.
type SkillGroup struct {
	Category string   `json:"category"`
	Items    []string `json:"items"`
}

// Certification is a certificate with its issuer.
type Certification struct {
	Name   string `json:"name"`
	Issuer string `json:"issuer"`
	Date   string `json:"date"`
	ID     string `json:"id"`
}

// Project is a side or open-source project.
type Project struct {
	Name         string   `json:"name"`
	URL          string   `json:"url"`
	Start        string   `json:"start"`
	End          string   `json:"end"`
	Summary      string   `json:"summary"`
	Highlights   []string `json:"highlights"`
	Technologies []string `json:"technologies"`
}

// Activity is a non-professional activity worth mentioning (teaching, music,
// volunteering). It still carries dates because ATS filters on recency.
type Activity struct {
	Name         string   `json:"name"`
	Organization string   `json:"organization"`
	Location     string   `json:"location"`
	Start        string   `json:"start"`
	End          string   `json:"end"`
	Summary      string   `json:"summary"`
	Highlights   []string `json:"highlights"`
}

// SpokenLanguage is a language proficiency.
type SpokenLanguage struct {
	Name  string `json:"name"`
	Level string `json:"level"`
}

// Load reads and decodes a resume from a JSON file.
func Load(path string) (*Resume, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	r, err := Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return r, nil
}

// Parse decodes a resume from raw JSON. An unknown field is an error rather
// than a silent no-op: a typo in a key is the most common reason a resume
// renders with a section missing.
func Parse(raw []byte) (*Resume, error) {
	var r Resume
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&r); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	if r.Lang == "" {
		r.Lang = LangFR
	}
	if _, err := ParseLang(string(r.Lang)); err != nil {
		return nil, err
	}
	r.Normalize()
	return &r, nil
}

// Normalize trims every string and drops empty entries so that renderers and
// the linter never have to test for blank fields.
func (r *Resume) Normalize() {
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

	r.Experience = cleanSlice(r.Experience, func(e *Experience) {
		e.Title = clean(e.Title)
		e.Company = clean(e.Company)
		e.Location = clean(e.Location)
		e.Start = clean(e.Start)
		e.End = clean(e.End)
		e.Summary = clean(e.Summary)
		e.Team = clean(e.Team)
		e.Highlights = cleanList(e.Highlights)
		e.Stack = cleanList(e.Stack)
	})

	r.Education = cleanSlice(r.Education, func(e *Education) {
		e.Degree = clean(e.Degree)
		e.School = clean(e.School)
		e.Location = clean(e.Location)
		e.Start = clean(e.Start)
		e.End = clean(e.End)
		e.Summary = clean(e.Summary)
		e.Coursework = cleanList(e.Coursework)
	})

	r.Skills = cleanSlice(r.Skills, func(s *SkillGroup) {
		s.Category = clean(s.Category)
		s.Items = cleanList(s.Items)
	})

	r.Certifications = cleanSlice(r.Certifications, func(c *Certification) {
		c.Name = clean(c.Name)
		c.Issuer = clean(c.Issuer)
		c.Date = clean(c.Date)
		c.ID = clean(c.ID)
	})

	r.Projects = cleanSlice(r.Projects, func(p *Project) {
		p.Name = clean(p.Name)
		p.URL = clean(p.URL)
		p.Start = clean(p.Start)
		p.End = clean(p.End)
		p.Summary = clean(p.Summary)
		p.Highlights = cleanList(p.Highlights)
		p.Technologies = cleanList(p.Technologies)
	})

	r.Activities = cleanSlice(r.Activities, func(a *Activity) {
		a.Name = clean(a.Name)
		a.Organization = clean(a.Organization)
		a.Location = clean(a.Location)
		a.Start = clean(a.Start)
		a.End = clean(a.End)
		a.Summary = clean(a.Summary)
		a.Highlights = cleanList(a.Highlights)
	})

	r.Languages = cleanSlice(r.Languages, func(l *SpokenLanguage) {
		l.Name = clean(l.Name)
		l.Level = clean(l.Level)
	})
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

var labelsFR = Labels{
	Summary:         "PROFIL",
	Experience:      "EXPÉRIENCE PROFESSIONNELLE",
	Education:       "FORMATION",
	Skills:          "COMPÉTENCES TECHNIQUES",
	Certifications:  "CERTIFICATIONS",
	Projects:        "PROJETS",
	Activities:      "ACTIVITÉS",
	Languages:       "LANGUES",
	Present:         "Aujourd'hui",
	Technologies:    "Technologies",
	Team:            "Équipe",
	Website:         "Site",
	Issued:          "Émis le",
	Honors:          "Distinctions",
	CourseworkLabel: "Matières",
}

var labelsEN = Labels{
	Summary:         "PROFESSIONAL SUMMARY",
	Experience:      "PROFESSIONAL EXPERIENCE",
	Education:       "EDUCATION",
	Skills:          "TECHNICAL SKILLS",
	Certifications:  "CERTIFICATIONS",
	Projects:        "PROJECTS",
	Activities:      "ACTIVITIES",
	Languages:       "LANGUAGES",
	Present:         "Present",
	Technologies:    "Technologies",
	Team:            "Team",
	Website:         "Website",
	Issued:          "Issued",
	Honors:          "Honors",
	CourseworkLabel: "Coursework",
}
