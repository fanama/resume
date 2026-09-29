package fill

import (
	"encoding/json"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/fanama/resume/atscv/internal/layout"
	"github.com/fanama/resume/atscv/internal/model"
	"github.com/fanama/resume/atscv/internal/render/txt"
)

// frenchResume is the text an extraction hands over for a two page French CV: a
// header with the contact block, a profile, an experience section with two
// posts, a formation, a skills block and languages.
const frenchResume = `Camille Fontaine
Ingénieure logiciel
camille.fontaine@example.fr | +33 6 12 34 56 78 | Lyon, France
linkedin.com/in/camillefontaine | github.com/camillefontaine

Profil
Ingénieure logiciel avec huit ans d'expérience sur des systèmes distribués. Je
m'intéresse à la fiabilité et à la lisibilité du code.

EXPÉRIENCE PROFESSIONNELLE

Ingénieure logiciel senior | Nexteo | 03/2021 - aujourd'hui
Migration du monolithe vers des services Go, -40% de latence.
Animation de la revue de code, 12 personnes.
Mise en place de l'observabilité avec OpenTelemetry.

Développeuse backend | Atelier Kraft | 09/2018 - 02/2021
Développement d'une API REST en Go et PostgreSQL.
Refonte du schéma de base de données, -60% de temps de migration.

FORMATION
Master Informatique,分布式 systèmes | Université Lyon 1 | 2016 - 2018
Licence Informatique | IUT de Lyon | 2013 - 2016

COMPÉTENCES
Langages: Go, Python, TypeScript, SQL
Outils: Docker, Kubernetes, PostgreSQL, Git

LANGUES
Français : Langue maternelle
Anglais : C1
`

func dump(t *testing.T, r *model.Resume) string {
	t.Helper()
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestRunReadsAWholeFrenchCV(t *testing.T) {
	res, err := Run(frenchResume, "")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	r := res.Resume

	if r.Name != "Camille Fontaine" {
		t.Errorf("Name = %q, want Camille Fontaine", r.Name)
	}
	if r.Headline != "Ingénieure logiciel" {
		t.Errorf("Headline = %q, want Ingénieure logiciel", r.Headline)
	}
	if r.Contact.Email != "camille.fontaine@example.fr" {
		t.Errorf("Email = %q", r.Contact.Email)
	}
	if r.Contact.Phone != "+33612345678" {
		t.Errorf("Phone = %q, want +33612345678", r.Contact.Phone)
	}
	if r.Contact.City != "Lyon" || r.Contact.Country != "France" {
		t.Errorf("City/Country = %q/%q, want Lyon/France", r.Contact.City, r.Contact.Country)
	}
	if len(r.Contact.Links) != 2 {
		t.Errorf("links = %d, want 2: %+v", len(r.Contact.Links), r.Contact.Links)
	}
	if r.Summary == "" {
		t.Error("Summary is empty: the profile paragraph was dropped")
	}

	if len(r.Experience) != 2 {
		t.Fatalf("experience = %d, want 2\n%s", len(r.Experience), dump(t, r))
	}
	first := r.Experience[0]
	if first.Title == "" || first.Company != "Nexteo" {
		t.Errorf("first job = %q at %q, want a title at Nexteo", first.Title, first.Company)
	}
	if first.Start != "03/2021" || first.End != "present" {
		t.Errorf("first job period = %q - %q, want 03/2021 - present", first.Start, first.End)
	}
	if len(first.Highlights) != 3 {
		t.Errorf("first job highlights = %d, want 3: %+v", len(first.Highlights), first.Highlights)
	}
	if !strings.HasPrefix(first.Highlights[0], "Migration du monolithe") {
		t.Errorf("first highlight = %q, want the line under the title", first.Highlights[0])
	}

	// Every period has to survive: layout is what will format and lint them.
	l := layout.New(&model.Resume{}, layout.Options{Lang: res.Lang})
	for i, e := range r.Experience {
		if p := l.ParsePeriod(e.Start, e.End); !p.HasStart {
			t.Errorf("job %d: %q - %q is not a period layout can read", i, e.Start, e.End)
		}
	}

	if len(r.Education) != 2 {
		t.Errorf("education = %d, want 2\n%s", len(r.Education), dump(t, r))
	}
	if len(r.Skills) == 0 {
		t.Errorf("no skill group read\n%s", dump(t, r))
	}
	if len(r.Languages) != 2 {
		t.Errorf("languages = %d, want 2: %+v", len(r.Languages), r.Languages)
	}
	if len(res.Unmapped) != 0 {
		t.Errorf("Unmapped = %v, want none: every section landed somewhere", res.Unmapped)
	}
}

// The English CV: different headings, a different date spelling, a different
// separator between the title and the company. A French-only reader is the
// most likely bug here, so the test carries the whole document.
func TestRunReadsAnEnglishCV(t *testing.T) {
	const text = `Jordan Blake
Senior Data Engineer
jordan.blake@example.com · +1 (415) 555-0142 · San Francisco, United States

PROFILE
Data engineer with six years building batch and streaming pipelines.

WORK EXPERIENCE

Senior Data Engineer | Vantage Labs | Jan 2021 - Present
Rebuilt the ingestion pipeline in Airflow, 3x throughput.
Introduced schema registry and contract testing.

Data Engineer | Northwind | Jun 2018 - Dec 2020
Owned the warehouse migration from Redshift to BigQuery.

EDUCATION
BSc Computer Science | University of Bristol | 2014 - 2017

TECHNICAL SKILLS
Languages: Python, SQL, Go
Infrastructure: Terraform, Airflow, BigQuery

LANGUAGES
English : Native
French : B2
`
	res, err := Run(text, "")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	r := res.Resume
	if res.Lang != model.LangEN {
		t.Errorf("Lang = %q, want en: the headings are English", res.Lang)
	}
	if r.Name != "Jordan Blake" {
		t.Errorf("Name = %q", r.Name)
	}
	if r.Contact.Phone != "+14155550142" {
		t.Errorf("Phone = %q, want +14155550142", r.Contact.Phone)
	}
	if r.Contact.City != "San Francisco" || r.Contact.Country != "United States" {
		t.Errorf("City/Country = %q/%q", r.Contact.City, r.Contact.Country)
	}
	if len(r.Experience) != 2 {
		t.Fatalf("experience = %d, want 2\n%s", len(r.Experience), dump(t, r))
	}
	// "Jan 2021" is kept as printed: layout reads a month name, and lint asks
	// for a month rather than a bare year.
	if got := r.Experience[0]; got.Company != "Vantage Labs" || got.Start != "Jan 2021" || got.End != "present" {
		t.Errorf("first job = %+v, want Vantage Labs, Jan 2021 - present", got)
	}
	if len(r.Education) != 1 {
		t.Errorf("education = %d, want 1\n%s", len(r.Education), dump(t, r))
	}
	if len(r.Languages) != 2 {
		t.Errorf("languages = %d, want 2: %+v", len(r.Languages), r.Languages)
	}
}

// A PDF with no text layer is the failure this feature has to explain rather
// than hide. Returning an empty draft would look like a successful import.
func TestRunRefusesTextItCannotRead(t *testing.T) {
	for name, text := range map[string]string{
		"empty":       "",
		"blank lines": "\n\n   \n\t\n",
		"a scan":      "������ ��� ��",
	} {
		if _, err := Run(text, ""); !errors.Is(err, ErrNoText) {
			t.Errorf("%s: err = %v, want ErrNoText", name, err)
		}
	}
}

// The heading table is written by hand in both languages, so a key that is not
// already folded is dead weight: foldKey can never produce it. Accents and case
// are what a PDF loses, so this is a real failure and not a style point.
func TestEveryHeadingKeyIsFolded(t *testing.T) {
	for key := range headings {
		if got := foldKey(key); got != key {
			t.Errorf("heading %q is not in folded form: it would only be reached as %q", key, got)
		}
	}
}

// A heading with the accents and the case a PDF leaves behind has to land on
// the same section as the clean spelling.
func TestHeadingsSurviveTheWayAPDFManglesThem(t *testing.T) {
	for _, h := range []string{
		"EXPERIENCE", "Expérience", "EXPÉRIENCE", "  experience  ", "2. Experience",
		"EXPERIENCE PROFESSIONNELLE", "Compétences", "COMPETENCES", "FORMATION",
		"Études", "Langues", "PROJETS", "CERTIFICATIONS", "Profil", "BÉNÉVOLAT",
	} {
		if _, ok := sectionOf(h); !ok {
			t.Errorf("%q is not recognised as a heading", h)
		}
	}
	// And a line of body text that starts with the word is not a heading.
	for _, notHeading := range []string{
		"Experience utilisateur de la plateforme",
		"Le language Go est un langage compilé",
		"",
	} {
		if _, ok := sectionOf(notHeading); ok {
			t.Errorf("%q was read as a heading", notHeading)
		}
	}
}

// A description that wraps is one description, not a list of fragments. The
// page breaks it wherever the width runs out, so reading each of its lines as
// a highlight filled the form with clauses that stop mid-sentence and lost the
// paragraph the candidate actually wrote.
func TestAWrappedDescriptionIsPutBackTogether(t *testing.T) {
	text := `Jean Dupont
jean@exemple.fr
EXPÉRIENCE PROFESSIONNELLE
Ingénieur Logiciel | Acme | 2019 - 2023
Au sein d'une équipe de huit personnes, j'ai pris en charge la refonte complète de la
plateforme de paiement, depuis la conception jusqu'à la mise en production.
- Livré une API REST
`
	res, err := Run(text, model.LangFR)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Resume.Experience) != 1 {
		t.Fatalf("experience = %d, want 1", len(res.Resume.Experience))
	}
	exp := res.Resume.Experience[0]
	const want = "Au sein d'une équipe de huit personnes, j'ai pris en charge la refonte complète de la " +
		"plateforme de paiement, depuis la conception jusqu'à la mise en production."
	if exp.Summary != want {
		t.Errorf("summary =\n %q\nwant\n %q", exp.Summary, want)
	}
	if len(exp.Highlights) != 1 || exp.Highlights[0] != "Livré une API REST" {
		t.Errorf("highlights = %q, want the one bullet", exp.Highlights)
	}
}

// The counter-example to the test above, and the reason the description cannot
// simply be "everything before the first bullet": plenty of templates print
// three achievements as three unmarked lines and no description at all. Each
// is a complete line, so none of them is a paragraph.
func TestUnmarkedAchievementsStayThreeHighlights(t *testing.T) {
	res, err := Run(frenchResume, model.LangFR)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Resume.Experience) == 0 {
		t.Fatal("no experience read")
	}
	if got := len(res.Resume.Experience[0].Highlights); got != 3 {
		t.Errorf("highlights = %d, want 3: %q", got, res.Resume.Experience[0].Highlights)
	}
}

// A marked bullet wraps too, and its second line carries no mark. Both halves
// are one highlight; read apart, the list fills with orphaned tails.
func TestAWrappedBulletStaysOneHighlight(t *testing.T) {
	text := `Jean Dupont
jean@exemple.fr
EXPÉRIENCE PROFESSIONNELLE
Ingénieur Logiciel | Acme | 2019 - 2023
- Rédaction du document Crédit Impôt Recherche (CIR), valorisant les projets IA de
l'entreprise auprès de l'administration fiscale.
- Livré une API REST
`
	res, err := Run(text, model.LangFR)
	if err != nil {
		t.Fatal(err)
	}
	hl := res.Resume.Experience[0].Highlights
	if len(hl) != 2 {
		t.Fatalf("highlights = %d, want 2: %q", len(hl), hl)
	}
	const want = "Rédaction du document Crédit Impôt Recherche (CIR), valorisant les projets IA de " +
		"l'entreprise auprès de l'administration fiscale."
	if hl[0] != want {
		t.Errorf("first highlight =\n %q\nwant\n %q", hl[0], want)
	}
}

// What a school states under a degree is the coursework. It used to be dropped
// on the floor: readEducation read the notes and never looked at the bullets,
// so a degree with its modules listed imported as a bare line.
func TestADegreeKeepsItsCoursework(t *testing.T) {
	text := `Jean Dupont
jean@exemple.fr
FORMATION
Master Informatique | Université de Paris | 2017 - 2019
- Systèmes distribués
- Sécurité des réseaux
`
	res, err := Run(text, model.LangFR)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Resume.Education) != 1 {
		t.Fatalf("education = %d, want 1", len(res.Resume.Education))
	}
	if got := res.Resume.Education[0].Coursework; len(got) != 2 {
		t.Errorf("coursework = %q, want the two modules", got)
	}
}

// The whole point of the importer is that the app can read its own output. A
// resume rendered to text and read back must come back with the same
// descriptions and the same number of highlights, or the round trip silently
// edits the candidate's CV.
func TestRenderedOutputReadsBackUnchanged(t *testing.T) {
	b, err := os.ReadFile("../../data/resume.fr.json")
	if err != nil {
		t.Skipf("no sample resume: %v", err)
	}
	var want model.Resume
	if err := json.Unmarshal(b, &want); err != nil {
		t.Fatal(err)
	}
	res, err := Run(txt.Render(layout.New(&want, layout.Options{Lang: want.Lang})), want.Lang)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Resume.Experience) != len(want.Experience) {
		t.Fatalf("experience = %d, want %d", len(res.Resume.Experience), len(want.Experience))
	}
	for i, w := range want.Experience {
		g := res.Resume.Experience[i]
		if g.Summary != w.Summary {
			t.Errorf("experience %d summary =\n %q\nwant\n %q", i, g.Summary, w.Summary)
		}
		if len(g.Highlights) != len(w.Highlights) {
			t.Errorf("experience %d highlights = %d, want %d: %q",
				i, len(g.Highlights), len(w.Highlights), g.Highlights)
		}
		if g.Team != w.Team {
			t.Errorf("experience %d team = %q, want %q", i, g.Team, w.Team)
		}
	}
	if len(res.Resume.Education) != len(want.Education) {
		t.Fatalf("education = %d, want %d", len(res.Resume.Education), len(want.Education))
	}
	for i, w := range want.Education {
		g := res.Resume.Education[i]
		if g.Degree != w.Degree {
			t.Errorf("education %d degree = %q, want %q", i, g.Degree, w.Degree)
		}
		if g.School != w.School {
			t.Errorf("education %d school = %q, want %q", i, g.School, w.School)
		}
	}
}

// "Équipe:" and "Technologies:" are fields this tool prints itself. Read back
// as highlights, they appended two lines of metadata to the achievements of
// every job and left the fields they belong to empty.
func TestTeamAndStackGoBackToTheirFields(t *testing.T) {
	text := `Jean Dupont
jean@exemple.fr
EXPÉRIENCE PROFESSIONNELLE
Ingénieur Logiciel | Acme | 2019 - 2023
- Livré une API REST
Équipe: Chef de projet et deux développeurs
Technologies: Go, React, PostgreSQL
`
	res, err := Run(text, model.LangFR)
	if err != nil {
		t.Fatal(err)
	}
	exp := res.Resume.Experience[0]
	if exp.Team != "Chef de projet et deux développeurs" {
		t.Errorf("team = %q", exp.Team)
	}
	if len(exp.Stack) != 3 {
		t.Errorf("stack = %q, want three items", exp.Stack)
	}
	if len(exp.Highlights) != 1 {
		t.Errorf("highlights = %q, want only the bullet", exp.Highlights)
	}
}

// A skills block usually names no category: it is a handful of comma separated
// lines, and a PDF with two columns wraps them into many more. Reading each
// line as its own group produced a form with a dozen cards all headed
// "Compétences", which is not a classification but the absence of one. They
// belong in one group.
func TestUncategorisedSkillLinesBecomeOneGroup(t *testing.T) {
	text := `Jean Dupont
jean@exemple.fr
COMPÉTENCES
Python, Java, JavaScript
Sécurité des API (OWASP Top 10)
Linux, Windows Admin
Architecture IAM, Protocoles
EXPÉRIENCE
Dev | Acme | 2019 - 2023
- a livré une API
`
	r, err := Run(text, model.LangFR)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Resume.Skills) != 1 {
		t.Fatalf("got %d skill groups, want 1: %+v", len(r.Resume.Skills), r.Resume.Skills)
	}
	got := r.Resume.Skills[0]
	if got.Category != "Compétences" {
		t.Errorf("category = %q, want %q", got.Category, "Compétences")
	}
	// Nothing may be dropped by the merge.
	for _, want := range []string{
		"Python", "Java", "JavaScript", "Sécurité des API (OWASP Top 10)",
		"Linux", "Windows Admin", "Architecture IAM", "Protocoles",
	} {
		if !slices.Contains(got.Items, want) {
			t.Errorf("item %q was lost, have %v", want, got.Items)
		}
	}
}

// Merging the uncategorised lines must not merge the categorised ones: a block
// that does classify its skills keeps that classification, and a category the
// extraction split across two lines is joined rather than repeated.
func TestNamedSkillCategoriesSurviveTheMerge(t *testing.T) {
	text := `Jean Dupont
jean@exemple.fr
COMPÉTENCES
Langages : Python, Java
Cloud : AWS, Azure
Python, Go
Langages : Rust, C
Bash, Git
EXPÉRIENCE
Dev | Acme | 2019 - 2023
- a livré une API
`
	r, err := Run(text, model.LangFR)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Resume.Skills) != 3 {
		t.Fatalf("got %d groups, want 3 (Langages, Cloud, the bare list): %+v",
			len(r.Resume.Skills), r.Resume.Skills)
	}
	byName := map[string][]string{}
	for _, g := range r.Resume.Skills {
		byName[g.Category] = g.Items
	}
	// "Langages" is stated twice and has to be one group holding both lines.
	for _, want := range []string{"Python", "Java", "Rust", "C"} {
		if !slices.Contains(byName["Langages"], want) {
			t.Errorf("Langages lost %q, have %v", want, byName["Langages"])
		}
	}
	if n := len(byName["Cloud"]); n != 2 {
		t.Errorf("Cloud has %d items, want 2: %v", n, byName["Cloud"])
	}
	// The bare lines are pooled under the default label, and a skill already
	// filed under a real category is not duplicated into it.
	for _, want := range []string{"Go", "Bash", "Git"} {
		if !slices.Contains(byName["Compétences"], want) {
			t.Errorf("the bare list lost %q, have %v", want, byName["Compétences"])
		}
	}
}

// The order of the section is the order the candidate chose. The bare list is
// placed where its first line appeared, not pushed to the end.
func TestTheSkillGroupsKeepTheOrderOfTheSection(t *testing.T) {
	text := `Jean Dupont
jean@exemple.fr
COMPÉTENCES
Python, Go
Cloud : AWS, Azure
EXPÉRIENCE
Dev | Acme | 2019 - 2023
- a livré une API
`
	r, err := Run(text, model.LangFR)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Resume.Skills) != 2 {
		t.Fatalf("got %d groups, want 2: %+v", len(r.Resume.Skills), r.Resume.Skills)
	}
	if r.Resume.Skills[0].Category != "Compétences" {
		t.Errorf("the bare list came first in the PDF, so it comes first here: got %q",
			r.Resume.Skills[0].Category)
	}
	if r.Resume.Skills[1].Category != "Cloud" {
		t.Errorf("second group = %q, want %q", r.Resume.Skills[1].Category, "Cloud")
	}
}

// The profile section is the one a candidate names in their own words: it is
// "Profil" in one template, "À propos de moi" in the next and "Description" in
// a third. Enumerating every wording is a losing game, so it is matched on its
// shape, and this test is what says which shapes.
func TestTheProfileSectionIsRecognisedHoweverItIsNamed(t *testing.T) {
	for _, h := range []string{
		// French
		"PROFIL", "Profil", "Profil professionnel", "Mon profil", "À PROPOS",
		"À propos de moi", "A propos de mon parcours", "Présentation", "Accroche",
		"En bref", "Bref aperçu", "Aperçu", "Qui suis-je", "Description",
		"Description personnelle", "Objectif professionnel", "Objectif de carrière",
		"Biographie", "1. Profil", "II. À propos",
		// English
		"Profile", "Personal Profile", "Professional Profile", "Profile Summary",
		"About", "About Me", "About myself", "SUMMARY", "Professional Summary",
		"Career Summary", "Executive Summary", "Summary of Qualifications",
		"Overview", "Bio", "Career Objective", "Personal Statement",
	} {
		kind, ok := sectionOf(h)
		if !ok {
			t.Errorf("%q is not recognised as a heading at all", h)
			continue
		}
		if kind != kindSummary {
			t.Errorf("%q was read as %q, want %q", h, kind, kindSummary)
		}
	}
}

// The risk of matching the profile on its shape is the opposite of a miss. A
// resume quotes the wording of job ads, and "Description du poste" or "Profil
// recherché" describe the role, not the candidate. Reading one as the summary
// would put the wrong paragraph at the top of the document, so each of these
// has to stay out of the summary.
func TestJobAdWordingIsNotTheCandidatesProfile(t *testing.T) {
	for _, h := range []string{
		"Profil recherché", "Description du poste", "Description des missions",
		"Description de l'offre", "Description technique", "Description du projet",
		"Profil de l'entreprise", "Profils LinkedIn", "Objectifs du poste",
		"Job Description", "About the company", "About us",
		"A propos de l'entreprise", "Summary of experience", "Experience summary",
	} {
		if kind, ok := sectionOf(h); ok && kind == kindSummary {
			t.Errorf("%q was read as the candidate's profile", h)
		}
	}
}

// A heading listed in both language tables counts for nothing, so a document
// whose only heading is "EXPERIENCE" is decided by something else, or not at
// all. The overlap is silent, which is why it is checked rather than reviewed.
func TestTheLanguageTablesDoNotOverlap(t *testing.T) {
	for key := range frenchOnly {
		if _, both := englishOnly[key]; both {
			t.Errorf("heading %q counts for both languages, so it decides nothing", key)
		}
	}
	// And the keys have to be real headings, or they count for a spelling the
	// table never holds.
	for key := range frenchOnly {
		if _, ok := headings[key]; !ok {
			t.Errorf("frenchOnly holds %q, which is not a heading", key)
		}
	}
	for key := range englishOnly {
		if _, ok := headings[key]; !ok {
			t.Errorf("englishOnly holds %q, which is not a heading", key)
		}
	}
}

// The language is read from the document, not from the server's setting: a
// French CV pasted into an English editor has to find its French headings.
func TestTheLanguageComesFromTheDocument(t *testing.T) {
	for _, tc := range []struct {
		name string
		text string
		want model.Lang
	}{
		{"french headings", frenchResume, model.LangFR},
		{"english headings", "Jordan Blake\n\nWORK EXPERIENCE\n\nEngineer | Acme | 2020 - 2022\nShipped the migration with a small team.\n\nEDUCATION\n\nBSc | Bristol | 2014 - 2017\n", model.LangEN},
	} {
		res, err := Run(tc.text, "")
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if res.Lang != tc.want {
			t.Errorf("%s: Lang = %q, want %q", tc.name, res.Lang, tc.want)
		}
	}
	// An explicit language is obeyed: the caller knows better than the guess.
	res, err := Run(frenchResume, model.LangEN)
	if err != nil {
		t.Fatal(err)
	}
	if res.Lang != model.LangEN {
		t.Errorf("an explicit language was overridden: got %q", res.Lang)
	}
}

// A header that prints the title on its own line, the company under it, and the
// period on that same second line, is the shape this tool's own renderer
// produces. Both halves used to be lost: the company was read as a city and the
// title was dropped on the floor.
func TestTheLayoutThisToolRendersIsReadBack(t *testing.T) {
	const rendered = `Jean Dupont
Développeur Go
Paris | jean@exemple.fr
EXPÉRIENCE PROFESSIONNELLE
Ingénieur
Acme, Paris | 2021 - Aujourd'hui
Plateforme de paiement.
- Migration vers Go, latence divisée par 4.
FORMATION
Master
SU, Paris | 2016 - 2018`

	r, err := Run(rendered, model.LangFR)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Resume.Experience) != 1 {
		t.Fatalf("found %d jobs, want 1", len(r.Resume.Experience))
	}
	e := r.Resume.Experience[0]
	if e.Title != "Ingénieur" {
		t.Errorf("title = %q, want Ingénieur", e.Title)
	}
	if e.Company != "Acme" {
		t.Errorf("company = %q, want Acme", e.Company)
	}
	if e.Location != "Paris" {
		t.Errorf("location = %q, want Paris", e.Location)
	}
	if e.Start != "2021" || e.End != "present" {
		t.Errorf("period = %q..%q, want 2021..present", e.Start, e.End)
	}
	if len(r.Resume.Education) != 1 {
		t.Fatalf("found %d schools, want 1", len(r.Resume.Education))
	}
	if s := r.Resume.Education[0].School; s != "SU" {
		t.Errorf("school = %q, want SU", s)
	}
}

// A job that states where it was, under a title printed on its own line, must
// not lose either half. "Paris, France" read as a title and a company is the
// failure this covers, and it is the one that loses the job.
func TestARepeatedWordIsNotAlwaysAPlace(t *testing.T) {
	const doc = `Claire Nantes
Développeuse
claire@example.fr
EXPÉRIENCE
Consultante
Paris, France | 2020 - 2022`

	r, err := Run(doc, model.LangFR)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Resume.Experience) != 1 {
		t.Fatalf("found %d jobs, want 1", len(r.Resume.Experience))
	}
	e := r.Resume.Experience[0]
	if e.Title != "Consultante" {
		t.Errorf("title = %q, want Consultante: the line above the period is the title", e.Title)
	}
	if e.Company != "Paris" {
		t.Errorf("company = %q, want Paris", e.Company)
	}
	if e.Location != "France" {
		t.Errorf("location = %q, want France", e.Location)
	}
}
