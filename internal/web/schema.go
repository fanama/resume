package web

// The editor is built from this schema instead of a hand-written HTML form per
// section: the model has eight sections with different fields, and a form that
// has to be edited every time a field is added would drift away from the JSON
// contract. The server publishes the shape, the browser renders the fields.

// FieldKind tells the browser which widget to draw.
type FieldKind string

const (
	// FieldText is a single-line input.
	FieldText FieldKind = "text"
	// FieldTextarea is a multi-line input, used for prose.
	FieldTextarea FieldKind = "textarea"
	// FieldList is a repeatable list of short strings: highlights, technologies.
	FieldList FieldKind = "list"
	// FieldEntries is a repeatable list of objects: contact links.
	FieldEntries FieldKind = "entries"
)

// Field describes one editable field of a section entry.
type Field struct {
	// Key is the JSON key, used verbatim by the browser.
	Key string `json:"key"`
	// Kind selects the widget.
	Kind FieldKind `json:"kind"`
	// LabelFR and LabelEN are the field labels.
	LabelFR string `json:"label_fr"`
	LabelEN string `json:"label_en"`
	// Placeholder is an example value, never a real datum.
	Placeholder string `json:"placeholder,omitempty"`
	// Mono asks for a monospaced font, for dates and identifiers.
	Mono bool `json:"mono,omitempty"`
}

// Section describes a repeatable section of the resume.
type Section struct {
	// Key is the JSON key of the array.
	Key string `json:"key"`
	// LabelFR and LabelEN are the section headings, the same words the renderer
	// prints so the editor and the document never disagree.
	LabelFR string `json:"label_fr"`
	LabelEN string `json:"label_en"`
	// EntryFR and EntryEN name a single entry, for the "add" button.
	EntryFR string `json:"entry_fr"`
	EntryEN string `json:"entry_en"`
	// Fields describe the entries of the array.
	Fields []Field `json:"fields"`
	// Singleton marks a section rendered as a single card instead of a list.
	Singleton bool `json:"singleton,omitempty"`
	// Root marks a singleton whose fields are the top level keys of the resume
	// rather than a nested object. The name, the headline, the summary and the
	// language live at the root of the JSON, so the editor has to write them
	// there: nesting them would build a model the parser refuses.
	Root bool `json:"root,omitempty"`
}

// ContactSchema describes the contact block, which is a struct and not an
// array: it is edited as its own card.
func ContactSchema() Section {
	return Section{
		Key:       "contact",
		LabelFR:   "Coordonnées",
		LabelEN:   "Contact",
		EntryFR:   "Coordonnées",
		EntryEN:   "Contact",
		Singleton: true,
		Fields: []Field{
			{Key: "email", Kind: FieldText, LabelFR: "E-mail", LabelEN: "Email", Placeholder: "prenom.nom@exemple.fr"},
			{Key: "phone", Kind: FieldText, LabelFR: "Téléphone", LabelEN: "Phone", Placeholder: "+33 6 12 34 56 78", Mono: true},
			{Key: "city", Kind: FieldText, LabelFR: "Ville", LabelEN: "City"},
			{Key: "region", Kind: FieldText, LabelFR: "Région", LabelEN: "Region"},
			{Key: "country", Kind: FieldText, LabelFR: "Pays", LabelEN: "Country"},
			{Key: "note", Kind: FieldText, LabelFR: "Mention", LabelEN: "Note", Placeholder: "Mobility, work authorization…"},
			{Key: "links", Kind: FieldEntries, LabelFR: "Liens", LabelEN: "Links"},
		},
	}
}

// IdentitySchema describes the top level scalar fields.
func IdentitySchema() Section {
	return Section{
		Key:       "identity",
		LabelFR:   "Identité",
		LabelEN:   "Identity",
		EntryFR:   "Identité",
		EntryEN:   "Identity",
		Singleton: true,
		Root:      true,
		Fields: []Field{
			{Key: "lang", Kind: FieldText, LabelFR: "Langue du CV (fr ou en)", LabelEN: "Resume language (fr or en)", Placeholder: "fr", Mono: true},
			{Key: "name", Kind: FieldText, LabelFR: "Nom", LabelEN: "Name"},
			{Key: "headline", Kind: FieldText, LabelFR: "Titre", LabelEN: "Headline", Placeholder: "Développeur Fullstack"},
			{Key: "summary", Kind: FieldTextarea, LabelFR: "Profil", LabelEN: "Summary", Placeholder: "Trois lignes maximum, le poste visé en premier."},
		},
	}
}

// Sections is the full editor schema, in the order the renderer prints them.
func Sections() []Section {
	return []Section{
		{
			Key: "experience", LabelFR: "Expérience", LabelEN: "Experience",
			EntryFR: "poste", EntryEN: "job",
			Fields: []Field{
				{Key: "title", Kind: FieldText, LabelFR: "Poste", LabelEN: "Title"},
				{Key: "company", Kind: FieldText, LabelFR: "Entreprise", LabelEN: "Company"},
				{Key: "location", Kind: FieldText, LabelFR: "Lieu", LabelEN: "Location"},
				{Key: "start", Kind: FieldText, LabelFR: "Début", LabelEN: "Start", Placeholder: "2023", Mono: true},
				{Key: "end", Kind: FieldText, LabelFR: "Fin", LabelEN: "End", Placeholder: "present", Mono: true},
				{Key: "summary", Kind: FieldTextarea, LabelFR: "Description", LabelEN: "Description"},
				{Key: "highlights", Kind: FieldList, LabelFR: "Réalisations", LabelEN: "Achievements"},
				{Key: "team", Kind: FieldText, LabelFR: "Équipe", LabelEN: "Team"},
				{Key: "stack", Kind: FieldList, LabelFR: "Technologies", LabelEN: "Technologies"},
			},
		},
		{
			Key: "education", LabelFR: "Formation", LabelEN: "Education",
			EntryFR: "diplôme", EntryEN: "degree",
			Fields: []Field{
				{Key: "degree", Kind: FieldText, LabelFR: "Diplôme", LabelEN: "Degree"},
				{Key: "school", Kind: FieldText, LabelFR: "École", LabelEN: "School"},
				{Key: "location", Kind: FieldText, LabelFR: "Lieu", LabelEN: "Location"},
				{Key: "start", Kind: FieldText, LabelFR: "Début", LabelEN: "Start", Mono: true},
				{Key: "end", Kind: FieldText, LabelFR: "Fin", LabelEN: "End", Mono: true},
				{Key: "summary", Kind: FieldTextarea, LabelFR: "Description", LabelEN: "Description"},
				{Key: "coursework", Kind: FieldList, LabelFR: "Matières", LabelEN: "Coursework"},
			},
		},
		{
			Key: "skills", LabelFR: "Compétences", LabelEN: "Skills",
			EntryFR: "catégorie", EntryEN: "category",
			Fields: []Field{
				{Key: "category", Kind: FieldText, LabelFR: "Catégorie", LabelEN: "Category"},
				{Key: "items", Kind: FieldList, LabelFR: "Éléments", LabelEN: "Items"},
			},
		},
		{
			Key: "certifications", LabelFR: "Certifications", LabelEN: "Certifications",
			EntryFR: "certification", EntryEN: "certification",
			Fields: []Field{
				{Key: "name", Kind: FieldText, LabelFR: "Nom", LabelEN: "Name"},
				{Key: "issuer", Kind: FieldText, LabelFR: "Organisme", LabelEN: "Issuer"},
				{Key: "date", Kind: FieldText, LabelFR: "Date", LabelEN: "Date", Mono: true},
				{Key: "id", Kind: FieldText, LabelFR: "Identifiant", LabelEN: "ID", Mono: true},
			},
		},
		{
			Key: "projects", LabelFR: "Projets", LabelEN: "Projects",
			EntryFR: "projet", EntryEN: "project",
			Fields: []Field{
				{Key: "name", Kind: FieldText, LabelFR: "Nom", LabelEN: "Name"},
				{Key: "url", Kind: FieldText, LabelFR: "URL", LabelEN: "URL", Mono: true},
				{Key: "start", Kind: FieldText, LabelFR: "Début", LabelEN: "Start", Mono: true},
				{Key: "end", Kind: FieldText, LabelFR: "Fin", LabelEN: "End", Mono: true},
				{Key: "summary", Kind: FieldTextarea, LabelFR: "Description", LabelEN: "Description"},
				{Key: "highlights", Kind: FieldList, LabelFR: "Réalisations", LabelEN: "Achievements"},
				{Key: "technologies", Kind: FieldList, LabelFR: "Technologies", LabelEN: "Technologies"},
			},
		},
		{
			Key: "activities", LabelFR: "Activités", LabelEN: "Activities",
			EntryFR: "activité", EntryEN: "activity",
			Fields: []Field{
				{Key: "name", Kind: FieldText, LabelFR: "Nom", LabelEN: "Name"},
				{Key: "organization", Kind: FieldText, LabelFR: "Organisation", LabelEN: "Organization"},
				{Key: "location", Kind: FieldText, LabelFR: "Lieu", LabelEN: "Location"},
				{Key: "start", Kind: FieldText, LabelFR: "Début", LabelEN: "Start", Mono: true},
				{Key: "end", Kind: FieldText, LabelFR: "Fin", LabelEN: "End", Mono: true},
				{Key: "summary", Kind: FieldTextarea, LabelFR: "Description", LabelEN: "Description"},
				{Key: "highlights", Kind: FieldList, LabelFR: "Réalisations", LabelEN: "Achievements"},
			},
		},
		{
			Key: "languages", LabelFR: "Langues", LabelEN: "Languages",
			EntryFR: "langue", EntryEN: "language",
			Fields: []Field{
				{Key: "name", Kind: FieldText, LabelFR: "Langue", LabelEN: "Language"},
				{Key: "level", Kind: FieldText, LabelFR: "Niveau", LabelEN: "Level"},
			},
		},
	}
}
