package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode"

	"github.com/fanama/resume/atscv/internal/layout"
	"github.com/fanama/resume/atscv/internal/lint"
	"github.com/fanama/resume/atscv/internal/model"
	"github.com/fanama/resume/atscv/internal/render/adoc"
	"github.com/fanama/resume/atscv/internal/render/docx"
	"github.com/fanama/resume/atscv/internal/render/pdf"
	"github.com/fanama/resume/atscv/internal/render/txt"
)

// input is a decoded request: the resume plus the layout every renderer shares.
type input struct {
	resume *model.Resume
	layout *layout.Layout
	lang   model.Lang
	name   string
}

// readInput decodes the resume from the request. The payload is either a form
// field named "data" or a raw JSON body, so the same endpoints serve the editor
// and a curl call.
func (s *Server) readInput(w http.ResponseWriter, r *http.Request) (input, error) {
	if r.Method != http.MethodPost {
		return input{}, fmt.Errorf("use POST, not %s", r.Method)
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	var raw []byte
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			return input{}, fmt.Errorf("read body: %w", err)
		}
		raw = body
	} else {
		if err := r.ParseForm(); err != nil {
			return input{}, fmt.Errorf("read form: %w", err)
		}
		raw = []byte(r.PostFormValue("data"))
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return input{}, fmt.Errorf(`no resume in the request: send the JSON in the "data" form field or as a JSON body`)
	}
	resume, err := model.Parse(raw)
	if err != nil {
		return input{}, err
	}
	lang := resume.Lang
	if lang == "" {
		lang = s.opts.Lang
	}
	l := layout.New(resume, layout.Options{
		Lang:            lang,
		DateFormat:      s.opts.DateFormat,
		StripDiacritics: isOn(r.PostFormValue("ascii")),
		SectionRules:    s.opts.SectionRules,
	})
	name := r.PostFormValue("name")
	if name == "" {
		name = resume.Name
	}
	return input{resume: resume, layout: l, lang: lang, name: sanitizeFilename(name)}, nil
}

func isOn(v string) bool { return v == "on" || v == "true" || v == "1" }

// sanitizeFilename keeps a user supplied name safe to put in a
// Content-Disposition header and in a file the browser saves: letters and
// digits of any alphabet survive, the characters a file system or a header
// parser chokes on do not.
func sanitizeFilename(s string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(s) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
		case r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' ||
			r == '"' || r == '<' || r == '>' || r == '|' || r == '\'':
			// dropped: a separator or a quote would break the header or the path
		default:
			if !unicode.IsControl(r) {
				b.WriteRune(r)
			}
		}
	}
	out := strings.Trim(b.String(), " .-")
	if out == "" {
		return "resume"
	}
	if len(out) > 60 {
		out = strings.TrimRight(out[:60], " .-")
	}
	return out
}

// wantsJSON reports whether the caller expects a JSON answer rather than an
// HTML fragment.
func wantsJSON(r *http.Request) bool {
	if r.URL.Query().Get("format") == "json" {
		return true
	}
	return strings.Contains(r.Header.Get("Accept"), "application/json")
}

// writeJSON answers with JSON and no caching: a report must never be cached.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(true)
	_ = enc.Encode(v)
}

// errorResponse is the body of a failed request.
type errorResponse struct {
	Error string `json:"error"`
}

// handleSchema returns the editor schema. The browser builds the whole form
// from it, so adding a field to the model costs one line on the server and
// nothing at all in the browser.
func (s *Server) handleSchema(w http.ResponseWriter, r *http.Request) {
	lang := s.opts.Lang
	if r.Method == http.MethodPost {
		if in, err := s.readInput(w, r); err == nil {
			lang = in.lang
		}
	}
	if l, err := model.ParseLang(r.URL.Query().Get("lang")); err == nil {
		lang = l
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"lang":       string(lang),
		"dateFormat": string(s.opts.DateFormat),
		"accent":     s.opts.Accent,
		"fontSize":   s.opts.FontSize,
		"rules":      s.opts.SectionRules,
		"identity":   IdentitySchema(),
		"contact":    ContactSchema(),
		"sections":   Sections(),
	})
}

// handleParse validates a resume and answers with the normalized JSON. It is
// the endpoint to call before rendering: it reports the first defect instead of
// silently dropping a section.
func (s *Server) handleParse(w http.ResponseWriter, r *http.Request) {
	in, err := s.readInput(w, r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	body, err := json.Marshal(in.resume)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(body)
}

// handleLint answers the report, as an HTML fragment for the editor or as JSON
// for a script.
func (s *Server) handleLint(w http.ResponseWriter, r *http.Request) {
	in, err := s.readInput(w, r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	report := lint.Run(in.resume, in.layout)
	if wantsJSON(r) {
		writeJSON(w, http.StatusOK, newReportPayload(report))
		return
	}
	s.render(w, r, "report", map[string]any{"Report": report, "Lang": in.lang})
}

// handlePreview returns the resume as an HTML fragment styled like the printed
// document. It reads the same layout stream as the PDF and the DOCX, so the
// preview cannot drift from what is downloaded.
func (s *Server) handlePreview(w http.ResponseWriter, r *http.Request) {
	in, err := s.readInput(w, r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, "preview", map[string]any{
		"Layout": in.layout,
		"Lang":   in.lang,
		"Accent": s.opts.Accent,
		"Body":   s.opts.FontSize,
	})
}

// handleText returns the plain text rendering, escaped, for the "what a parser
// reads" panel. It is a fragment and not the txt download: a raw text/plain body
// swapped into the page would be parsed as HTML.
func (s *Server) handleText(w http.ResponseWriter, r *http.Request) {
	in, err := s.readInput(w, r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if wantsJSON(r) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = io.WriteString(w, txt.Render(in.layout))
		return
	}
	s.render(w, r, "text", map[string]any{"Text": txt.Render(in.layout), "Lang": in.lang})
}

// reportPayload is the JSON shape of a lint report. The structures are flattened
// because a JSON consumer wants the score and the findings, not the formatting
// of the terminal report.
type reportPayload struct {
	Score      int            `json:"score"`
	HasErrors  bool           `json:"hasErrors"`
	Stats      lint.Stats     `json:"stats"`
	BySeverity map[string]int `json:"bySeverity"`
	Findings   []findingJSON  `json:"findings"`
}

type findingJSON struct {
	Severity string `json:"severity"`
	Rule     string `json:"rule"`
	Where    string `json:"where"`
	Message  string `json:"message"`
	Hint     string `json:"hint,omitempty"`
}

func newReportPayload(report lint.Report) reportPayload {
	out := reportPayload{
		Score:      report.Score(),
		HasErrors:  report.HasErrors(),
		Stats:      report.Stats,
		BySeverity: map[string]int{"error": 0, "warning": 0, "info": 0},
		Findings:   []findingJSON{},
	}
	for _, f := range report.Findings {
		severity := f.Severity.String()
		out.BySeverity[severity]++
		out.Findings = append(out.Findings, findingJSON{
			Severity: severity, Rule: f.Rule, Where: f.Where, Message: f.Message, Hint: f.Hint,
		})
	}
	return out
}

// handleRender writes the document itself, as a download.
func (s *Server) handleRender(w http.ResponseWriter, r *http.Request) {
	in, err := s.readInput(w, r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	body, contentType, err := s.renderBytes(r.PathValue("format"), in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	name := in.name + "." + extensionFor(r.PathValue("format"))
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", fmt.Sprint(len(body)))
	w.Header().Set("Content-Disposition", contentDisposition(name))
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(body)
}

// contentDisposition builds a download header that works with a name holding
// spaces or accents. The quoted form is the ASCII fallback every browser reads,
// the RFC 5987 form carries the real name for the ones that understand it.
func contentDisposition(name string) string {
	ascii := strings.Map(func(r rune) rune {
		if r < 32 || r > 126 || r == '"' || r == '\\' {
			return '_'
		}
		return r
	}, name)
	if ascii == "" {
		ascii = "resume"
	}
	return fmt.Sprintf("attachment; filename=%q; filename*=UTF-8''%s",
		ascii, url.PathEscape(name))
}

func extensionFor(format string) string {
	switch format {
	case "docx", "pdf", "adoc", "txt":
		return format
	default:
		return "bin"
	}
}

// renderBytes produces a document in memory with the same options as the CLI,
// so a download from the browser is the file the command line would have
// written.
func (s *Server) renderBytes(format string, in input) ([]byte, string, error) {
	resume := in.resume
	switch format {
	case "docx":
		opts := docx.Options{
			FontFamily:   "Calibri",
			FontSize:     s.opts.FontSize,
			SectionColor: s.opts.Accent,
			SectionRules: s.opts.SectionRules,
			LangTag:      langTag(in.lang),
			Author:       resume.Name,
			Title:        resume.Name,
			Keywords:     keywordString(resume),
		}
		var buf bytes.Buffer
		if err := docx.Render(&buf, docx.FromLayout(in.layout), opts); err != nil {
			return nil, "", err
		}
		return buf.Bytes(), docxContentType, nil
	case "pdf":
		rgb, err := parseAccent(s.opts.Accent)
		if err != nil {
			return nil, "", err
		}
		opts := pdf.Options{
			FontFamily:   "Arial",
			FontDir:      s.opts.FontDir,
			FontSize:     s.opts.FontSize,
			SectionColor: rgb,
			SectionRules: s.opts.SectionRules,
			DimColor:     pdf.RGB{R: 0x40, G: 0x40, B: 0x40},
			Author:       resume.Name,
			Title:        resume.Name,
		}
		body, err := pdf.Bytes(in.layout, opts)
		if err != nil {
			return nil, "", err
		}
		return body, "application/pdf", nil
	case "adoc":
		opts := adoc.Options{Lang: in.layout.OptsLang(), SectionRules: s.opts.SectionRules}
		return []byte(adoc.Render(in.layout, opts)), "text/plain; charset=utf-8", nil
	case "txt":
		return []byte(txt.Render(in.layout)), "text/plain; charset=utf-8", nil
	default:
		return nil, "", fmt.Errorf("unknown format %q, want docx, pdf, adoc or txt", format)
	}
}

const docxContentType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"

// fail answers an error, as an HTML fragment or as JSON.
func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	if wantsJSON(r) {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	s.render(w, r, "report", map[string]any{"Error": err.Error(), "Lang": s.opts.Lang})
}

// render executes one template of the set by name. ParseFS names a template
// after its file, so "preview" is stored as "preview.html".
func (s *Server) render(w http.ResponseWriter, _ *http.Request, name string, data map[string]any) {
	var buf bytes.Buffer
	if err := s.tmpl.ExecuteTemplate(&buf, name+".html", data); err != nil {
		http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(buf.Bytes())
}

// handleStyle serves the accent and the body size as a stylesheet. The policy
// forbids inline styles, and the editor must not have to give up that
// protection to be coloured: the value goes through the same parser as the
// flag, so what reaches the sheet is always a colour and never an injection.
func (s *Server) handleStyle(w http.ResponseWriter, r *http.Request) {
	accent := s.opts.Accent
	if q := r.URL.Query().Get("accent"); q != "" {
		rgb, err := parseAccent(q)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		accent = fmt.Sprintf("%02X%02X%02X", rgb.R, rgb.G, rgb.B)
	}
	size := s.opts.FontSize
	if q := r.URL.Query().Get("size"); q != "" {
		if v, err := strconv.ParseFloat(q, 64); err == nil && v >= 6 && v <= 24 {
			size = v
		}
	}
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = fmt.Fprintf(w, ":root{--accent:#%s;--body:%gpt}\n", accent, size)
}

// handleDemo serves the resume a fresh editor starts from. It is a read only
// copy of a file the operator chose: no session, no disk write, and a 404 when
// none was configured, which the editor reads as "start empty".
func (s *Server) handleDemo(w http.ResponseWriter, r *http.Request) {
	if s.opts.DemoJSON == "" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, s.opts.DemoJSON)
}

// handleHome serves the landing page. It reads no resume and calls no renderer:
// a visitor who only wants to know what the tool does never touches their data.
//
// The landing page is French whatever the editor language is. Announcing it as
// English would make a screen reader pronounce French text with English rules,
// and would be a plain lie in the document language.
func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "home", map[string]any{
		"Lang":   model.LangFR,
		"Accent": s.opts.Accent,
		"Body":   s.opts.FontSize,
	})
}

// handleEditor serves the editor shell.
func (s *Server) handleEditor(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "page", map[string]any{
		"Lang":     s.opts.Lang,
		"DateFmt":  string(s.opts.DateFormat),
		"Accent":   s.opts.Accent,
		"Body":     s.opts.FontSize,
		"FontSize": s.opts.FontSize,
	})
}

// langTag is the w:lang value of the document, the language a proofreader
// should see.
func langTag(l model.Lang) string {
	if l == model.LangEN {
		return "en-US"
	}
	return "fr-FR"
}

// parseAccent converts an RRGGBB string to the PDF color.
func parseAccent(s string) (pdf.RGB, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) != 6 {
		return pdf.RGB{}, fmt.Errorf("invalid accent color %q, want RRGGBB", s)
	}
	var v uint64
	for _, c := range s {
		var d uint64
		switch {
		case c >= '0' && c <= '9':
			d = uint64(c - '0')
		case c >= 'a' && c <= 'f':
			d = uint64(c-'a') + 10
		case c >= 'A' && c <= 'F':
			d = uint64(c-'A') + 10
		default:
			return pdf.RGB{}, fmt.Errorf("invalid accent color %q, want RRGGBB", s)
		}
		v = v*16 + d
	}
	return pdf.RGB{R: int(v >> 16), G: int(v>>8) & 0xFF, B: int(v & 0xFF)}, nil
}

// keywordString builds the keywords of the docx properties: the headline and the
// skills, so the file is searchable in a document management system before
// anybody opens it.
func keywordString(r *model.Resume) string {
	var parts []string
	if r.Headline != "" {
		parts = append(parts, r.Headline)
	}
	for _, group := range r.Skills {
		parts = append(parts, group.Items...)
	}
	return strings.Join(parts, ", ")
}
