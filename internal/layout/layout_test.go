package layout

import (
	"strings"
	"testing"

	"github.com/fanama/resume/atscv/internal/model"
)

func TestParsePeriodAcceptsTheDocumentedForms(t *testing.T) {
	l := New(&model.Resume{Name: "X"}, DefaultOptions())
	cases := []struct {
		start, end              string
		sy, sm, ey, em          int
		present, hasStart, end2 bool
	}{
		{start: "2023", end: "present", sy: 2023, sm: 0, present: true, hasStart: true},
		{start: "2023-03", end: "2023/08", sy: 2023, sm: 3, ey: 2023, em: 8, hasStart: true, end2: true},
		{start: "01/2022", end: "12/2022", sy: 2022, sm: 1, ey: 2022, em: 12, hasStart: true, end2: true},
		{start: "January 2023", end: "Present", sy: 2023, sm: 1, present: true, hasStart: true},
		{start: "mars 2022", end: "aujourd'hui", sy: 2022, sm: 3, present: true, hasStart: true},
		{start: "2020", end: "2022", sy: 2020, ey: 2022, hasStart: true, end2: true},
		{start: "", end: "2022", ey: 2022, end2: true},
		{start: "en cours", end: "en cours", present: true},
	}
	for _, c := range cases {
		p := l.ParsePeriod(c.start, c.end)
		if p.StartYear != c.sy || p.StartMonth != c.sm {
			t.Errorf("ParsePeriod(%q,%q) start = %d-%d, want %d-%d", c.start, c.end, p.StartYear, p.StartMonth, c.sy, c.sm)
		}
		if p.Present != c.present {
			t.Errorf("ParsePeriod(%q,%q) present = %v, want %v", c.start, c.end, p.Present, c.present)
		}
		if p.HasStart != c.hasStart || p.HasEnd != c.end2 {
			t.Errorf("ParsePeriod(%q,%q) flags = %v/%v, want %v/%v", c.start, c.end, p.HasStart, p.HasEnd, c.hasStart, c.end2)
		}
	}
}

func TestPeriodFormatting(t *testing.T) {
	cases := []struct {
		format        DateFormat
		start, end    string
		wantNumeric   string
		wantMonthName string
		wantYear      string
	}{
		{DateNumeric, "2023-01", "present", "01/2023 - Aujourd'hui", "janv. 2023 - Aujourd'hui", "2023 - Aujourd'hui"},
		{DateNumeric, "2019", "2022", "2019 - 2022", "2019 - 2022", "2019 - 2022"},
		{DateNumeric, "2020-03", "2020-08", "03/2020 - 08/2020", "mars 2020 - août 2020", "2020 - 2020"},
	}
	for _, c := range cases {
		l := New(&model.Resume{Name: "X"}, Options{Lang: model.LangFR, DateFormat: c.format})
		got := l.period(c.start, c.end)
		want := c.wantNumeric
		if c.format == DateMonthName {
			want = c.wantMonthName
		}
		if got != want {
			t.Errorf("FR period(%q,%q) = %q, want %q", c.start, c.end, got, want)
		}
	}

	// The same period must be translated in an English layout.
	lEN := New(&model.Resume{Name: "X"}, Options{Lang: model.LangEN, DateFormat: DateNumeric})
	if got := lEN.period("2023-01", "present"); got != "01/2023 - Present" {
		t.Errorf("EN period = %q, want %q", got, "01/2023 - Present")
	}
	lENMonth := New(&model.Resume{Name: "X"}, Options{Lang: model.LangEN, DateFormat: DateMonthName})
	if got := lENMonth.period("2020-03", "2020-08"); got != "Mar 2020 - Aug 2020" {
		t.Errorf("EN month period = %q, want %q", got, "Mar 2020 - Aug 2020")
	}

	// A year-only date must never gain an invented month.
	l := New(&model.Resume{Name: "X"}, Options{Lang: model.LangFR, DateFormat: DateNumeric})
	if got := l.FormatMonth(2023, 0); got != "2023" {
		t.Errorf("FormatMonth(2023, 0) = %q, want %q", got, "2023")
	}
	if got := l.FormatMonth(2023, 3); got != "03/2023" {
		t.Errorf("FormatMonth(2023, 3) = %q, want %q", got, "03/2023")
	}
}

func TestPeriodFallsBackToTheRawInput(t *testing.T) {
	l := New(&model.Resume{Name: "X"}, DefaultOptions())
	if got := l.period("since forever", "and counting"); got != "since forever - and counting" {
		t.Errorf("unparsable period = %q", got)
	}
	if got := l.period("", ""); got != "" {
		t.Errorf("empty period = %q, want empty", got)
	}
	if got := l.period("2020", ""); got != "2020" {
		t.Errorf("open start period = %q, want %q", got, "2020")
	}
}

func TestContactLinesArePlainText(t *testing.T) {
	lines := contactLines(model.Contact{
		City: "Paris", Country: "France",
		Email: "a@b.c", Phone: "+33 6 12 34 56 78",
		Links: []model.Link{
			{Label: "GitHub", URL: "https://github.com/fanama/"},
			{Label: "LinkedIn", URL: "http://www.linkedin.com/in/x"},
			{Label: "Blog", URL: "https://fanama.dev"},
		},
	})
	if len(lines) != 3 {
		t.Fatalf("got %d contact lines, want 3: %q", len(lines), lines)
	}
	if lines[0] != "Paris, France | a@b.c | +33 6 12 34 56 78" {
		t.Errorf("identity line = %q", lines[0])
	}
	if lines[1] != "github.com/fanama | www.linkedin.com/in/x" {
		t.Errorf("first link line = %q", lines[1])
	}
	if lines[2] != "fanama.dev" {
		t.Errorf("second link line = %q", lines[2])
	}
	for _, l := range lines {
		if strings.ContainsAny(l, "•→✉\t") {
			t.Errorf("contact line %q contains a glyph an ATS may mangle", l)
		}
	}
}

func TestLayoutSectionOrderIsFixed(t *testing.T) {
	r := &model.Resume{
		Name:      "Jean Dupont",
		Headline:  "Développeur",
		Summary:   "Résumé.",
		Contact:   model.Contact{City: "Paris", Email: "a@b.c"},
		Languages: []model.SpokenLanguage{{Name: "Français", Level: "Natif"}},
		Skills:    []model.SkillGroup{{Category: "Langages", Items: []string{"Go"}}},
		Projects:  []model.Project{{Name: "P"}},
		Certifications: []model.Certification{
			{Name: "C"},
		},
		Activities: []model.Activity{{Name: "A"}},
		Education:  []model.Education{{Degree: "Master"}},
		Experience: []model.Experience{{Title: "Développeur"}},
	}
	l := New(r, Options{Lang: model.LangFR})
	var sections []string
	var first []string
	for _, b := range l.Blocks {
		if b.Kind == KindSection {
			sections = append(sections, b.Text)
		}
		if len(first) < 2 {
			first = append(first, b.Text)
		}
	}
	want := []string{"PROFIL", "EXPÉRIENCE PROFESSIONNELLE", "FORMATION", "COMPÉTENCES TECHNIQUES", "CERTIFICATIONS", "PROJETS", "ACTIVITÉS", "LANGUES"}
	if strings.Join(sections, ">") != strings.Join(want, ">") {
		t.Errorf("section order = %q, want %q", sections, want)
	}
	if first[0] != "Jean Dupont" || first[1] != "Développeur" {
		t.Errorf("the header must start with the name and the headline, got %q", first)
	}
}

func TestEnglishLabelsAreUsedForAnEnglishResume(t *testing.T) {
	r := &model.Resume{
		Lang:       model.LangEN,
		Name:       "Jean Dupont",
		Summary:    "Summary.",
		Experience: []model.Experience{{Title: "Dev", Start: "2020", End: "present", Stack: []string{"Go"}}},
	}
	l := New(r, Options{})
	joined := blocksText(l)
	for _, want := range []string{"PROFESSIONAL SUMMARY", "PROFESSIONAL EXPERIENCE", "Present", "Technologies: Go"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in the English layout:\n%s", want, joined)
		}
	}
	for _, unwanted := range []string{"PROFIL", "Aujourd'hui"} {
		if strings.Contains(joined, unwanted) {
			t.Errorf("French label %q leaked into the English layout", unwanted)
		}
	}
}

func TestStripDiacritics(t *testing.T) {
	r := &model.Resume{Name: "Fananamperantsoa", Summary: "Développeur à Paris,山路 cenários/test çà"}
	l := New(r, Options{StripDiacritics: true})
	joined := blocksText(l)
	if !strings.Contains(joined, "Developpeur a Paris") {
		t.Errorf("diacritics were not folded: %q", joined)
	}
	if !strings.Contains(joined, "山路") {
		t.Errorf("non-latin characters must be preserved: %q", joined)
	}
}

func TestFold(t *testing.T) {
	cases := map[string]string{
		"Éléonore":           "Eleonore",
		"çà et là":           "ca et la",
		"Cœur & Æther":       "Cour & Ather", // a ligature folds to a single letter
		"Straße":             "Straße",       // the sharp s has no ASCII base, it is kept
		"":                   "",
		"Go / Rust / Python": "Go / Rust / Python",
		"山路 / テスト":           "山路 / テスト",
	}
	for in, want := range cases {
		if got := Fold(in); got != want {
			t.Errorf("Fold(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEmptySectionsAreOmitted(t *testing.T) {
	l := New(&model.Resume{Name: "Jean", Summary: "Résumé."}, Options{Lang: model.LangFR})
	joined := blocksText(l)
	if strings.Contains(joined, "EXPÉRIENCE") || strings.Contains(joined, "FORMATION") {
		t.Errorf("an empty section must not be printed:\n%s", joined)
	}
}

func blocksText(l *Layout) string {
	var b strings.Builder
	for _, blk := range l.Blocks {
		switch blk.Kind {
		case KindEntryTitle:
			b.WriteString(blk.Text)
			if blk.Dim != "" {
				b.WriteString(" | " + blk.Dim)
			}
			b.WriteString("\n")
		case KindField:
			if blk.Bold != "" {
				b.WriteString(blk.Bold + ": " + blk.Text + "\n")
				continue
			}
			b.WriteString(blk.Text + "\n")
		case KindBullet:
			b.WriteString("- " + blk.Text + "\n")
		default:
			b.WriteString(blk.Text + "\n")
		}
	}
	return b.String()
}
