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
	"strings"

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
