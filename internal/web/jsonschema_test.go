package web

import (
	"encoding/json"
	"net/http"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/fanama/resume/atscv/internal/model"
)

// TestResumeJSONSchemaCoversTheModel is the drift guard for the decoding
// grammar. The browser constrains the model with this schema and then hands
// the answer to /api/parse, which rejects unknown fields: a section added to
// model.Resume and forgotten here would make the grammar demand a document the
// parser refuses, and every rewrite would fail. The check walks the struct and
// the schema together.
func TestResumeJSONSchemaCoversTheModel(t *testing.T) {
	schema := ResumeJSONSchema()
	if schema["type"] != "object" {
		t.Fatalf("the root is a %v, want object", schema["type"])
	}
	checkSchemaCovers(t, "Resume", reflect.TypeOf(model.Resume{}), schema)
}

// checkSchemaCovers compares one Go type against one schema node: every key of
// the type must be a described property, and every property must be a key of
// the type. Equal both ways, or the grammar and the parser disagree.
func checkSchemaCovers(t *testing.T, path string, typ reflect.Type, schema map[string]any) {
	t.Helper()
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}

	switch typ.Kind() {
	case reflect.String:
		if schema["type"] != "string" {
			t.Errorf("%s is a string in Go and %v in the schema", path, schema["type"])
		}
	case reflect.Slice:
		if schema["type"] != "array" {
			t.Fatalf("%s is a slice in Go and %v in the schema", path, schema["type"])
		}
		items, ok := schema["items"].(map[string]any)
		if !ok {
			t.Fatalf("%s has no items, the grammar would accept any array", path)
		}
		checkSchemaCovers(t, path+"[]", typ.Elem(), items)
	case reflect.Struct:
		if schema["type"] != "object" {
			t.Fatalf("%s is a struct in Go and %v in the schema", path, schema["type"])
		}
		properties, ok := schema["properties"].(map[string]any)
		if !ok {
			t.Fatalf("%s describes no properties", path)
		}
		want := map[string]reflect.Type{}
		for _, f := range fieldsOf(reflect.New(typ).Elem().Interface()) {
			want[f.key] = f.typ
		}
		for key, sub := range want {
			node, ok := properties[key].(map[string]any)
			if !ok {
				t.Errorf("%s.%s is in the model and not in the grammar", path, key)
				continue
			}
			checkSchemaCovers(t, path+"."+key, sub, node)
		}
		for key := range properties {
			if _, ok := want[key]; !ok {
				t.Errorf("%s.%s is in the grammar and not in the model: the parser would reject it", path, key)
			}
		}
	default:
		t.Errorf("%s is a %v, a kind the converter does not describe", path, typ.Kind())
	}
}

// TestResumeJSONSchemaIsStrict pins the two properties the grammar compiler
// needs: every object forbids unknown keys, and every object lists every key
// as required. Without them the constraint is looser than /api/parse, and the
// model is free to emit exactly the defects the gate then has to catch.
func TestResumeJSONSchemaIsStrict(t *testing.T) {
	walkSchemaObjects(t, "Resume", ResumeJSONSchema(), func(t *testing.T, path string, node map[string]any) {
		if node["additionalProperties"] != false {
			t.Errorf("%s accepts unknown keys; the parser refuses them", path)
		}
		properties, _ := node["properties"].(map[string]any)
		required, _ := node["required"].([]string)
		if len(required) != len(properties) {
			t.Fatalf("%s requires %d of its %d keys; strict mode wants them all", path, len(required), len(properties))
		}
		seen := map[string]bool{}
		for _, key := range required {
			if _, ok := properties[key]; !ok {
				t.Errorf("%s requires %q, which is not a property", path, key)
			}
			seen[key] = true
		}
		for key := range properties {
			if !seen[key] {
				t.Errorf("%s.%s is optional, so the model may omit it and the grammar still accepts the answer", path, key)
			}
		}
	})
}

// walkSchemaObjects visits every object node of a schema, whatever its depth.
func walkSchemaObjects(t *testing.T, path string, node map[string]any, check func(*testing.T, string, map[string]any)) {
	t.Helper()
	switch node["type"] {
	case "object":
		check(t, path, node)
		properties, _ := node["properties"].(map[string]any)
		for key, sub := range properties {
			if child, ok := sub.(map[string]any); ok {
				walkSchemaObjects(t, path+"."+key, child, check)
			}
		}
	case "array":
		if items, ok := node["items"].(map[string]any); ok {
			walkSchemaObjects(t, path+"[]", items, check)
		}
	}
}

// TestResumeJSONSchemaPinsTheLanguage covers the one field with a closed set of
// values. The parser accepts exactly two tags, so the grammar should admit
// exactly two: a model free to write "fr-FR" or "english" wastes a reply on a
// value the server normalizes right back.
func TestResumeJSONSchemaPinsTheLanguage(t *testing.T) {
	properties, _ := ResumeJSONSchema()["properties"].(map[string]any)
	lang, ok := properties["lang"].(map[string]any)
	if !ok {
		t.Fatal("the grammar has no lang property")
	}
	values, ok := lang["enum"].([]any)
	if !ok {
		t.Fatalf("lang is %v, an open string; the two tags the parser accepts are not pinned", lang)
	}
	got := map[string]bool{}
	for _, v := range values {
		s, _ := v.(string)
		got[s] = true
	}
	for _, want := range []string{"fr", "en"} {
		if !got[want] {
			t.Errorf("the grammar does not admit %q", want)
		}
	}
	if len(got) != 2 {
		t.Errorf("the grammar admits %d languages, want the two the parser knows", len(got))
	}
}

// TestSchemaPublishesTheGrammar is the other end of the drift guard: the
// analysis panel reads the constraint from /api/schema, so a schema that
// stopped being published, or that stopped being serializable, would silently
// turn every rewrite back into free text with no visible reason.
func TestSchemaPublishesTheGrammar(t *testing.T) {
	s := testServer(t)
	rec := get(t, s, "/api/schema")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/schema = %d", rec.Code)
	}
	var published struct {
		ResumeSchema map[string]any `json:"resumeSchema"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &published); err != nil {
		t.Fatalf("the schema is not readable: %v", err)
	}
	if published.ResumeSchema == nil {
		t.Fatal("/api/schema publishes no resumeSchema: the panel has nothing to constrain the model with")
	}
	if published.ResumeSchema["type"] != "object" {
		t.Errorf("the published grammar is a %v", published.ResumeSchema["type"])
	}
	// The panel stringifies this object into response_format.schema, so it has
	// to survive a round trip through JSON untouched.
	encoded, err := json.Marshal(published.ResumeSchema)
	if err != nil {
		t.Fatalf("the grammar does not survive marshalling: %v", err)
	}
	if !strings.Contains(string(encoded), `"additionalProperties":false`) {
		t.Error("the published grammar lost its strictness on the way out")
	}
}

// TestTheGrammarAdmitsOnlyWhatTheParserAccepts is the property the whole
// feature rests on: if the decoding grammar admits a document, /api/parse must
// accept it. The two are derived from the same struct precisely so this holds,
// and a schema stricter than the validator is exactly what is wanted — the
// grammar may forbid what the parser would tolerate, never the reverse, or a
// successful rewrite would be thrown away by the gate that follows it.
//
// The document is built by walking the schema, which is the only honest way to
// ask "is everything this grammar allows also valid?": the schema is the
// permit, and the server is the judge.
func TestTheGrammarAdmitsOnlyWhatTheParserAccepts(t *testing.T) {
	doc := documentOfShape(t, ResumeJSONSchema())
	doc["name"] = "Marie Lefèvre"

	// The empty document must parse: it is the smallest thing the grammar lets
	// the model emit, and the one a failing rewrite is most likely to produce.
	body, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	rec := post(t, testServer(t), "/api/parse", string(body))
	if rec.Code != http.StatusOK {
		t.Fatalf("a document of the grammar's own shape was refused: %d %s", rec.Code, rec.Body)
	}

	// And with one entry filled in, which is what a real rewrite returns.
	filled := documentOfShape(t, ResumeJSONSchema())
	filled["name"] = "Marie Lefèvre"
	filled["experience"] = []any{map[string]any{
		"title": "Ingénieure", "company": "Acme", "location": "Lyon",
		"start": "2021", "end": "", "summary": "", "team": "",
		"highlights": []any{"Migration vers Go, latence divisée par 3."},
		"stack":      []any{"Go"},
	}}
	body, err = json.Marshal(filled)
	if err != nil {
		t.Fatal(err)
	}
	rec = post(t, testServer(t), "/api/parse", string(body))
	if rec.Code != http.StatusOK {
		t.Fatalf("a filled document of the grammar's shape was refused: %d %s", rec.Code, rec.Body)
	}
}

// documentOfShape builds the smallest document a schema admits: every key the
// grammar requires, holding the smallest value of its type. Arrays are empty,
// because that is what a rewrite of an absent section produces.
func documentOfShape(t *testing.T, schema map[string]any) map[string]any {
	t.Helper()
	out := map[string]any{}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("the schema describes no properties: %v", schema)
	}
	for key, raw := range properties {
		node, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("%s is not a schema node: %v", key, raw)
		}
		out[key] = valueOfShape(t, key, node)
	}
	return out
}

func valueOfShape(t *testing.T, path string, node map[string]any) any {
	t.Helper()
	switch node["type"] {
	case "string":
		if values, ok := node["enum"].([]any); ok && len(values) > 0 {
			first, _ := values[0].(string)
			return first
		}
		return ""
	case "array":
		return []any{}
	case "object":
		return documentOfShape(t, node)
	default:
		t.Fatalf("%s is a %v, a type the grammar should not admit", path, node["type"])
		return nil
	}
}

// TestTheAnalysisUsesTheGrammar is the wiring guard. The schema is useless
// unless the panel asks for it and hands it to the engine as a response
// format: a published schema nobody reads constrains nothing, and the failure
// is invisible — the rewrite still works, just without its guarantee. The
// browser half cannot be exercised here, so the served file is read for the
// two calls that make the constraint real.
func TestTheAnalysisUsesTheGrammar(t *testing.T) {
	s := testServer(t)
	rec := get(t, s, "/static/analysis.js")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /static/analysis.js = %d", rec.Code)
	}
	script := rec.Body.String()
	for _, want := range []struct{ needle, why string }{
		{"'/api/schema'", "the panel never fetches the schema the server publishes"},
		{"resumeSchema", "the panel ignores the schema key it is served"},
		{"response_format", "the panel never passes the constraint to the engine"},
		{"json_object", "the constraint is not a JSON-schema response format"},
	} {
		if !strings.Contains(script, want.needle) {
			t.Errorf("analysis.js contains no %s: %s", want.needle, want.why)
		}
	}
}

// fieldPath matches the shape the translation rules spell a field in: the
// dotted path of the published schema, with [] for the arrays. It is what tells
// a named field apart from the backticks that live in the code fences of
// extractJSON, which are not paths at all.
var fieldPath = regexp.MustCompile(`^[a-z][A-Za-z]*(?:\[\])?(?:\.[a-z][A-Za-z]*(?:\[\])?)*$`)

// TestTheTranslationNamesEveryFieldOfTheDocument is the drift guard for the
// translation, and the reason it can be called complete. The prompt spells out
// every field of the document by name, so a field added to model.Resume and
// forgotten there would come back in the source language — a defect only a
// reader of the translated CV would see, in a document that passed every other
// check. The schema names the fields, the prompt must name all of them, and
// each one belongs to exactly one of the three lists: to translate, to
// translate around the proper nouns, or to leave alone.
func TestTheTranslationNamesEveryFieldOfTheDocument(t *testing.T) {
	rec := get(t, testServer(t), "/static/analysis.js")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /static/analysis.js = %d", rec.Code)
	}
	script := rec.Body.String()

	named := map[string]int{}
	for _, m := range regexp.MustCompile("`([^`]+)`").FindAllStringSubmatch(script, -1) {
		if fieldPath.MatchString(m[1]) {
			named[m[1]]++
		}
	}

	leaves := schemaLeaves(ResumeJSONSchema(), "")
	if len(leaves) < 40 {
		t.Fatalf("the schema has only %d leaves: the walk is not reading the document", len(leaves))
	}
	for _, path := range leaves {
		switch named[path] {
		case 1: // named once, in one of the three lists
		case 0:
			t.Errorf("the translation rules never name %s: that field would come back in the source language", path)
		default:
			t.Errorf("the translation rules name %s %d times: it belongs to exactly one list", path, named[path])
		}
	}
	known := map[string]bool{}
	for _, path := range leaves {
		known[path] = true
	}
	for path := range named {
		if !known[path] {
			t.Errorf("the translation rules name %s, which is not a field of the document: the rules and the schema disagree", path)
		}
	}
}

// TestTheTranslationIsCheckedAgainstTheDocumentItCameFrom guards the second half
// of the promise the prompt makes. Naming every field is a request, not a
// result: this is the read-back, the only thing that notices a field the model
// translated away, an entry it dropped, a URL it "cleaned up" or a lang it
// forgot to switch. It runs in the browser, where no test can follow it, so the
// served file is read for the wiring — the comparison exists, it is called on a
// translation and only there, and its verdict travels the same road as a
// refusal from the server.
func TestTheTranslationIsCheckedAgainstTheDocumentItCameFrom(t *testing.T) {
	rec := get(t, testServer(t), "/static/analysis.js")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /static/analysis.js = %d", rec.Code)
	}
	script := rec.Body.String()
	for _, want := range []struct{ needle, why string }{
		{"untouched", "nothing compares the translation to the document it was written from: the prompt's promise is unchecked"},
		{"mode === 'translate' ? untouched(", "the comparison is not called on a translation, or it runs on the optimization too, where rewording is the job"},
		{"damaged.join(", "the comparison's verdict is discarded instead of being handed back"},
		{"'lang', target, after.lang", "a translation whose lang never switched would be applied under the old language, and its labels with it"},
		{"'name', before.name, after.name", "the name is not pinned: a translation may not rename the person"},
		{"'contact.email', before.contact.email", "the contact details are not pinned"},
		{"'contact.links[].url'", "the URLs are not pinned: a retyped URL is a dead link in a resume"},
		{"'certifications[].date'", "the dates are not pinned: a translated date is a resume no ATS can sort"},
	} {
		if !strings.Contains(script, want.needle) {
			t.Errorf("analysis.js contains no %s: %s", want.needle, want.why)
		}
	}
}

// schemaLeaves lists every leaf of a schema as the path the translation rules
// spell: a string is a leaf, an array of strings is a leaf with a trailing [],
// and a nested object keeps walking. The root lang and name are leaves too —
// they are fields of the document like any other.
func schemaLeaves(schema map[string]any, prefix string) []string {
	properties, _ := schema["properties"].(map[string]any)
	var out []string
	for _, key := range sortedKeys(properties) {
		node, _ := properties[key].(map[string]any)
		path := prefix + key
		switch node["type"] {
		case "object":
			out = append(out, schemaLeaves(node, path+".")...)
		case "array":
			items, _ := node["items"].(map[string]any)
			if items["type"] == "object" {
				out = append(out, schemaLeaves(items, path+"[].")...)
			} else {
				out = append(out, path+"[]")
			}
		default: // a string, the lang enum included
			out = append(out, path)
		}
	}
	return out
}

func sortedKeys(properties map[string]any) []string {
	keys := make([]string, 0, len(properties))
	for key := range properties {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// TestTheRewriteGetsOneSecondChance guards the refusal path. It used to end on
// "try again" and leave the reader to press the button, discarding the one
// thing the run does know — what was wrong. The retry lives in the browser,
// where no test can reach it, so the served file is read for the pieces that
// make it real: a bounded loop, the defect kept, the defect handed back, and
// the stop reason that tells a cut answer from a written one.
func TestTheRewriteGetsOneSecondChance(t *testing.T) {
	s := testServer(t)
	rec := get(t, s, "/static/analysis.js")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /static/analysis.js = %d", rec.Code)
	}
	script := rec.Body.String()

	// Bounded: one retry, never a loop. A loop would generate forever on a
	// model that keeps making the same mistake, and the reader would watch it.
	if !strings.Contains(script, "attempt < 2") {
		t.Error("analysis.js has no bounded second attempt: a refusal is still a dead end, or a retry without end")
	}
	if strings.Contains(script, "while (attempt") {
		t.Error("analysis.js retries in a while loop: a model that keeps refusing would generate forever")
	}

	// Kept and handed back: the defect is what the second attempt is told.
	if !strings.Contains(script, "defect") {
		t.Error("the defect is thrown away: the second attempt is not told what was wrong")
	}
	if !strings.Contains(script, "role: 'user'") || !strings.Contains(script, "messages.push") {
		t.Error("the second attempt is not given the defect as a further message")
	}

	// A cut answer and a written one end the same way on screen: only the stop
	// reason tells them apart, and only the cut one is worth retrying blind.
	if !strings.Contains(script, "finish_reason") {
		t.Error("analysis.js ignores the stop reason: a truncated answer is reported as a bad document")
	}
	if !strings.Contains(script, "'length'") {
		t.Error("the token limit is not recognised as the cause of a cut answer")
	}
}

// TestTheAnswerIsRenderedByASafeParser guards how an answer is shown. The
// model writes markdown, and showing it raw — « ## Points forts », « **Go ** »
// — makes the panel harder to read; stripping those marks means rendering,
// and rendering is exactly where untrusted text becomes dangerous. So the
// renderer must be present and must be the safe kind: every node it builds
// starts empty, from createElement, and its text travels by textContent —
// innerHTML, the parsers, the markup setters are nowhere in the file.
func TestTheAnswerIsRenderedByASafeParser(t *testing.T) {
	rec := get(t, testServer(t), "/static/analysis.js")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /static/analysis.js = %d", rec.Code)
	}
	script := rec.Body.String()

	for _, want := range []struct{ needle, why string }{
		{"renderAnswer", "the streamed answer is shown raw, markdown symbols and all"},
		{"createElement", "the answer's structure is built some other way"},
		{"createTextNode", "even plain text is not handed to the DOM as text"},
		{"ia-md", "the pretty answer ships without the class that styles it"},
	} {
		if !strings.Contains(script, want.needle) {
			t.Errorf("analysis.js contains no %s: %s", want.needle, want.why)
		}
	}
	for _, banned := range []string{".innerHTML", "outerHTML", "insertAdjacentHTML", "DOMParser", "document.write"} {
		if strings.Contains(script, banned) {
			t.Errorf("analysis.js contains %s: an answer the model wrote was never parsed as markup", banned)
		}
	}
}
