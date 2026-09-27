package adoc

import (
	"strings"
	"testing"

	"github.com/fanama/resume/atscv/internal/layout"
	"github.com/fanama/resume/atscv/internal/model"
)

func testLayout(lang model.Lang) *layout.Layout {
	r := &model.Resume{
		Lang:     lang,
		Name:     "Jean Dupont",
		Headline: "Développeur Fullstack",
		Contact: model.Contact{
			City: "Paris", Country: "France", Email: "jean@example.com",
			Links: []model.Link{{Label: "GitHub", URL: "https://github.com/fanama"}},
		},
		Summary: "Développeur avec 5 ans d'expérience.",
		Experience: []model.Experience{{
			Title: "Développeur", Company: "SFR", Location: "Paris",
			Start: "2023-01", End: "present",
			Highlights: []string{"Plateforme LLM pour 1000 utilisateurs par jour"},
			Stack:      []string{"React", "NodeJS"},
		}},
	}
	return layout.New(r, layout.Options{Lang: lang})
}

func TestRender(t *testing.T) {
	out := Render(testLayout(model.LangFR), Options{Lang: model.LangFR})
	if !strings.HasPrefix(out, "// Generated") && !strings.HasPrefix(out, "= Jean Dupont") {
		t.Errorf("the file must start with the title, got:\n%s", firstLines(out, 3))
	}
	if !strings.HasPrefix(out, "= Jean Dupont") {
		t.Errorf("the document title is the name, got:\n%s", firstLines(out, 2))
	}
	for _, want := range []string{
		":lang: fr",
		":pdf-page-size: A4",
		"[.text-center]",
		"== PROFIL",
		"== EXPÉRIENCE PROFESSIONNELLE",
		"*Développeur*",
		"* Plateforme LLM pour 1000 utilisateurs par jour",
		"*Technologies*: React, NodeJS",
		"github.com/fanama",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	// The name must not be repeated in the body: it is the document title.
	if strings.Count(out, "Jean Dupont") != 1 {
		t.Errorf("the name appears %d times, it should appear once as the title:\n%s",
			strings.Count(out, "Jean Dupont"), out)
	}
}

// TestNoTableNoColumns is the AsciiDoc side of the ATS contract: a table or a
// column turns the resume into an unreadable sequence for a parser.
func TestNoTableNoColumns(t *testing.T) {
	out := Render(testLayout(model.LangFR), Options{Lang: model.LangFR})
	for _, unwanted := range []string{"|===", "cols=", "[cols=", "[options=", "sidebar", "image::", "include::"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("the AsciiDoc holds %q, which breaks the reading order", unwanted)
		}
	}
	// Achievement lines are a list item, not a literal dash.
	if strings.Contains(out, "\n- ") {
		t.Errorf("a typed dash was found, use a list item:\n%s", out)
	}
}

func TestNoNumberedSectionsOrTOC(t *testing.T) {
	out := Render(testLayout(model.LangFR), Options{Lang: model.LangFR})
	for _, unwanted := range []string{":sectnums:", ":toc:"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("a resume must not carry %q", unwanted)
		}
	}
}

func TestEnglishLayout(t *testing.T) {
	out := Render(testLayout(model.LangEN), Options{Lang: model.LangEN})
	if !strings.Contains(out, ":lang: en") {
		t.Error("the lang attribute is missing")
	}
	if !strings.Contains(out, "== PROFESSIONAL EXPERIENCE") {
		t.Errorf("the English heading is missing:\n%s", out)
	}
	if strings.Contains(out, "== EXPÉRIENCE") {
		t.Error("a French heading leaked into the English document")
	}
}

func TestSectionRules(t *testing.T) {
	plain := Render(testLayout(model.LangFR), Options{Lang: model.LangFR})
	ruled := Render(testLayout(model.LangFR), Options{Lang: model.LangFR, SectionRules: true})
	if strings.Contains(plain, "'''") {
		t.Error("a rule was drawn without the option")
	}
	if n := strings.Count(ruled, "'''"); n < 2 {
		t.Errorf("%d rules drawn, want one per section", n)
	}
}

func TestHeaderComment(t *testing.T) {
	out := Render(testLayout(model.LangFR), Options{Lang: model.LangFR, Header: "line one\nline two"})
	if !strings.HasPrefix(out, "// line one\n// line two\n") {
		t.Errorf("the header comment is wrong:\n%s", firstLines(out, 3))
	}
}

func TestEmptyLayoutDoesNotPanic(t *testing.T) {
	l := layout.New(&model.Resume{}, layout.Options{Lang: model.LangFR})
	out := Render(l, Options{})
	if !strings.Contains(out, "= Resume") {
		t.Errorf("an empty resume still needs a title:\n%s", out)
	}
}

func firstLines(s string, n int) string {
	lines := strings.SplitN(s, "\n", n+1)
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}
