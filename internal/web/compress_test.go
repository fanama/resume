package web

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// gunzip returns the body a compressed answer really carried.
func gunzip(t *testing.T, body []byte) string {
	t.Helper()
	zr, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("the answer is not a gzip stream: %v", err)
	}
	out, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("reading the gzip stream: %v", err)
	}
	if err := zr.Close(); err != nil {
		t.Fatalf("closing the gzip stream: %v", err)
	}
	return string(out)
}

// getEncoded sends a GET with an Accept-Encoding and an optional validator,
// which is the only way to ask for the compressed representation.
func getEncoded(t *testing.T, s *Server, path, encoding, validator string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if encoding != "" {
		req.Header.Set("Accept-Encoding", encoding)
	}
	if validator != "" {
		req.Header.Set("If-None-Match", validator)
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

// postEncoded is post with an Accept-Encoding, which is what a browser sends.
func postEncoded(t *testing.T, s *Server, path, resume, encoding string) *httptest.ResponseRecorder {
	t.Helper()
	form := url.Values{"data": {resume}}
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if encoding != "" {
		req.Header.Set("Accept-Encoding", encoding)
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

func TestAcceptsGzip(t *testing.T) {
	for _, tc := range []struct {
		header string
		want   bool
	}{
		{"", false},
		{"gzip", true},
		{"gzip, deflate, br", true},
		{"deflate, gzip", true},
		{"identity", false},
		{"deflate", false},
		{"gzip;q=0", false},
		{"gzip;q=0.0", false},
		{"gzip;q=1", true},
		{"br;q=1, gzip;q=0.5", true},
	} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		if tc.header != "" {
			req.Header.Set("Accept-Encoding", tc.header)
		}
		if got := acceptsGzip(req); got != tc.want {
			t.Errorf("acceptsGzip(%q) = %v, want %v", tc.header, got, tc.want)
		}
	}
}

// TestStaticIsCompressedForAClientThatAsks covers the transfer size of the
// editor: pdf.js and its worker are 1.8 Mio together and a phone importing a
// PDF pays for every byte. The compressed body is a different representation,
// so it carries its own validator: a browser holding the raw copy must not get
// a 304 that points at bytes it never received.
func TestStaticIsCompressedForAClientThatAsks(t *testing.T) {
	srv := testServer(t)

	plain := get(t, srv, "/static/app.js")
	if plain.Code != http.StatusOK {
		t.Fatalf("status = %d", plain.Code)
	}
	if ce := plain.Header().Get("Content-Encoding"); ce != "" {
		t.Errorf("a client that asked for nothing got Content-Encoding %q", ce)
	}

	gz := getEncoded(t, srv, "/static/app.js", "gzip", "")
	if gz.Code != http.StatusOK {
		t.Fatalf("compressed status = %d", gz.Code)
	}
	if ce := gz.Header().Get("Content-Encoding"); ce != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", ce)
	}
	if vary := gz.Header().Get("Vary"); !strings.Contains(vary, "Accept-Encoding") {
		t.Errorf("Vary = %q, a shared cache would hand one representation to the other", vary)
	}
	if ct := gz.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/javascript") {
		t.Errorf("the compressed script is served as %q: a browser would download it instead of running it", ct)
	}
	if cl := gz.Header().Get("Content-Length"); cl != fmt.Sprint(gz.Body.Len()) {
		t.Errorf("Content-Length = %q, the body is %d bytes", cl, gz.Body.Len())
	}
	if body := gunzip(t, gz.Body.Bytes()); body != plain.Body.String() {
		t.Error("the compressed representation is not the same file")
	}
	if gz.Body.Len() >= plain.Body.Len() {
		t.Errorf("gzip made app.js bigger: %d bytes for %d", gz.Body.Len(), plain.Body.Len())
	}

	rawTag, gzTag := plain.Header().Get("ETag"), gz.Header().Get("ETag")
	if rawTag == "" || gzTag == "" {
		t.Fatal("a representation came back without an ETag")
	}
	if rawTag == gzTag {
		t.Errorf("both representations share the tag %s, so a validator for one describes the other", rawTag)
	}
	// The validator of the raw copy must not satisfy the compressed request.
	stale := getEncoded(t, srv, "/static/app.js", "gzip", rawTag)
	if stale.Code != http.StatusOK {
		t.Errorf("the raw tag against a compressed request: status %d, want 200 with the file", stale.Code)
	}
	// The compressed one must.
	fresh := getEncoded(t, srv, "/static/app.js", "gzip", gzTag)
	if fresh.Code != http.StatusNotModified {
		t.Errorf("the compressed tag: status %d, want 304", fresh.Code)
	}
	if fresh.Body.Len() != 0 {
		t.Errorf("a 304 carried %d bytes", fresh.Body.Len())
	}
}

// TestAPIAnswersAreCompressed is the half the reader feels: every edit posts
// the whole resume and gets a fragment back, and both halves are text a gzip
// pass shrinks by three or four.
func TestAPIAnswersAreCompressed(t *testing.T) {
	srv := testServer(t)
	rec := postEncoded(t, srv, "/api/preview", sample, "gzip")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if ce := rec.Header().Get("Content-Encoding"); ce != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", ce)
	}
	if vary := rec.Header().Get("Vary"); !strings.Contains(vary, "Accept-Encoding") {
		t.Errorf("Vary = %q, a shared cache would keep the wrong body", vary)
	}
	html := gunzip(t, rec.Body.Bytes())
	if !strings.Contains(html, "Jean Dupont") {
		t.Errorf("the inflated preview lost the resume: %.160s", html)
	}
	if rec.Body.Len() >= len(html) {
		t.Errorf("the answer is %d bytes compressed for %d uncompressed: gzip bought nothing", rec.Body.Len(), len(html))
	}

	// A client that did not ask for it still gets the body as it is.
	plain := post(t, srv, "/api/preview", sample)
	if ce := plain.Header().Get("Content-Encoding"); ce != "" {
		t.Errorf("a client that asked for nothing got Content-Encoding %q", ce)
	}
	if !strings.Contains(plain.Body.String(), "Jean Dupont") {
		t.Error("the plain preview lost the resume")
	}
}

// TestDocumentDownloadsAreNotCompressed pins the rule that keeps a download
// honest: these handlers promise a Content-Length, and the length they promise
// is the length of the file. Compressing after the promise would hand the
// browser a body it cannot count, and a PDF or a .docx would not shrink
// anyway.
func TestDocumentDownloadsAreNotCompressed(t *testing.T) {
	srv := testServer(t)
	for _, format := range []string{"pdf", "docx", "txt", "adoc"} {
		rec := postEncoded(t, srv, "/api/render/"+format, sample, "gzip")
		if rec.Code != http.StatusOK {
			t.Errorf("/api/render/%s status = %d: %s", format, rec.Code, rec.Body.String())
			continue
		}
		if ce := rec.Header().Get("Content-Encoding"); ce != "" {
			t.Errorf("/api/render/%s came back as %q while promising %s bytes",
				format, ce, rec.Header().Get("Content-Length"))
		}
		if cl := rec.Header().Get("Content-Length"); cl != fmt.Sprint(rec.Body.Len()) {
			t.Errorf("/api/render/%s Content-Length = %q, the body is %d bytes",
				format, cl, rec.Body.Len())
		}
	}
	pdf := postEncoded(t, srv, "/api/render/pdf", sample, "gzip")
	if !bytes.HasPrefix(pdf.Body.Bytes(), []byte("%PDF-")) {
		t.Error("the PDF download is not a PDF anymore")
	}
}
