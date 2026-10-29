package model

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
