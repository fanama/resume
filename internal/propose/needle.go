package propose

import (
	"context"
	"fmt"
	"strings"

	needle "github.com/FlameInTheDark/needle-go"
)

// Engine is the needle-go half of the package: one grammar-constrained tool call
// per field, on the machine, with no network once the engine binary has been
// fetched (14 MB, cached under ~/.cache/needle-go on first use).
//
// The library is imported under the name of the module it declares, because the
// fork this project builds against kept the original module path; go.mod carries
// the replace that points it at github.com/ionut-dejeu-qubiz/needle3-go.
type Engine struct {
	// generation selects the model: 3 is the one this project measured. It is a
	// field rather than a constant so that a test can pin it without a build
	// tag, and so that a future generation is a one line change.
	generation int
}

// NewEngine returns the proposer backed by the local engine.
func NewEngine() *Engine { return &Engine{generation: 3} }

// Propose asks the engine for one field.
//
// Three deliberate choices, each of them a measurement:
//
//   - the schema holds exactly one property, and it is named after the field.
//     A multi-property schema is where the engine loses track of which value
//     goes where — asked for a name and a headline together it returned the same
//     sentence for both, and asked for a language and its level it returned the
//     level as the language;
//   - the question lives in the schema's description and never in the text. An
//     instruction inside the text is text, and the engine has been measured
//     answering "Leave a field empty if absent" as the value of a field;
//   - strict validation is off. Its grounding audit is right to reject a
//     reformatted value, but it also rejects a paragraph the model reflowed, and
//     a suggestion is judged by the reader, not by an audit that cannot see the
//     difference between a paraphrase and a fabrication.
func (e *Engine) Propose(ctx context.Context, f Field) (Proposal, error) {
	name := propertyOf(f.Path)
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			name: map[string]any{"type": "string", "description": f.Ask},
		},
		"required": []string{name},
	}
	args, err := needle.ExtractRaw(ctx, f.Text, schema,
		needle.ExtractGeneration(e.generation),
		needle.ExtractMaxTokens(400),
		needle.ExtractStrict(false),
	)
	if err != nil {
		return Proposal{}, fmt.Errorf("%s: %w", f.Path, err)
	}
	// A nil map is the engine's way of saying nothing in the text matched the
	// schema. That is an answer, not a failure: the field stays empty.
	if args == nil {
		return Proposal{Path: f.Path}, nil
	}
	value, _ := args[name].(string)
	return Proposal{Path: f.Path, Value: strings.TrimSpace(value)}, nil
}

// propertyOf names the schema property after the field, which is what the engine
// reads: "experience[1].summary" asks about a summary, and the word is the only
// hint the schema can carry about the role of the value.
func propertyOf(path string) string {
	name := path
	if i := strings.LastIndex(name, "."); i >= 0 {
		name = name[i+1:]
	}
	if i := strings.Index(name, "["); i >= 0 {
		name = name[:i]
	}
	if name == "" {
		return "value"
	}
	return name
}
