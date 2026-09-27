package lint

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/fanama/resume/atscv/internal/layout"
	"github.com/fanama/resume/atscv/internal/model"
)

func run(t *testing.T, r *model.Resume) Report {
	t.Helper()
	return Run(r, layout.New(r, layout.Options{Lang: r.Lang}))
}

func hasRule(rep Report, rule string) bool {
	for _, f := range rep.Findings {
		if f.Rule == rule {
			return true
		}
	}
	return false
}

func countSeverity(rep Report, sev Severity) int {
	n := 0
	for _, f := range rep.Findings {
		if f.Severity == sev {
			n++
		}
	}
	return n
}

func TestEmptyResumeReportsBlockingErrors(t *testing.T) {
	rep := run(t, &model.Resume{})
	if !rep.HasErrors() {
		t.Fatal("an empty resume must report errors")
	}
	for _, rule := range []string{"name", "contact.email", "section.experience"} {
		if !hasRule(rep, rule) {
			t.Errorf("missing the %q error", rule)
		}
	}
	if rep.Stats.Pages != 1 {
		t.Errorf("an empty resume is still one page, got %d", rep.Stats.Pages)
	}
	if rep.Score() >= 60 {
		t.Errorf("an empty resume cannot score %d", rep.Score())
	}
}

func TestInvalidEmailIsAnError(t *testing.T) {
	r := &model.Resume{Name: "Jean", Contact: model.Contact{Email: "jean[at]example.com"}}
	rep := run(t, r)
	if !hasRule(rep, "contact.email") || !rep.HasErrors() {
		t.Errorf("a malformed email must be an error, got %+v", rep.Findings)
	}
}

func TestDatesAreChecked(t *testing.T) {
	r := &model.Resume{
		Name: "Jean",
		Experience: []model.Experience{
			{Title: "A", Company: "X", Start: "2024", End: "2020", Highlights: []string{"Réduit le temps de 30 %"}},
			{Title: "B", Company: "Y", Start: "recently", End: "present", Highlights: []string{"Fait"}},
			{Title: "C", Company: "Z", Start: "2020-05", End: "present", Highlights: []string{"Fait"}},
			{Title: "D", Company: "W", Start: "2016", Highlights: []string{"Fait"}},
		},
	}
	rep := run(t, r)
	if !hasRule(rep, "dates.order") {
		t.Error("a period that ends before it starts must be reported")
	}
	if !hasRule(rep, "dates.parse") {
		t.Error("an unreadable date must be reported")
	}
	if !hasRule(rep, "dates.end") {
		t.Error("a missing end date must be reported")
	}
}

func TestMixedGranularityIsDetected(t *testing.T) {
	r := &model.Resume{
		Name: "Jean",
		Experience: []model.Experience{
			{Title: "A", Start: "2020", End: "2021", Highlights: []string{"Fait"}},
			{Title: "B", Start: "2022-03", End: "present", Highlights: []string{"Fait"}},
		},
	}
	if rep := run(t, r); !hasRule(rep, "dates.granularity") {
		t.Errorf("mixing a year and a month must be reported, got %+v", rep.Findings)
	}

	// A month written with its name is still a month: no false positive.
	consistent := &model.Resume{
		Name: "Jean",
		Experience: []model.Experience{
			{Title: "A", Start: "January 2020", End: "Février 2021", Highlights: []string{"Fait"}},
			{Title: "B", Start: "2022-03", End: "present", Highlights: []string{"Fait"}},
		},
	}
	if rep := run(t, consistent); hasRule(rep, "dates.granularity") {
		t.Errorf("a month name must not be read as a year only, got %+v", rep.Findings)
	}
}

func TestEmojiIsAnErrorAndAGlyphIsAWarning(t *testing.T) {
	r := &model.Resume{
		Name:    "Jean",
		Summary: "Résumé",
		Contact: model.Contact{City: "Paris", Email: "jean@example.com"},
		Experience: []model.Experience{
			{Title: "A", Company: "X", Start: "2020", End: "present", Highlights: []string{"Lancement 🚀 de la plateforme"}},
			{Title: "B", Company: "Y", Start: "2018", End: "2020", Highlights: []string{"Refonte • de l'outil"}},
		},
	}
	rep := run(t, r)
	if !hasRule(rep, "emoji") {
		t.Error("an emoji must be reported")
	}
	if !hasRule(rep, "glyphs") {
		t.Error("a decorative glyph must be reported")
	}
	if countSeverity(rep, Error) != 1 {
		t.Errorf("only the emoji may be an error here, got %d", countSeverity(rep, Error))
	}
}

func TestStatsAndScore(t *testing.T) {
	r := &model.Resume{
		Name:     "Jean Dupont",
		Headline: "Développeur Go",
		Summary:  strings.Repeat("Expérience en développement fullstack avec Go et Python. ", 8),
		Contact:  model.Contact{City: "Paris", Country: "France", Email: "jean@example.com", Phone: "+33 6 12 34 56 78"},
		Experience: []model.Experience{{
			Title: "Développeur", Company: "X", Location: "Paris", Start: "2020-01", End: "present",
			Highlights: []string{"Réduction de 30 % du temps de réponse", "Migration de 12 services"},
			Stack:      []string{"Go", "PostgreSQL"},
		}},
		Skills: []model.SkillGroup{{Category: "Langages", Items: []string{"Go", "Python", "Rust", "TypeScript", "SQL"}}},
	}
	rep := run(t, r)
	if rep.Stats.Entries != 1 {
		t.Errorf("entries = %d, want 1", rep.Stats.Entries)
	}
	if rep.Stats.Bullets != 2 {
		t.Errorf("bullets = %d, want 2", rep.Stats.Bullets)
	}
	if rep.Stats.Metrics != 2 {
		t.Errorf("metrics = %d, want 2", rep.Stats.Metrics)
	}
	if rep.Stats.Skills != 5 {
		t.Errorf("skills = %d, want 5", rep.Stats.Skills)
	}
	if rep.Stats.Words == 0 || rep.Stats.Chars == 0 {
		t.Error("the text statistics must not be empty")
	}
	if rep.Score() > 100 || rep.Score() < 0 {
		t.Errorf("score %d is out of range", rep.Score())
	}
	if rep.Stats.ATSReadability > 100 {
		t.Errorf("readability %d is out of range", rep.Stats.ATSReadability)
	}
}

func TestSortedFindingsPutsErrorsFirst(t *testing.T) {
	rep := run(t, &model.Resume{})
	sorted := rep.SortedFindings()
	for i := 1; i < len(sorted); i++ {
		if sorted[i-1].Severity > sorted[i].Severity {
			t.Fatalf("findings are not sorted by severity at index %d", i)
		}
	}
}

// TestShippedDataHasNoBlockingError guards the two data files the generator
// ships with: they must never introduce an ATS-hostile layout.
func TestShippedDataHasNoBlockingError(t *testing.T) {
	for _, name := range []string{"resume.fr.json", "resume.en.json"} {
		path := filepath.Join("..", "..", "data", name)
		r, err := model.Load(path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		rep := Run(r, layout.New(r, layout.Options{Lang: r.Lang}))
		if rep.HasErrors() {
			for _, f := range rep.SortedFindings() {
				if f.Severity == Error {
					t.Errorf("%s: %s", name, f)
				}
			}
		}
		if rep.Stats.Pages > 2 {
			t.Errorf("%s spans %d pages, an ATS resume should stay at 2", name, rep.Stats.Pages)
		}
	}
}
