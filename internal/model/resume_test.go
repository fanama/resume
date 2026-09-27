package model

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseLang(t *testing.T) {
	for _, in := range []string{"fr", "FR", "fr-FR", "french", "francais", " en-US ", "anglais"} {
		if _, err := ParseLang(in); err != nil {
			t.Errorf("ParseLang(%q) = %v, want no error", in, err)
		}
	}
	for _, in := range []string{"", "de", "es", "klingon"} {
		if _, err := ParseLang(in); err == nil {
			t.Errorf("ParseLang(%q) accepted an unsupported language", in)
		}
	}
}

func TestParseRejectsUnknownFields(t *testing.T) {
	// A typo in a key silently drops a whole section, so it must be an error.
	_, err := Parse([]byte(`{"name":"Jean","experiences":[]}`))
	if err == nil {
		t.Fatal("an unknown field must be rejected")
	}
	if !strings.Contains(err.Error(), "experiences") {
		t.Errorf("the error should name the offending field, got %v", err)
	}
}

func TestParseDefaultsToFrench(t *testing.T) {
	r, err := Parse([]byte(`{"name":"Jean"}`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if r.Lang != LangFR {
		t.Errorf("Lang = %q, want %q", r.Lang, LangFR)
	}
	if _, err := Parse([]byte(`{"name":"Jean","lang":"de"}`)); err == nil {
		t.Error("an unsupported language must be rejected")
	}
}

func TestParseRejectsInvalidJSON(t *testing.T) {
	if _, err := Parse([]byte(`{"name":`)); err == nil {
		t.Error("a truncated file must be rejected")
	}
	if _, err := Load(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Error("a missing file must be reported")
	}
}

func TestNormalizeTrimsAndDropsEmptyEntries(t *testing.T) {
	r, err := Parse([]byte(`{
		"name": "  Jean Dupont  ",
		"contact": {"email": " jean@example.com ", "links": [{"label": "GitHub", "url": "  "}, {"label": "", "url": ""}]},
		"skills": [{"category": "Langages", "items": [" Go ", "  ", "Python"]}, {"category": "", "items": []}],
		"experience": [{"title": "  Développeur  ", "company": "", "highlights": ["  "]}]
	}`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if r.Name != "Jean Dupont" {
		t.Errorf("Name = %q, want it trimmed", r.Name)
	}
	if r.Contact.Email != "jean@example.com" {
		t.Errorf("Email = %q, want it trimmed", r.Contact.Email)
	}
	if len(r.Contact.Links) != 1 {
		t.Errorf("links = %d, want the empty one dropped", len(r.Contact.Links))
	}
	if len(r.Skills) != 1 {
		t.Errorf("skills = %d, want the empty group dropped", len(r.Skills))
	}
	if len(r.Skills[0].Items) != 2 {
		t.Errorf("items = %q, want the blank one dropped", r.Skills[0].Items)
	}
	if len(r.Experience) != 1 {
		t.Fatalf("an entry with a title must survive, got %d", len(r.Experience))
	}
	if r.Experience[0].Title != "Développeur" {
		t.Errorf("Title = %q, want it trimmed", r.Experience[0].Title)
	}
	if len(r.Experience[0].Highlights) != 0 {
		t.Errorf("highlights = %q, want the blank line dropped", r.Experience[0].Highlights)
	}
}

func TestLabelsAreTranslated(t *testing.T) {
	fr := LangFR.Labels()
	en := LangEN.Labels()
	if fr.Experience == en.Experience {
		t.Fatal("the experience heading must differ between the two languages")
	}
	if fr.Present != "Aujourd'hui" {
		t.Errorf("FR Present = %q", fr.Present)
	}
	if en.Present != "Present" {
		t.Errorf("EN Present = %q", en.Present)
	}
	if !strings.Contains(fr.Experience, "EXPERIENCE") && !strings.Contains(fr.Experience, "EXPÉRIENCE") {
		t.Errorf("FR experience heading = %q", fr.Experience)
	}
	// An unknown language falls back to French instead of printing nothing.
	if (Lang("de")).Labels().Experience != fr.Experience {
		t.Error("an unknown language must fall back to the French labels")
	}
}

// TestShippedDataFilesLoad keeps data/resume.*.json honest: they must parse,
// declare their language and hold a name and a contact block.
// TestShippedDataFilesKeepEverySection pins the sections of the shipped data.
// A section that silently disappears from data/resume.fr.json turns a two page
// resume into a shorter one without any test noticing, so the counts are
// asserted rather than only the "is it empty" case.
func TestShippedDataFilesKeepEverySection(t *testing.T) {
	for _, name := range []string{"resume.fr.json", "resume.en.json"} {
		path := filepath.Join("..", "..", "data", name)
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		r, err := Parse(raw)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if got := len(r.Experience); got != 3 {
			t.Errorf("%s: %d experiences, want 3", name, got)
		}
		if got := len(r.Education); got != 2 {
			t.Errorf("%s: %d education entries, want 2", name, got)
		}
		if got := len(r.Activities); got != 2 {
			t.Errorf("%s: %d activities, want 2", name, got)
		}
		if got := len(r.Skills); got != 6 {
			t.Errorf("%s: %d skill categories, want 6", name, got)
		}
		skills := 0
		for _, s := range r.Skills {
			if s.Category == "" || len(s.Items) == 0 {
				t.Errorf("%s: a skill category is empty: %+v", name, s)
			}
			skills += len(s.Items)
		}
		if skills != 28 {
			t.Errorf("%s: %d skills, want 28", name, skills)
		}
		if got := len(r.Languages); got != 3 {
			t.Errorf("%s: %d languages, want 3", name, got)
		}
		// The summary is the first thing read and the most expensive in lines:
		// it must stay a short paragraph, not a page of prose.
		if n := len([]rune(r.Summary)); n > 300 {
			t.Errorf("%s: the summary is %d characters, keep it under 300", name, n)
		}
	}
}

func TestShippedDataFilesLoad(t *testing.T) {
	for _, name := range []string{"resume.fr.json", "resume.en.json"} {
		path := filepath.Join("..", "..", "data", name)
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		r, err := Parse(raw)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if r.Name == "" {
			t.Errorf("%s: no name", name)
		}
		if r.Contact.Email == "" {
			t.Errorf("%s: no email", name)
		}
		if len(r.Experience) == 0 {
			t.Errorf("%s: no experience", name)
		}
		if len(r.Skills) == 0 {
			t.Errorf("%s: no skills", name)
		}
		if r.Lang != LangFR && r.Lang != LangEN {
			t.Errorf("%s: unexpected language %q", name, r.Lang)
		}
	}
}

func TestShippedDataFilesHaveNoLeftoverPlaceholder(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "data", "resume.fr.json"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	for _, unwanted := range []string{"TODO", "XXX", "Lorem ipsum", "…", "Lorem"} {
		if strings.Contains(string(raw), unwanted) {
			t.Errorf("the data file still holds the placeholder %q", unwanted)
		}
	}
}
