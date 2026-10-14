package propose

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/fanama/resume/atscv/internal/fill"
	"github.com/fanama/resume/atscv/internal/model"
)

// cvText is a two job CV with a header the extraction left without a headline,
// and with no description line under the second job: the two shapes the tests
// below turn on.
const cvText = `Marie Lefèvre — Développeuse back-end
Lyon, Auvergne-Rhône-Alpes, France · marie.lefevre@exemple.fr · 06 12 34 56 78

Développeuse back-end avec cinq ans d'expériences dans les services financiers.

EXPÉRIENCES
Ingénieure logicielle senior — Fintech Rhône, Lyon (mars 2021 – aujourd'hui)
Conception et exploitation des services de paiement.
- Migration progressive de monolithes Java vers Go, latence divisée par 3.

Développeuse back-end — AssurTech Lyon, Lyon (sept. 2019 – févr. 2022)

COMPÉTENCES
Langages : Go, Python, SQL
`

// draft runs the deterministic pass the way the command line does, so that the
// tests below are about the real draft and not about a fixture someone wrote to
// match the code.
func draft(t *testing.T, text string) *model.Resume {
	t.Helper()
	result, err := fill.Run(text, "")
	if err != nil {
		t.Fatalf("fill: %v", err)
	}
	if result.Resume == nil {
		t.Fatal("fill returned no draft")
	}
	return result.Resume
}

func paths(fields []Field) []string {
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		out = append(out, f.Path)
	}
	return out
}

// TestMissingAsksOnlyAboutTheFieldsTheDraftLeftEmpty is the first half of the
// contract, and the one that keeps the pass from being a way to lose good work:
// a field the rules already filled is never put to the model, whatever the model
// would have answered.
func TestMissingAsksOnlyAboutTheFieldsTheDraftLeftEmpty(t *testing.T) {
	r := draft(t, cvText)
	if len(r.Experience) != 2 {
		t.Fatalf("the fixture produced %d entries, want 2: the test would pass vacuously", len(r.Experience))
	}
	// The profile paragraph was read by the rules — the fixture's is filled,
	// messy separators and all — and both job descriptions were not. Two cases
	// to tell apart, and the looseness of the first is the point: a value the
	// rules read badly is still a value the model must not be asked to replace.
	if r.Summary == "" {
		t.Fatal("the fixture no longer fills the profile summary: the test tells nothing apart")
	}
	asked := paths(Missing(r, cvText))
	if contains(asked, "summary") {
		t.Errorf("the draft holds a summary and the model was asked for one: %v", asked)
	}
	if !contains(asked, "experience[0].summary") {
		t.Errorf("the job whose description the rules could not read was not asked about: %v", asked)
	}

	// And the property itself, over every section: a field is only put to the
	// model when the draft has nothing in it.
	for _, path := range asked {
		if filledInDraft(r, path) {
			t.Errorf("the model was asked about %s, which the draft already holds", path)
		}
	}
}

// filledInDraft answers the one question the tests keep asking: does the draft
// already hold a value for this path? It is written out rather than routed
// through reflection so that a path the package starts asking about, and this
// helper does not know, fails the test instead of passing it silently.
func filledInDraft(r *model.Resume, path string) bool {
	switch path {
	case "headline":
		return r.Headline != ""
	case "summary":
		return r.Summary != ""
	}
	section, field, index, ok := splitPath(path)
	if !ok {
		return true // an unknown path: treated as filled, so the caller fails
	}
	switch section {
	case "experience":
		return index < len(r.Experience) && r.Experience[index].Summary != ""
	case "education":
		return index < len(r.Education) && r.Education[index].Summary != ""
	case "projects":
		return index < len(r.Projects) && r.Projects[index].Summary != ""
	case "activities":
		return index < len(r.Activities) && r.Activities[index].Summary != ""
	}
	_ = field
	return true
}

// TestMissingDoesNotAskAboutAFieldWithNothingToRead is the second half. The
// second job of the fixture has a naming line and nothing under it: there is no
// sentence to find, so there is no question to ask. Asked anyway, a model answers
// something, and that something is an invention.
func TestMissingDoesNotAskAboutAFieldWithNothingToRead(t *testing.T) {
	// The fixture's second job is the last line of its section: a naming line
	// and nothing under it. The rules read nothing there, so the model is not
	// asked either — an answer would have to be invented.
	if asked := paths(Missing(draft(t, cvText), cvText)); contains(asked, "experience[1].summary") {
		t.Errorf("the model was asked about a job with no line to read: %v", asked)
	}

	// The same job with one line under it is a question worth asking, and it is
	// asked with only that line.
	withLine := strings.Replace(cvText,
		"Lyon (sept. 2019 – févr. 2022)\n",
		"Lyon (sept. 2019 – févr. 2022)\nDéveloppement des API internes de souscription.\n", 1)
	r := draft(t, withLine)
	asked := Missing(r, withLine)
	if !contains(paths(asked), "experience[1].summary") {
		t.Fatalf("the job with a line under it was not asked about: %v", paths(asked))
	}
	for _, f := range asked {
		if f.Path != "experience[1].summary" {
			continue
		}
		if !strings.Contains(f.Text, "Développement des API internes") {
			t.Errorf("the excerpt does not hold the job's own line: %q", f.Text)
		}
		if strings.Contains(f.Text, "AssurTech") {
			t.Errorf("the excerpt holds the naming line the model is not asked about: %q", f.Text)
		}
	}
}

// TestTheExcerptsStayInsideTheirOwnBlock pins the two boundaries that decide
// whether an answer can be right at all: the window of an entry stops at the next
// section, and the identity block stops at the first heading. Handed too much, the
// model answers with a line that belongs to somewhere else — measured on a real
// CV, the last job of a section came back as its skills list.
func TestTheExcerptsStayInsideTheirOwnBlock(t *testing.T) {
	r := draft(t, cvText)
	for _, f := range Missing(r, cvText) {
		switch {
		case f.Path == "headline" || f.Path == "summary":
			for _, line := range strings.Split(f.Text, "\n") {
				if fill.IsHeading(line) {
					t.Errorf("%s was handed the heading %q", f.Path, line)
				}
			}
		case strings.HasPrefix(f.Path, "experience["):
			if strings.Contains(f.Text, "Langages") || strings.Contains(f.Text, "COMPÉTENCES") {
				t.Errorf("%s was handed the skills that follow the section: %q", f.Path, f.Text)
			}
			if strings.Contains(f.Text, "AssurTech") && f.Path == "experience[0].summary" {
				t.Errorf("%s was handed the next job: %q", f.Path, f.Text)
			}
		}
	}
}

// TestTheEntryWindowLeavesOutItsNamingLine: the title, the employer and the dates
// are the values the rules already read, and a model offered the line it is not
// being asked about returns it. Measured before this rule existed, both jobs came
// back as their own title line.
func TestTheEntryWindowLeavesOutItsNamingLine(t *testing.T) {
	r := draft(t, cvText)
	found := false
	for _, f := range Missing(r, cvText) {
		if f.Path != "experience[0].summary" {
			continue
		}
		found = true
		for _, unwanted := range []string{"Ingénieure logicielle senior", "Fintech Rhône", "mars 2021"} {
			if strings.Contains(f.Text, unwanted) {
				t.Errorf("the naming line is in the window: %q holds %q", f.Text, unwanted)
			}
		}
	}
	if !found {
		t.Fatal("the fixture no longer leaves the first job's description empty: the test asserts nothing")
	}
}

// splitPath reads a path of the shape "experience[1].summary" back into its
// parts.
func splitPath(path string) (section, field string, index int, ok bool) {
	open := strings.Index(path, "[")
	dot := strings.Index(path, ".")
	if open < 0 || dot < 0 || dot < open {
		return "", "", 0, false
	}
	section = path[:open]
	close := strings.Index(path, "]")
	if close < open {
		return "", "", 0, false
	}
	n, err := strconv.Atoi(path[open+1 : close])
	if err != nil {
		return "", "", 0, false
	}
	return section, path[dot+1:], n, true
}

// recorder is a proposer that answers without an engine, which is what keeps this
// package's tests runnable on a machine that has never downloaded one.
type recorder struct {
	asked []Field
	reply func(Field) (Proposal, error)
}

func (r *recorder) Propose(_ context.Context, f Field) (Proposal, error) {
	r.asked = append(r.asked, f)
	if r.reply != nil {
		return r.reply(f)
	}
	return Proposal{Path: f.Path, Value: "proposed " + f.Path}, nil
}

// TestRunOffersTheDraftAndNeverTouchesIt: the third property of the package. What
// comes back is a report, and a run that changed the resume it was given would
// make the whole "second opinion" framing a lie.
func TestRunOffersTheDraftAndNeverTouchesIt(t *testing.T) {
	r := draft(t, cvText)
	before, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	rec := &recorder{}
	result := Run(context.Background(), rec, r, cvText)

	if len(rec.asked) == 0 {
		t.Fatal("nothing was asked: the fixture no longer leaves a field empty")
	}
	if result.Asked != len(rec.asked) {
		t.Errorf("Asked = %d, %d questions were put to the proposer", result.Asked, len(rec.asked))
	}
	if len(result.Proposals) != len(rec.asked) {
		t.Errorf("%d proposal(s) for %d question(s)", len(result.Proposals), len(rec.asked))
	}
	after, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("the run modified the draft it was handed")
	}
	// And the questions were only about fields the draft left empty.
	empty := map[string]bool{
		"headline": r.Headline == "",
		"summary":  r.Summary == "",
	}
	for _, f := range rec.asked {
		if empty[f.Path] { // a field is either named here or checked below
			continue
		}
		if strings.HasPrefix(f.Path, "experience[") {
			continue
		}
		t.Errorf("the proposer was asked about %s, which is not a field this package asks about", f.Path)
	}
}

// TestRunReportsWhatTheModelCouldNotAnswer: a field the engine returns nothing
// for, or fails on, is reported with its reason. Swallowed, a pass that answered
// nothing is indistinguishable from a document that had nothing to ask.
func TestRunReportsWhatTheModelCouldNotAnswer(t *testing.T) {
	r := draft(t, cvText)
	rec := &recorder{reply: func(f Field) (Proposal, error) {
		if f.Path == "headline" {
			return Proposal{}, errors.New("engine unavailable")
		}
		return Proposal{Path: f.Path, Value: "   "}, nil
	}}
	result := Run(context.Background(), rec, r, cvText)

	if len(result.Proposals) != 0 {
		t.Errorf("an empty answer became a proposal: %+v", result.Proposals)
	}
	if len(result.Failed) != result.Asked {
		t.Fatalf("%d failure(s) reported for %d question(s)", len(result.Failed), result.Asked)
	}
	byPath := map[string]string{}
	for _, f := range result.Failed {
		byPath[f.Path] = f.Reason
	}
	// Every question is accounted for, and the two reasons are told apart: an
	// error from the engine, and an engine that answered with nothing.
	reasons := map[string]bool{}
	for _, f := range rec.asked {
		reason, ok := byPath[f.Path]
		if !ok {
			t.Errorf("%s was asked and never reported", f.Path)
			continue
		}
		reasons[reason] = true
	}
	if !reasons["nothing matched"] {
		t.Errorf("a whitespace answer was not reported as an empty one: %v", reasonsFor(result))
	}
	if !strings.Contains(byPath["headline"], "engine unavailable") {
		t.Errorf("the engine's own error was replaced by %q", byPath["headline"])
	}
}

func reasonsFor(r Result) []string {
	out := make([]string, 0, len(r.Failed))
	for _, f := range r.Failed {
		out = append(out, f.Path+": "+f.Reason)
	}
	return out
}

// TestPropertyOfNamesTheSchemaProperty: the name is the only hint the schema can
// carry about the role of the value, so it has to be the field's own name and not
// the path with its index in it.
func TestPropertyOfNamesTheSchemaProperty(t *testing.T) {
	for path, want := range map[string]string{
		"headline":              "headline",
		"summary":               "summary",
		"experience[1].summary": "summary",
		"activities[0].summary": "summary",
		"":                      "value",
	} {
		if got := propertyOf(path); got != want {
			t.Errorf("propertyOf(%q) = %q, want %q", path, got, want)
		}
	}
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
