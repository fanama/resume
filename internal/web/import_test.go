package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// postImport sends extracted text to the endpoint the way the browser does.
func postImport(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/import", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	testServer(t).Handler().ServeHTTP(rec, req)
	return rec
}

func TestImportFillsAResumeFromExtractedText(t *testing.T) {
	rec := postImport(t, `{"text":"Camille Fontaine\nIngénieure logiciel\ncamille@example.fr\n\nEXPERIENCE\n\nIngénieure | Nexteo | 03/2021 - présent\nMigration du monolithe vers Go."}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	var got importResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Resume.Name != "Camille Fontaine" {
		t.Errorf("name = %q", got.Resume.Name)
	}
	if len(got.Resume.Experience) != 1 {
		t.Fatalf("experience = %d, want 1", len(got.Resume.Experience))
	}
	if got.Resume.Experience[0].Company != "Nexteo" {
		t.Errorf("company = %q, want Nexteo", got.Resume.Experience[0].Company)
	}
	if got.Resume.Experience[0].End != "present" {
		t.Errorf("end = %q, want present", got.Resume.Experience[0].End)
	}
}

// A PDF with no text layer is the one case the answer has to explain, and 422
// is the code that says "the request was fine, the content is not usable".
func TestImportExplainsAPDFWithoutText(t *testing.T) {
	rec := postImport(t, `{"text":"   "}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422: an empty draft would look like a success", rec.Code)
	}
	var body errorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.ToLower(body.Error), "scan") {
		t.Errorf("error = %q, want it to name the scan as the likely cause", body.Error)
	}
}

func TestImportRejectsABodyItCannotRead(t *testing.T) {
	for name, body := range map[string]string{
		"not json":     `nope`,
		"no text":      `{"lang":"fr"}`,
		"json array":   `["text"]`,
		"a pdf itself": `{"text":"` + strings.Repeat("a", maxImportText) + `"}`,
	} {
		rec := postImport(t, body)
		if rec.Code == http.StatusOK {
			t.Errorf("%s: status = 200, want a rejection", name)
		}
	}
}

// The endpoint is public like the rest of the API, so an unknown language hint
// must not be able to fail the request: it is a hint, not a contract.
func TestImportIgnoresALanguageHintItDoesNotKnow(t *testing.T) {
	rec := postImport(t, `{"lang":"klingon","text":"Jordan Blake\nSenior Engineer\n\nWORK EXPERIENCE\n\nEngineer | Acme | 2020 - 2022\nShipped the migration with a small team.\n\nEDUCATION\n\nBSc | Bristol | 2014 - 2017\n"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	var got importResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Lang != "en" {
		t.Errorf("lang = %q, want en, read from the document", got.Lang)
	}
}

// A section the schema cannot hold comes back in the answer instead of being
// dropped, so the editor can tell the candidate their content is missing.
func TestImportReportsTheSectionsItCouldNotPlace(t *testing.T) {
	rec := postImport(t, `{"text":"Camille Fontaine\n\nEXPERIENCE\n\nEngineer | Acme | 2020 - 2022\nShipped things.\n\nREFERENCES\n\nA referee, reachable on request."}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	var got importResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Unmapped) != 1 || !strings.Contains(got.Unmapped[0], "REFERENCES") {
		t.Errorf("unmapped = %q, want the REFERENCES heading reported", got.Unmapped)
	}
}

// A body larger than the bound is refused before it is parsed, so a public
// endpoint cannot be handed an unbounded document.
func TestImportBoundsTheBody(t *testing.T) {
	big := bytes.Repeat([]byte("a"), maxImportText+1024)
	rec := postImport(t, `{"text":"`+string(big)+`"}`)
	if rec.Code == http.StatusOK {
		t.Error("an oversized body was accepted")
	}
}
