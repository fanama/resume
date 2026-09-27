package txt

import (
	"strings"
	"testing"

	"github.com/fanama/resume/atscv/internal/layout"
	"github.com/fanama/resume/atscv/internal/model"
)

func testLayout() *layout.Layout {
	r := &model.Resume{
		Lang:     model.LangFR,
		Name:     "Jean Dupont",
		Headline: "Développeur Fullstack",
		Contact: model.Contact{
			City: "Paris", Country: "France", Email: "jean@example.com",
			Links: []model.Link{{Label: "GitHub", URL: "https://github.com/fanama"}},
		},
		Summary: "Développeur avec 5 ans d'expérience sur des plateformes de production.",
		Experience: []model.Experience{{
			Title: "Développeur", Company: "SFR", Location: "Paris",
			Start: "2023-01", End: "present",
			Highlights: []string{
				"Plateforme LLM pour 1000 utilisateurs par jour",
				"Couverture de tests portée à 70 %",
			},
			Stack: []string{"React", "NodeJS"},
		}},
		Skills: []model.SkillGroup{{Category: "Langages", Items: []string{"Go", "Python"}}},
	}
	return layout.New(r, layout.Options{Lang: model.LangFR})
}

func TestRender(t *testing.T) {
	out := Render(testLayout())
	if !strings.HasPrefix(out, "Jean Dupont") {
		t.Errorf("the preview must start with the name:\n%s", out)
	}
	for _, want := range []string{
		"PROFIL",
		"EXPÉRIENCE PROFESSIONNELLE",
		"COMPÉTENCES TECHNIQUES",
		"- Plateforme LLM pour 1000 utilisateurs par jour",
		"Technologies: React, NodeJS",
		"Paris, France | jean@example.com",
		"github.com/fanama",
		"01/2023 - Aujourd'hui",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	// The reading order of the preview is the reading order of the documents.
	if strings.Index(out, "PROFIL") > strings.Index(out, "EXPÉRIENCE") {
		t.Errorf("the sections are out of order:\n%s", out)
	}
	if strings.Index(out, "Développeur | SFR") > strings.Index(out, "Technologies") {
		t.Errorf("the fields must follow their entry:\n%s", out)
	}
}

func TestNoDecorativeCharacter(t *testing.T) {
	out := Render(testLayout())
	for _, unwanted := range []string{"•", "→", "✉", "\t", "|==="} {
		if strings.Contains(out, unwanted) {
			t.Errorf("the preview holds %q, an extractor would see noise", unwanted)
		}
	}
}

func TestLinesStayReadable(t *testing.T) {
	out := Render(testLayout())
	for _, line := range strings.Split(out, "\n") {
		if len([]rune(line)) > 100 {
			t.Errorf("a line of %d characters is too long to skim: %q", len([]rune(line)), line)
		}
	}
}

func TestEmptyResumeDoesNotPanic(t *testing.T) {
	out := Render(layout.New(&model.Resume{}, layout.Options{Lang: model.LangFR}))
	// A resume with no name has nothing to print, but it must not invent a
	// section and it must not crash.
	if strings.Contains(out, "EXPÉRIENCE") || strings.Contains(out, "PROFIL") {
		t.Errorf("an empty section must not be printed:\n%s", out)
	}
	if len([]rune(strings.TrimSpace(out))) != 0 {
		t.Errorf("unexpected output for an empty resume: %q", out)
	}
}
