package web

// Compression. Nothing in this app is served compressed by default, and the
// assets are large: pdf.js and its worker are 1.8 Mio on their own, which a
// phone on a train pays for every time it imports a PDF. Two mechanisms, both
// decided by the client's Accept-Encoding:
//
//   - the embedded files are compressed once, when the server starts, and go
//     out ready made: no CPU per request, and a tag computed on the bytes that
//     actually travel, so a validator still means what it says;
//   - the API answers are compressed as they are written, because they are
//     built per request and are all of a few kilobytes.

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
)

// staticAsset is one embedded file with the two representations the server can
// hand out and the validator of each.
type staticAsset struct {
	raw    []byte
	gz     []byte // nil when compression would not save anything
	rawTag string
	gzTag  string
}

// serveAssets answers /static/. Every file is read and compressed once, here,
// and a request is a lookup: the body is already in memory and the tag is
// already known.
//
// The assets are compiled into the binary, so their content only changes when
// the binary does, and the obvious move is a long max-age. That is a trap: a
// browser keeps the old app.js for that whole window, cannot tell it is out of
// date, and the editor silently runs the previous version of the tool. So the
// file is kept, but revalidated: no-cache, and a tag that is a hash of the
// bytes. An embed.FS file has no modification time to compare, which is why the
// tag is computed here rather than taken from a timestamp.
//
// The compressed form is a different representation, so it carries its own tag:
// a browser holding the raw bytes must not satisfy a compressed request with a
// 304 that points at them.
func serveAssets(fsys fs.FS) http.Handler {
	assets := make(map[string]staticAsset)
	_ = fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		raw, err := fs.ReadFile(fsys, p)
		if err != nil {
			return nil
		}
		a := staticAsset{raw: raw, rawTag: tagOf(raw)}
		if gz, err := gzipBytes(raw); err == nil && len(gz) < len(raw) {
			a.gz, a.gzTag = gz, tagOf(gz)
		}
		assets[p] = a
		return nil
	})
	files := http.FileServer(http.FS(fsys))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		a, ok := assets[name]
		if !ok {
			// Not one of ours: a missing file keeps the answer of the file
			// server, which is the 404 that says so.
			files.ServeHTTP(w, r)
			return
		}
		body, tag := a.raw, a.rawTag
		gzipped := a.gz != nil && acceptsGzip(r)
		if gzipped {
			body, tag = a.gz, a.gzTag
		}
		h := w.Header()
		if gzipped {
			// Set before the validator: a 304 describes the representation the
			// client asked for, encoding included.
			h.Set("Content-Encoding", "gzip")
		}
		h.Set("ETag", tag)
		// no-cache, not no-store: the file is kept, and revalidated on the next
		// load. An unchanged answer is a 304 with no body.
		h.Set("Cache-Control", "no-cache")
		h.Set("Vary", "Accept-Encoding")
		if matchesETag(r.Header.Get("If-None-Match"), tag) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		// The type is decided from the original bytes: a compressed body
		// sniffs as a gzip archive and the browser would download the script
		// instead of running it.
		if ct := mime.TypeByExtension(path.Ext(name)); ct != "" {
			h.Set("Content-Type", ct)
		} else {
			h.Set("Content-Type", http.DetectContentType(a.raw))
		}
		h.Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	})
}

// tagOf is the validator of a body: a hash of the bytes, quoted the way a
// strong ETag is quoted.
func tagOf(raw []byte) string {
	sum := sha256.Sum256(raw)
	return `"` + hex.EncodeToString(sum[:8]) + `"`
}

// gzipBytes compresses at the fastest level that still saves real space. This
// runs once per file at startup, not per request, so the trade is latency of
// the first answer against CPU of every answer.
func gzipBytes(raw []byte) ([]byte, error) {
	var buf bytes.Buffer
	zw, err := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
	if err != nil {
		return nil, err
	}
	if _, err := zw.Write(raw); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// acceptsGzip reports whether the client can read a gzip body. An explicit
// q=0 is a refusal, not an absence.
func acceptsGzip(r *http.Request) bool {
	for _, enc := range strings.Split(r.Header.Get("Accept-Encoding"), ",") {
		part := strings.TrimSpace(enc)
		if !strings.HasPrefix(strings.ToLower(part), "gzip") {
			continue
		}
		for _, param := range strings.Split(part, ";")[1:] {
			param = strings.TrimSpace(param)
			if !strings.HasPrefix(param, "q=") {
				continue
			}
			if q, err := strconv.ParseFloat(strings.TrimPrefix(param, "q="), 64); err == nil && q <= 0 {
				return false
			}
		}
		return true
	}
	return false
}

// gzipResponses compresses what the API writes when the client can read it.
// The static files are left alone: they arrive already compressed, from
// serveAssets, with a tag of their own.
func gzipResponses(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/static/") || !acceptsGzip(r) {
			next.ServeHTTP(w, r)
			return
		}
		// Whatever comes back varies by the request encoding, and a shared
		// cache has to be told so even if this answer turns out empty.
		w.Header().Add("Vary", "Accept-Encoding")
		g := &gzipResponse{ResponseWriter: w, status: http.StatusOK, accepts: true}
		defer g.finish()
		next.ServeHTTP(g, r)
	})
}

// gzipResponse delays every decision until the first byte is written: the
// content type may still be unset, in which case the bytes themselves say what
// they are, and a Content-Length the handler already promised is a promise
// about the body as it is, not as it would be compressed.
type gzipResponse struct {
	http.ResponseWriter
	status   int
	accepts  bool
	started  bool
	compress bool
	zw       *gzip.Writer
}

func (g *gzipResponse) WriteHeader(code int) {
	if g.started {
		return
	}
	g.status = code
	// Not flushed yet: see Write. A handler that returns without writing a
	// byte still gets its status out, in finish.
}

func (g *gzipResponse) Write(b []byte) (int, error) {
	if !g.started {
		g.start(b)
	}
	if g.zw != nil {
		return g.zw.Write(b)
	}
	return g.ResponseWriter.Write(b)
}

// start fixes the headers and only then lets the status through. The type is
// the one the handler set, or the one the first bytes declare.
func (g *gzipResponse) start(b []byte) {
	g.started = true
	h := g.Header()
	_, promisedLength := h["Content-Length"]
	ctype := h.Get("Content-Type")
	if ctype == "" && len(b) > 0 {
		ctype = http.DetectContentType(b)
		h.Set("Content-Type", ctype)
	}
	g.compress = !promisedLength && compressible(ctype)
	if g.compress {
		h.Set("Content-Encoding", "gzip")
		h.Del("Content-Length")
		g.zw, _ = gzip.NewWriterLevel(g.ResponseWriter, gzip.BestSpeed)
	}
	g.ResponseWriter.WriteHeader(g.status)
}

// finish flushes the encoder and, for a handler that only ever set a status,
// that status. The footer of a gzip body has to reach the client before the
// server closes the connection, which is why this is a defer and not the end
// of the handler.
func (g *gzipResponse) finish() {
	if g.zw != nil {
		_ = g.zw.Close()
		return
	}
	if !g.started {
		g.ResponseWriter.WriteHeader(g.status)
	}
}

// Flush is part of http.ResponseWriter's optional interface: without it a
// streaming handler would silently stop flushing through the compressor.
func (g *gzipResponse) Flush() {
	if g.zw != nil {
		_ = g.zw.Flush()
	}
	if f, ok := g.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// compressible is the media types worth the CPU. The downloads of the documents
// are not among them: a PDF is already compressed, a .docx is a zip, and both
// arrive with a Content-Length the handler has already promised.
func compressible(ctype string) bool {
	media := strings.ToLower(strings.TrimSpace(strings.SplitN(ctype, ";", 2)[0]))
	switch media {
	case "application/json", "application/javascript", "application/x-javascript",
		"application/xhtml+xml", "application/manifest+json", "image/svg+xml",
		"application/xml", "application/ld+json":
		return true
	}
	return strings.HasPrefix(media, "text/")
}
