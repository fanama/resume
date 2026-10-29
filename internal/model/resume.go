// Package model holds the resume data structure, its JSON schema and the
// localized labels used to render it.
package model

import (
	"fmt"
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
