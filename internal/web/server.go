// Package web serves an HTTP API and a browser editor for building resumes.
//
// The server is deliberately stateless: the browser owns the draft and keeps it
// in localStorage, every request carries the whole resume JSON. That means the
// same endpoints serve the editor, a shell script or an automation, and that
// no CV data is ever written to disk or to a session store on the server.
package web

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/fanama/resume/atscv/internal/layout"
	"github.com/fanama/resume/atscv/internal/model"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

// maxBody caps the size of a posted resume. A resume is a few kilobytes of
// JSON; anything larger is a mistake or an attempt to exhaust memory.
const maxBody = 2 << 20 // 2 MiB

// Options configures the server.
type Options struct {
	// DemoJSON is a resume proposed to a visitor whose editor holds no session
	// yet. It is served, never written: the browser copies it into its own
	// localStorage, where it becomes an ordinary draft that the visitor owns.
	// Empty means the editor starts empty, as it always did.
	DemoJSON string
	// Public lifts the loopback rule. A container has to listen on 0.0.0.0 to be
	// reachable at all, and the rule exists precisely to stop a typo from
	// publishing the API. So lifting it is an explicit decision, never a default,
	// and the banner says what it means.
	Public bool
	// Addr is the listen address. It must stay on the loopback interface: the
	// server has no authentication and renders whatever it is handed.
	Addr string
	// Lang is the default editor language, "fr" or "en".
	Lang model.Lang
	// DateFormat is how periods are written in the preview and the exports.
	DateFormat layout.DateFormat
	// Accent is the RRGGBB accent color of the generated documents.
	Accent string
	// SectionRules draws the hairlines under the headings.
	SectionRules bool
	// FontSize is the body size in points.
	FontSize float64
	// FontDir is an extra directory searched for the PDF TrueType files.
	FontDir string
}

// DefaultOptions returns the options of a local server.
func DefaultOptions() Options {
	return Options{
		Addr:         "127.0.0.1:8080",
		Lang:         model.LangFR,
		DateFormat:   layout.DateNumeric,
		Accent:       "1F3864",
		SectionRules: true,
		FontSize:     10.5,
	}
}

// Server holds the handlers and the parsed templates.
type Server struct {
	opts Options
	tmpl *template.Template
	mux  *http.ServeMux
}

// New builds a server. The templates are parsed once, into a single set: a
// broken template is a programming error and must not wait for the first
// request to surface.
func New(opts Options) (*Server, error) {
	if opts.FontSize <= 0 {
		opts.FontSize = 10.5
	}
	if opts.DateFormat == "" {
		opts.DateFormat = layout.DateNumeric
	}
	if opts.Lang == "" {
		opts.Lang = model.LangFR
	}
	tmpl, err := template.New("atscv").ParseFS(templateFS,
		"templates/home.html", "templates/page.html", "templates/preview.html",
		"templates/report.html", "templates/text.html")
	if err != nil {
		return nil, fmt.Errorf("templates: %w", err)
	}
	s := &Server{opts: opts, tmpl: tmpl, mux: http.NewServeMux()}
	s.routes()
	return s, nil
}

// Handler returns the HTTP handler, wrapped with the security headers.
func (s *Server) Handler() http.Handler {
	return s.securityHeaders(s.mux)
}

// securityHeaders sets a strict policy. The editor loads nothing but its own
// files, so everything can be denied by default; a resume is personal data and
// has no reason to reach a third party.
func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Content-Security-Policy", strings.Join([]string{
			"default-src 'self'",
			"script-src 'self'",
			"style-src 'self'",
			"img-src 'self' data:",
			"connect-src 'self'",
			"form-action 'self'",
			"frame-ancestors 'none'",
			"base-uri 'none'",
			"object-src 'none'",
		}, "; "))
		next.ServeHTTP(w, r)
	})
}

func (s *Server) routes() {
	static, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic(err)
	}
	files := http.FileServer(http.FS(static))
	s.mux.Handle("GET /static/", http.StripPrefix("/static/", revalidate(static, files)))
	// The landing page is the front door, the editor sits behind it. Both come
	// from the same binary and the same stylesheet, so the site and the tool
	// cannot drift apart.
	s.mux.HandleFunc("GET /{$}", s.handleHome)
	s.mux.HandleFunc("GET /editor", s.handleEditor)
	s.mux.HandleFunc("GET /api/style.css", s.handleStyle)
	s.mux.HandleFunc("GET /api/demo", s.handleDemo)

	// The API. Every endpoint takes the resume as the "data" form field, or as
	// a JSON body when the request asks for JSON.
	s.mux.HandleFunc("POST /api/schema", s.handleSchema)
	s.mux.HandleFunc("POST /api/import", s.handleImport)
	s.mux.HandleFunc("POST /api/parse", s.handleParse)
	s.mux.HandleFunc("POST /api/lint", s.handleLint)
	s.mux.HandleFunc("POST /api/preview", s.handlePreview)
	s.mux.HandleFunc("POST /api/text", s.handleText)
	s.mux.HandleFunc("POST /api/render/{format}", s.handleRender)
	s.mux.HandleFunc("GET /api/schema", s.handleSchema)
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"status":"ok"}`)
	})
}

// revalidate serves an embedded file with a validator instead of a blind cache
// lifetime. The assets are compiled into the binary, so their content only
// changes when the binary does, and the obvious move is a long max-age. That is
// a trap: a browser keeps the old app.js for that whole window, cannot tell it
// is out of date, and the editor silently runs the previous version of the tool.
// A new binary, a new editor, an hour of confusion.
//
// So: let the browser store the file, but make it ask. The tag is a hash of the
// bytes, computed once at startup, which is also why it is worth computing it
// here rather than trusting a timestamp: an embed.FS file has no modification
// time to compare.
func revalidate(fsys fs.FS, next http.Handler) http.Handler {
	tags := make(map[string]string)
	_ = fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		raw, err := fs.ReadFile(fsys, p)
		if err != nil {
			return nil
		}
		sum := sha256.Sum256(raw)
		tags[p] = `"` + hex.EncodeToString(sum[:8]) + `"`
		return nil
	})

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tag, ok := tags[strings.TrimPrefix(r.URL.Path, "/")]
		if ok {
			w.Header().Set("ETag", tag)
			// no-cache, not no-store: the file is kept, and revalidated on the
			// next load. An unchanged answer is a 304 with no body.
			w.Header().Set("Cache-Control", "no-cache")
			if matchesETag(r.Header.Get("If-None-Match"), tag) {
				w.WriteHeader(http.StatusNotModified)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// matchesETag answers the one question a browser asks. A weak tag is compared
// without its marker, and a list is searched rather than compared whole.
func matchesETag(header, tag string) bool {
	if header == "" {
		return false
	}
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimSpace(candidate)
		candidate = strings.TrimPrefix(candidate, "W/")
		if candidate == tag || candidate == "*" {
			return true
		}
	}
	return false
}

// ListenAndServe starts the server and blocks. The address is checked against
// the loopback interface: an open API that renders any resume it is handed must
// not be reachable from the network. Options.Public is the one way past that,
// for a container behind a reverse proxy.
func (s *Server) ListenAndServe() error {
	if !s.opts.Public {
		if err := s.checkLoopback(s.opts.Addr); err != nil {
			return err
		}
	}
	srv := &http.Server{
		Addr:              s.opts.Addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		ErrorLog:          nil,
	}
	ln, err := net.Listen("tcp", s.opts.Addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", s.opts.Addr, err)
	}
	// A wildcard address is not an address a human can open. Print the loopback
	// with the port the listener actually got, which is what the operator and
	// the healthcheck both need.
	shown := ln.Addr().String()
	if host, port, err := net.SplitHostPort(shown); err == nil {
		if ip := net.ParseIP(host); ip != nil && ip.IsUnspecified() {
			shown = net.JoinHostPort("localhost", port)
		}
	}
	fmt.Printf("atscv editor on http://%s\n", shown)
	fmt.Println("the drafts live in the browser (localStorage), nothing is written on the server")
	if s.opts.Public {
		fmt.Println("WARNING: this instance is reachable from the network and has no " +
			"authentication, no rate limit and no account. Anyone who can reach it can " +
			"render the CV they post, and use the CPU this machine has. Put it behind " +
			"your own authentication, or keep it on the loopback.")
	} else {
		fmt.Println("stop with Ctrl-C")
	}
	return s.serve(srv, ln)
}

// serve runs the server until the context is cancelled, then drains the requests
// in flight. A container runtime sends SIGTERM before it kills the process, and
// a resume being rendered at that moment deserves its answer.
func (s *Server) serve(srv *http.Server, ln net.Listener) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errs := make(chan error, 1)
	go func() { errs <- srv.Serve(ln) }()
	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
		stop()
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutCtx); err != nil {
			return fmt.Errorf("shutdown: %w", err)
		}
		fmt.Println("\nstopped")
		return nil
	}
}

// checkLoopback refuses a non local address unless it is explicitly a loopback
// one, so a typo like -addr 0.0.0.0:8080 fails loudly instead of publishing the
// CV API to the network.
func (s *Server) checkLoopback(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid address %q: %w", addr, err)
	}
	if host == "localhost" {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("invalid host %q in %q, want a loopback address", host, addr)
	}
	if !ip.IsLoopback() {
		return fmt.Errorf("refusing to listen on %s: the API has no authentication, use a loopback address such as 127.0.0.1:8080", addr)
	}
	return nil
}
