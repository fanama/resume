package web

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/fanama/resume/atscv/internal/lint"
	"github.com/fanama/resume/atscv/internal/model"
	"github.com/fanama/resume/atscv/internal/render/tex"
	"github.com/fanama/resume/atscv/internal/render/txt"
)

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
		// The grammar the analysis panel constrains its rewrites with: the
		// same resume shape the parser enforces, published as a JSON Schema,
		// so the browser can only emit documents the server will accept.
		"resumeSchema": ResumeJSONSchema(),
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

// handleTex returns the LaTeX source of the resume, for the "LaTeX" panel of
// the editor. Like the text panel it is a fragment and not the download: the
// source is read while typing, and the bytes it shows are the ones
// /api/render/tex saves, built from the same options so the two cannot drift.
func (s *Server) handleTex(w http.ResponseWriter, r *http.Request) {
	in, err := s.readInput(w, r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	source := tex.Render(in.layout, s.texOptions(in))
	if wantsJSON(r) {
		w.Header().Set("Content-Type", "text/x-tex; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = io.WriteString(w, source)
		return
	}
	s.render(w, r, "tex", map[string]any{"Source": source})
}

// handleRender writes the document itself, as a download.
func (s *Server) handleRender(w http.ResponseWriter, r *http.Request) {
	in, err := s.readInput(w, r)
	if err != nil {
		s.failDownload(w, err)
		return
	}
	body, contentType, err := s.renderBytes(r.PathValue("format"), in)
	if err != nil {
		// A download is never an htmx fragment, so it never gets the 200 that
		// lets a fragment be swapped: the caller is a fetch() that checks
		// res.ok, or a curl -o file, and both have to be able to tell a
		// document from an error. Answering 200 with an HTML fragment made a
		// failed render download an HTML page named after the candidate.
		s.failDownload(w, err)
		return
	}
	name := in.name + "." + extensionFor(r.PathValue("format"))
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", fmt.Sprint(len(body)))
	w.Header().Set("Content-Disposition", contentDisposition(name))
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(body)
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
