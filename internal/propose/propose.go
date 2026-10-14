// Package propose asks a small local model for the handful of fields that the
// rules of internal/fill cannot read.
//
// The split of work is the point of the package. internal/fill reads what a
// shape can decide: an email is an email, a heading is a heading, a period is a
// period. Those readings are exact, cheap and testable, and needle is not needed
// for any of them. What is left over is prose — the headline sentence, the
// paragraph under a job title, the line a diploma carries — and no rule can
// decide which lines of a page those are, because the page has already lost its
// geometry.
//
// So this package does not extract, it proposes. Three properties hold the
// design together:
//
//   - a field is only put to the model when the deterministic draft left it
//     empty (Missing never asks about a value that is already there);
//   - a field is only put to the model when the document still has lines to read
//     for it, so an absent value stays absent instead of being invented;
//   - nothing here writes to the draft. An answer is a suggestion the reader
//     accepts or ignores.
//
// The third property is not a matter of taste. Measured on a real CV, the engine
// behind Proposer returns about four usable values out of twenty questions: it
// confuses the roles of neighbouring fields, and the same question asked twice
// about the same line can answer once and return nothing the second time. That
// is a useful second opinion and an unusable authority.
package propose

import (
	"context"
	"fmt"
	"strings"

	"github.com/fanama/resume/atscv/internal/fill"
	"github.com/fanama/resume/atscv/internal/layout"
	"github.com/fanama/resume/atscv/internal/model"
)

// Field is one question put to the model: the prose field of the draft that no
// rule could fill, what that field holds, and the lines of the document the
// value has to be read from.
type Field struct {
	// Path is where the value goes, as a path in the draft:
	// "headline", "summary", "experience[1].summary".
	Path string
	// Ask describes the wanted value in one sentence. It is the schema's own
	// description, never part of the text: an instruction left inside the text
	// is text to extract, and the engine has been measured returning it as the
	// value.
	Ask string
	// Text is the excerpt the value must come from — the header block, or one
	// entry of a section. Never the whole document: handed a whole CV the engine
	// reads a field of one entry as a field of the next.
	Text string
}

// Proposal is one answer. A proposal is a suggestion: it is printed for a reader
// to accept, and no code path in this package puts it into a resume.
type Proposal struct {
	Path  string `json:"path"`
	Value string `json:"value"`
}

// Failure is a field the model could not answer, kept rather than swallowed: a
// pass that silently returns nothing looks exactly like a document with nothing
// left to fill.
type Failure struct {
	Path   string
	Reason string
}

// Result is what one pass produced.
type Result struct {
	Proposals []Proposal
	Failed    []Failure
	// Asked is how many fields were put to the model, so that a pass that
	// proposed nothing can be read as "nothing was asked" or "nothing was
	// answered".
	Asked int
}

// Proposer answers one Field at a time. One field per call is a requirement, not
// a style: the engine compiles the schema into a decoding grammar, and a schema
// of several properties is where it loses track of which value goes where.
type Proposer interface {
	Propose(ctx context.Context, f Field) (Proposal, error)
}

// Run puts every missing field to the proposer and collects what came back. A
// field the proposer could not answer, or answered with nothing, is reported
// rather than guessed at.
func Run(ctx context.Context, p Proposer, r *model.Resume, text string) Result {
	var out Result
	for _, f := range Missing(r, text) {
		out.Asked++
		got, err := p.Propose(ctx, f)
		if err != nil {
			out.Failed = append(out.Failed, Failure{Path: f.Path, Reason: err.Error()})
			continue
		}
		value := strings.TrimSpace(got.Value)
		if value == "" {
			out.Failed = append(out.Failed, Failure{Path: f.Path, Reason: "nothing matched"})
			continue
		}
		out.Proposals = append(out.Proposals, Proposal{Path: f.Path, Value: value})
	}
	return out
}

// The fields this package will propose, and what it says about each one. They
// are the prose fields of the schema, and nothing else: a rule already reads
// every date, every email, every skill list and every language pair.
const (
	askHeadline = "the professional headline of the candidate: the few words naming the job they do"
	askSummary  = "the professional summary paragraph of the candidate, written as one paragraph"
	askMission  = "the one sentence describing what the candidate did in this job, taken from the lines given"
	askDegree   = "the one sentence describing what the candidate studied, taken from the lines given"
	askProject  = "the one sentence describing what this project does, taken from the lines given"
	askActivity = "the one sentence describing what the candidate did there, taken from the lines given"
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
	lines := lines(text)
	var out []Field

	// The identity block: the top of the document, which is where the name and
	// the headline live. It is a window rather than a parsed block because the
	// only thing needed here is context for the model.
	head := strings.Join(headerBlock(lines), "\n")
	if r.Headline == "" && head != "" {
		out = append(out, Field{Path: "headline", Ask: askHeadline, Text: head})
	}
	if r.Summary == "" && head != "" {
		out = append(out, Field{Path: "summary", Ask: askSummary, Text: head})
	}

	for i, e := range r.Experience {
		if e.Summary != "" {
			continue
		}
		if excerpt := entryExcerpt(lines, []string{e.Title, e.Company}, experienceAnchors(r)); excerpt != "" {
			out = append(out, Field{Path: fmt.Sprintf("experience[%d].summary", i), Ask: askMission, Text: excerpt})
		}
	}
	for i, e := range r.Education {
		if e.Summary != "" {
			continue
		}
		if excerpt := entryExcerpt(lines, []string{e.Degree, e.School}, educationAnchors(r)); excerpt != "" {
			out = append(out, Field{Path: fmt.Sprintf("education[%d].summary", i), Ask: askDegree, Text: excerpt})
		}
	}
	for i, p := range r.Projects {
		if p.Summary != "" {
			continue
		}
		if excerpt := entryExcerpt(lines, []string{p.Name}, projectAnchors(r)); excerpt != "" {
			out = append(out, Field{Path: fmt.Sprintf("projects[%d].summary", i), Ask: askProject, Text: excerpt})
		}
	}
	for i, a := range r.Activities {
		if a.Summary != "" {
			continue
		}
		if excerpt := entryExcerpt(lines, []string{a.Name, a.Organization}, activityAnchors(r)); excerpt != "" {
			out = append(out, Field{Path: fmt.Sprintf("activities[%d].summary", i), Ask: askActivity, Text: excerpt})
		}
	}
	return out
}

// lines splits the document the way the draft was built from it: one line each,
// trimmed, the empty ones gone. internal/fill does a heavier cleaning, because it
// has to match headings; this only has to find the same lines again.
func lines(text string) []string {
	var out []string
	for _, raw := range strings.Split(text, "\n") {
		line := strings.Join(strings.Fields(raw), " ")
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// headerBlock is the identity block: the lines before the first heading, capped
// so that a document written without a heading at all cannot hand the model a
// whole page. Reading a section heading as the headline is the failure this
// avoids — and the reason the heading test is borrowed from internal/fill rather
// than written again here.
func headerBlock(lines []string) []string {
	var out []string
	for _, line := range lines {
		if fill.IsHeading(line) || len(out) == headLines {
			break
		}
		out = append(out, line)
	}
	return out
}

// entryExcerpt returns the lines of one entry: the line that names it, decided
// by names, and the lines under it up to the next entry of the same section,
// decided by anchors — the values the deterministic pass read for the whole
// section.
//
// The window stops at the next entry rather than at a fixed length, because an
// entry with no description of its own must not be handed the description of the
// one below it. It returns "" when there is nothing but the naming line: a field
// with no lines to read from is a field that has to stay empty, and a model asked
// about nothing answers with something.
func entryExcerpt(lines []string, names, anchors []string) string {
	start := entryStart(lines, names)
	if start < 0 {
		return ""
	}
	end := start + 1
	for end < len(lines) && end-start <= entryLines {
		// The window ends at the next entry — a line the section's anchors
		// already know, or the shape every entry shares, a period on the naming
		// line — and at the next section heading, which the anchors know nothing
		// about: without it the last entry of a section would be handed the
		// skills and the languages that follow it.
		if containsAny(lines[end], anchors) || isEntryAnchor(lines[end]) || fill.IsHeading(lines[end]) {
			break
		}
		end++
	}
	if end == start+1 {
		return ""
	}
	// The naming line is left out on purpose. Its values -- the title, the
	// company, the place, the dates -- are the ones the deterministic pass has
	// already read, and a model offered the line it is not being asked about
	// answers with it: asked for the mission sentence of a job, both entries came
	// back as their own title line, dates included.
	return strings.Join(lines[start+1:end], "\n")
}

// isEntryAnchor reports whether a line starts a new entry rather than continuing
// the one above it. An entry opens with a line that carries a period — a job, a
// diploma, a project all state when they happened — and the shape is the only
// signal available once the page has been flattened.
func isEntryAnchor(line string) bool {
	return strings.Contains(line, " - ") && hasYear(line) ||
		strings.Contains(line, " – ") && hasYear(line) ||
		strings.Contains(line, "|") && hasYear(line)
}

// hasYear is deliberately loose: it decides where a window ends, and a wrong
// guess costs one line of context rather than a wrong value.
func hasYear(line string) bool {
	for _, field := range strings.Fields(line) {
		digits := 0
		for _, r := range field {
			if r >= '0' && r <= '9' {
				digits++
			}
		}
		if digits == 4 {
			return true
		}
	}
	return false
}

// entryStart finds the line that opens an entry.
//
// The first line naming the entry is not enough, and this is not a theory: on a
// CV whose headline is "Développeuse back-end" and whose second job is also a
// "Développeuse back-end", the loose search started at the top of the page and
// handed the model the contact line as the job's mission. An entry line states at
// least two of the things that name it -- the title and the employer, or one of
// them and a period -- so a second pass keeps the loose match only as a fallback.
func entryStart(lines []string, names []string) int {
	for i, line := range lines {
		if containsAny(line, names) && (countAny(line, names) >= 2 || hasYear(line)) {
			return i
		}
	}
	for i, line := range lines {
		if containsAny(line, names) {
			return i
		}
	}
	return -1
}

// countAny counts how many of the anchors the line states.
func countAny(line string, anchors []string) int {
	folded := fold(line)
	seen := 0
	for _, a := range anchors {
		if a != "" && strings.Contains(folded, fold(a)) {
			seen++
		}
	}
	return seen
}

// containsAny reports whether the line states one of the anchors, compared the
// way a heading is compared: an extraction damages accents and case first.
func containsAny(line string, anchors []string) bool {
	folded := fold(line)
	for _, a := range anchors {
		if a == "" {
			continue
		}
		if strings.Contains(folded, fold(a)) {
			return true
		}
	}
	return false
}

func fold(s string) string {
	return layout.Fold(strings.ToLower(strings.TrimSpace(s)))
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
