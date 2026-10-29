package lint

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/fanama/resume/atscv/internal/model"
)

// checkIdentity covers the header block.
func (c *checker) checkIdentity() {
	if c.resume.Name == "" {
		c.add(Error, "name", "header", "the resume has no name",
			"an ATS keys the whole profile on the name, put it in \"name\"")
		return
	}
	name := c.resume.Name
	if len([]rune(name)) > 60 {
		c.add(Info, "name", "header",
			fmt.Sprintf("the name is %d characters long", len([]rune(name))),
			"keep it under 60 characters: surnames with several particles are parsed better alone")
	}
	if low := strings.ToLower(name); strings.Contains(low, "cv") || strings.Contains(low, "curriculum") {
		c.add(Info, "name", "header", "the name contains \"CV\"",
			"the file name already says it, keep the name clean")
	}
	if c.resume.Headline == "" {
		c.add(Warn, "headline", "header", "no job title under the name",
			"one line with the exact title of the offer is the highest-return keyword of the whole file")
	}
	if !utf8.ValidString(name) {
		c.add(Error, "encoding", "header", "the name is not valid UTF-8",
			"export the JSON from an editor that writes UTF-8")
	}
}

// checkContact covers the fields a parser uses to reach the candidate.
func (c *checker) checkContact() {
	ct := c.resume.Contact
	switch {
	case ct.Email == "":
		c.add(Error, "contact.email", "header", "no email address",
			"it is often the only mandatory field of the parsed profile")
	case !emailRe.MatchString(ct.Email):
		c.add(Error, "contact.email", "header",
			fmt.Sprintf("%q is not a valid email address", ct.Email),
			"check for a missing dot in the domain or a trailing space")
	}
	if ct.Phone == "" {
		c.add(Warn, "contact.phone", "header", "no phone number",
			"many filters require a reachable number, write it in international format")
	}
	if ct.City == "" && ct.Country == "" {
		c.add(Warn, "contact.location", "header", "no city",
			"the location drives the eligibility filters of most job boards")
	}
	for i, link := range ct.Links {
		if link.URL == "" {
			c.add(Warn, "contact.links", "header",
				fmt.Sprintf("link %d has a label but no URL", i+1),
				"print the URL as text, never as a hidden hyperlink")
			continue
		}
		if !strings.Contains(link.URL, ".") {
			c.add(Warn, "contact.links", "header",
				fmt.Sprintf("link %q does not look like a URL", link.URL), "")
		}
	}
}

// checkSections covers the presence of the blocks a parser looks for.
func (c *checker) checkSections() {
	r := c.resume
	if strings.TrimSpace(r.Summary) == "" {
		c.add(Warn, "section.summary", "profile", "no profile paragraph",
			"three lines that repeat the offer's keywords are read by both the parser and the recruiter")
	} else {
		n := utf8.RuneCountInString(r.Summary)
		switch {
		case n < 150:
			c.add(Info, "section.summary", "profile",
				fmt.Sprintf("the profile is %d characters long", n),
				"aim for 300 to 800 characters of facts: years of experience, domain, stack")
		case n > 1200:
			c.add(Warn, "section.summary", "profile",
				fmt.Sprintf("the profile is %d characters long", n),
				"long paragraphs are skipped by a recruiter in six seconds")
		}
	}
	if len(r.Experience) == 0 {
		c.add(Error, "section.experience", "experience", "no professional experience",
			"it is the section an ATS weighs the most")
	}
	if len(r.Education) == 0 {
		c.add(Warn, "section.education", "education", "no education",
			"many filters still require a degree, even for a senior profile")
	}
	skills := 0
	for _, g := range r.Skills {
		skills += len(g.Items)
	}
	if skills < 5 {
		c.add(Warn, "section.skills", "skills",
			fmt.Sprintf("only %d skills listed", skills),
			"list at least a dozen: the keyword match is literal, so spell the technologies out")
	}
	if len(r.Languages) == 0 {
		c.add(Info, "section.languages", "languages", "no language",
			"several European portals filter on it")
	}
}

// checkExperience reviews the achievement lines.
func (c *checker) checkExperience() {
	for i, e := range c.resume.Experience {
		where := fmt.Sprintf("experience[%d]", i)
		if e.Title == "" {
			c.add(Error, "entry.title", where, "an entry has no job title",
				"the parser classifies the candidate on the title of each position")
		}
		if e.Company == "" {
			c.add(Warn, "entry.company", where, "the entry has no company name", "")
		}
		if len(e.Highlights) == 0 {
			c.add(Warn, "entry.bullets", where,
				fmt.Sprintf("%s has no achievement line", entryName(e)),
				"a job without a single quantified result is dropped by most screeners")
		}
		if len(e.Highlights) > 7 {
			c.add(Info, "entry.bullets", where,
				fmt.Sprintf("%s has %d achievement lines", entryName(e), len(e.Highlights)),
				"keep the five strongest, the rest belong to the interview")
		}
		quantified := 0
		for _, h := range e.Highlights {
			if metricRe.MatchString(h) {
				quantified++
			}
		}
		if len(e.Highlights) > 0 && quantified == 0 {
			c.add(Info, "entry.metrics", where,
				fmt.Sprintf("no line of %s carries a number", entryName(e)),
				"users, latency, coverage, revenue: one figure per line is what gets read")
		}
		if len(e.Stack) == 0 {
			c.add(Info, "entry.stack", where,
				fmt.Sprintf("%s lists no technology", entryName(e)), "")
		}
	}
	c.checkEducation()
}

// checkEducation reviews the education entries.
func (c *checker) checkEducation() {
	for i, e := range c.resume.Education {
		where := fmt.Sprintf("education[%d]", i)
		if e.Degree == "" && e.School == "" {
			c.add(Error, "entry.degree", where, "an education entry is empty", "")
		}
		if e.School == "" {
			c.add(Info, "entry.school", where, "the education entry has no school name", "")
		}
	}
}

func entryName(e model.Experience) string {
	switch {
	case e.Company != "":
		return e.Company
	case e.Title != "":
		return e.Title
	default:
		return "an entry"
	}
}
