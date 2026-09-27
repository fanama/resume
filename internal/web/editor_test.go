package web

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestEditorDrivesTheForm runs app.js in a miniature DOM. The Go tests cover the
// API; this one covers the only half no Go assertion can reach: that a keystroke
// writes the right key of the model, that a singleton offers no delete button,
// and that a draft survives a reload.
//
// It is skipped when node is missing: the editor has no build step and no
// dependency to install, and a test that would drag node into the build of a Go
// tool is a test nobody runs.
func TestEditorDrivesTheForm(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	dir := t.TempDir()

	page, err := os.ReadFile(filepath.Join("templates", "page.html"))
	if err != nil {
		t.Fatal(err)
	}
	ids := regexp.MustCompile(`id="([^"]+)"`).FindAllStringSubmatch(string(page), -1)
	seen := map[string]bool{}
	var declared []string
	for _, m := range ids {
		if seen[m[1]] {
			continue
		}
		seen[m[1]] = true
		declared = append(declared, m[1])
	}
	if len(declared) == 0 {
		t.Fatal("the page declares no id, the test would check nothing")
	}

	s := testServer(t)
	rec := get(t, s, "/api/schema")
	if rec.Code != 200 {
		t.Fatalf("schema status = %d", rec.Code)
	}
	var schema map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &schema); err != nil {
		t.Fatal(err)
	}
	fixture, err := json.Marshal(map[string]any{"ids": declared, "schema": schema})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "fixture.json")
	if err := os.WriteFile(path, fixture, 0o600); err != nil {
		t.Fatal(err)
	}

	out, err := exec.Command(node, filepath.Join("static", "dom_test.mjs"), path).CombinedOutput()
	if err != nil {
		t.Fatalf("the editor test failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "dom: all checks passed") {
		t.Fatalf("unexpected output:\n%s", out)
	}
}
