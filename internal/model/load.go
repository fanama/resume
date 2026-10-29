package model

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Load reads and decodes a resume from a JSON file.
func Load(path string) (*Resume, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	r, err := Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return r, nil
}

// Parse decodes a resume from raw JSON. An unknown field is an error rather
// than a silent no-op: a typo in a key is the most common reason a resume
// renders with a section missing.
func Parse(raw []byte) (*Resume, error) {
	var r Resume
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&r); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	if r.Lang == "" {
		r.Lang = LangFR
	}
	if _, err := ParseLang(string(r.Lang)); err != nil {
		return nil, err
	}
	r.Normalize()
	return &r, nil
}
