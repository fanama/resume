package web

import (
	"reflect"
	"strings"

	"github.com/fanama/resume/atscv/internal/model"
)

// The grammar the analysis panel compiles in the browser. WebLLM turns a JSON
// Schema into a decoding constraint: the model can only emit tokens that keep
// the answer a valid document of that shape, so a rewrite or a translation
// comes back as JSON by construction instead of being repaired after the fact.
//
// The schema is derived from model.Resume by reflection rather than written by
// hand, for the same reason the editor schema is: the struct is already the one
// /api/parse enforces, and a field added to it and forgotten here would make
// the browser constrain the model to a document the server then refuses. The
// drift guard lives in jsonschema_test.go.
//
// Two properties make it usable by the grammar compiler: every object forbids
// unknown keys and lists all of its keys as required. That mirrors what the
// parser does — an unknown field is an error there too — and it is what the
// compiler's strict mode expects.
func ResumeJSONSchema() map[string]any {
	return jsonSchemaFor(reflect.TypeOf(model.Resume{}))
}

// jsonSchemaFor describes one Go type as a JSON Schema. Only the kinds the
// resume model actually uses are described: a richer converter would be a
// second schema language to keep in step with the first, and nothing needs it.
func jsonSchemaFor(t reflect.Type) map[string]any {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.String:
		// The language is the one field with a closed set of values: the
		// parser accepts exactly two, so the grammar admits exactly two.
		if t == reflect.TypeOf(model.Lang("")) {
			return map[string]any{"type": "string", "enum": []any{"fr", "en"}}
		}
		return map[string]any{"type": "string"}
	case reflect.Slice:
		return map[string]any{"type": "array", "items": jsonSchemaFor(t.Elem())}
	case reflect.Struct:
		properties := map[string]any{}
		var required []string
		for i := range t.NumField() {
			f := t.Field(i)
			name := strings.Split(f.Tag.Get("json"), ",")[0]
			if name == "" || name == "-" {
				continue
			}
			properties[name] = jsonSchemaFor(f.Type)
			required = append(required, name)
		}
		return map[string]any{
			"type":                 "object",
			"properties":           properties,
			"required":             required,
			"additionalProperties": false,
		}
	default:
		// Unreachable for this model. An empty schema would constrain
		// nothing, which is the honest description of an unknown type.
		return map[string]any{}
	}
}
