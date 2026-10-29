package propose

import (
	"fmt"
	"strings"

	"github.com/fanama/resume/atscv/internal/model"
)

// The fields this package will propose, and what it says about each one. They
// are the prose fields of the schema, and nothing else: a rule already reads
// every date, every email, every skill list and every language pair.
const (
	taskHeadline = "the professional headline of the candidate: the few words naming the job they do"
	taskSummary  = "the professional summary paragraph of the candidate, written as one paragraph"
	taskMission  = "the one sentence describing what the candidate did in this job, taken from the lines given"
	taskDegree   = "the one sentence describing what the candidate studied, taken from the lines given"
	taskProject  = "the one sentence describing what this project does, taken from the lines given"
	taskActivity = "the one sentence describing what the candidate did there, taken from the lines given"
)

// headLines is how much of the top of the document is offered for the headline
// and the summary. The identity block of a one page CV is four or five lines; a
// longer window is not a richer context, it is a chance to read a section
// heading as prose.
const headLines = 8

// entryLines is the window offered for one entry of a section: the entry's own
// lines, up to the next entry. A job description of more than eight lines is
// rare, and a longer window invites the model to summarise the job after it.
const entryLines = 8

// Missing lists the prose fields a draft left empty and still has something to
// read for, in the order the document holds them.
//
// The second condition is the one that keeps this honest. A summary is empty in
// two very different situations: the CV stated one and no rule could place it,
// or the CV never stated one. Only the first is a question worth asking — for
// the second, the only possible answer is an invented one.
func Missing(r *model.Resume, text string) []Field {
	if r == nil {
		return nil
	}
	docLines := lines(text)
	// The identity block: the top of the document, which is where the name and
	// the headline live. It is a window rather than a parsed block because the
	// only thing needed here is context for the model.
	head := strings.Join(headerBlock(docLines), "\n")

	out := missingHeader(r, head)
	out = append(out, missingExperience(r, docLines)...)
	out = append(out, missingEducation(r, docLines)...)
	out = append(out, missingProjects(r, docLines)...)
	return append(out, missingActivities(r, docLines)...)
}

func missingHeader(r *model.Resume, head string) []Field {
	if head == "" {
		return nil
	}
	var out []Field
	if r.Headline == "" {
		out = append(out, Field{Path: "headline", Ask: taskHeadline, Text: head})
	}
	if r.Summary == "" {
		out = append(out, Field{Path: "summary", Ask: taskSummary, Text: head})
	}
	return out
}

func missingExperience(r *model.Resume, docLines []string) []Field {
	var out []Field
	for i, e := range r.Experience {
		if e.Summary != "" {
			continue
		}
		names := []string{e.Title, e.Company}
		if excerpt := entryExcerpt(docLines, names, experienceAnchors(r)); excerpt != "" {
			out = append(out, Field{
				Path: fmt.Sprintf("experience[%d].summary", i),
				Ask:  taskMission,
				Text: excerpt,
			})
		}
	}
	return out
}

func missingEducation(r *model.Resume, docLines []string) []Field {
	var out []Field
	for i, e := range r.Education {
		if e.Summary != "" {
			continue
		}
		names := []string{e.Degree, e.School}
		if excerpt := entryExcerpt(docLines, names, educationAnchors(r)); excerpt != "" {
			out = append(out, Field{
				Path: fmt.Sprintf("education[%d].summary", i),
				Ask:  taskDegree,
				Text: excerpt,
			})
		}
	}
	return out
}

func missingProjects(r *model.Resume, docLines []string) []Field {
	var out []Field
	for i, p := range r.Projects {
		if p.Summary != "" {
			continue
		}
		if excerpt := entryExcerpt(docLines, []string{p.Name}, projectAnchors(r)); excerpt != "" {
			out = append(out, Field{
				Path: fmt.Sprintf("projects[%d].summary", i),
				Ask:  taskProject,
				Text: excerpt,
			})
		}
	}
	return out
}

func missingActivities(r *model.Resume, docLines []string) []Field {
	var out []Field
	for i, a := range r.Activities {
		if a.Summary != "" {
			continue
		}
		names := []string{a.Name, a.Organization}
		if excerpt := entryExcerpt(docLines, names, activityAnchors(r)); excerpt != "" {
			out = append(out, Field{
				Path: fmt.Sprintf("activities[%d].summary", i),
				Ask:  taskActivity,
				Text: excerpt,
			})
		}
	}
	return out
}

// The anchors of one section: the values the deterministic pass already read for
// its entries. They are what tells one entry's lines from the next one's.
func experienceAnchors(r *model.Resume) []string {
	var out []string
	for _, e := range r.Experience {
		out = append(out, e.Title, e.Company)
	}
	return out
}

func educationAnchors(r *model.Resume) []string {
	var out []string
	for _, e := range r.Education {
		out = append(out, e.Degree, e.School)
	}
	return out
}

func projectAnchors(r *model.Resume) []string {
	var out []string
	for _, p := range r.Projects {
		out = append(out, p.Name)
	}
	return out
}

func activityAnchors(r *model.Resume) []string {
	var out []string
	for _, a := range r.Activities {
		out = append(out, a.Name, a.Organization)
	}
	return out
}
