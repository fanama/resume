package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"unicode"

	"github.com/fanama/resume/atscv/internal/layout"
	"github.com/fanama/resume/atscv/internal/model"
)

// input is a decoded request: the resume plus the layout every renderer shares.
type input struct {
	resume *model.Resume
	layout *layout.Layout
	lang   model.Lang
	name   string
	// style carries the look the request asked for: an empty field keeps
	// the server's option, a filled one names its own accent, font or size.
	// They ride the same request as the data, so a scripted curl call and the
	// editor ask for the same thing, and no style lives on the server.
	accent   string
	font     string
	fontSize float64
}

// readInput decodes the resume from the request. The payload is either a form
// field named "data" or a raw JSON body, so the same endpoints serve the editor
// and a curl call.
func (s *Server) readInput(w http.ResponseWriter, r *http.Request) (input, error) {
	if r.Method != http.MethodPost {
		return input{}, fmt.Errorf("use POST, not %s", r.Method)
	}
	raw, err := readPayload(w, r)
	if err != nil {
		return input{}, err
	}
	resume, err := model.Parse(raw)
	if err != nil {
		return input{}, err
	}
	lang := resume.Lang
	if lang == "" {
		lang = s.opts.Lang
	}
	// The look overrides the request names. Dates change the layout itself, so
	// they are read before it is built; the rest ride beside the layout.
	dates, err := s.styleDates(r)
	if err != nil {
		return input{}, err
	}
	accent, err := styleAccent(r)
	if err != nil {
		return input{}, err
	}
	font, err := styleFont(r)
	if err != nil {
		return input{}, err
	}
	size, err := styleSize(r)
	if err != nil {
		return input{}, err
	}
	return s.buildInput(r, resume, lang, dates, accent, font, size), nil
}

// buildInput builds the layout and the download name of a decoded request.
func (s *Server) buildInput(r *http.Request, resume *model.Resume, lang model.Lang, dates layout.DateFormat, accent, font string, size float64) input {
	l := layout.New(resume, layout.Options{
		Lang:            lang,
		DateFormat:      dates,
		StripDiacritics: isOn(r.PostFormValue("ascii")),
		SectionRules:    s.opts.SectionRules,
	})
	name := r.PostFormValue("name")
	if name == "" {
		name = resume.Name
	}
	return input{
		resume:   resume,
		layout:   l,
		lang:     lang,
		name:     sanitizeFilename(name),
		accent:   accent,
		font:     font,
		fontSize: size,
	}
}

// readPayload reads the resume bytes from a JSON body or from the "data" form
// field, refusing an empty payload.
func readPayload(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	var raw []byte
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			return nil, fmt.Errorf("read body: %w", err)
		}
		raw = body
	} else {
		if err := r.ParseForm(); err != nil {
			return nil, fmt.Errorf("read form: %w", err)
		}
		raw = []byte(r.PostFormValue("data"))
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, fmt.Errorf(`no resume in the request: send the JSON in the "data" form field or as a JSON body`)
	}
	return raw, nil
}

// styleDates reads the date format the request asks for, keeping the server's
// own when it names none.
func (s *Server) styleDates(r *http.Request) (layout.DateFormat, error) {
	dates := s.opts.DateFormat
	if q := styleParam(r, "dates"); q != "" {
		df, ok := documentDates[q]
		if !ok {
			return "", fmt.Errorf("unknown date format %q, want numeric, month or year", q)
		}
		dates = df
	}
	return dates, nil
}

// styleFont reads the typeface the request asks for.
func styleFont(r *http.Request) (string, error) {
	q := styleParam(r, "font")
	if q == "" {
		return "", nil
	}
	family, ok := documentFonts[q]
	if !ok {
		return "", fmt.Errorf("unknown font %q, want calibri, arial or times", q)
	}
	return family, nil
}

// styleSize reads the body size the request asks for.
func styleSize(r *http.Request) (float64, error) {
	q := styleParam(r, "size")
	if q == "" {
		return 0, nil
	}
	v, err := strconv.ParseFloat(q, 64)
	if err != nil || v < 6 || v > 24 {
		return 0, fmt.Errorf("invalid font size %q, want a number between 6 and 24", q)
	}
	return v, nil
}

func isOn(v string) bool { return v == "on" || v == "true" || v == "1" }

// documentFonts are the typefaces the editor may ask for. The list is the set
// an ATS reads without complaint and both renderers can name: one option value
// speaks to the PDF and to the DOCX with the same word.
var documentFonts = map[string]string{
	"calibri": "Calibri",
	"arial":   "Arial",
	"times":   "Times New Roman",
}

// documentDates are the period formats the layout knows, spelled as the CLI
// does: the editor and the command line offer the same list.
var documentDates = map[string]layout.DateFormat{
	"numeric": layout.DateNumeric,
	"month":   layout.DateMonthName,
	"year":    layout.DateYear,
}

// styleParam reads one document-style override the request names. The editor's
// form sends it in the body, a scripted call puts it in the query — FormValue
// answers both. Empty means "the server's own": the look is owned only where
// somebody typed one.
func styleParam(r *http.Request, key string) string {
	return strings.TrimSpace(r.FormValue(key))
}

// styleAccent validates the accent colour the request names and hands it back
// normalized, so a #rrggbb typed in the browser reaches the renderers the way
// an operator's flag would.
func styleAccent(r *http.Request) (string, error) {
	q := styleParam(r, "accent")
	if q == "" {
		return "", nil
	}
	rgb, err := parseAccent(q)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%02X%02X%02X", rgb.R, rgb.G, rgb.B), nil
}

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

// failDownload answers the error of a download. Unlike the HTMX panels, a
// render is never a fragment: the caller is a fetch() that guards on res.ok, or
// a curl -o file, and both have to be able to tell a document from an error. A
// 200 here saved an HTML page under the name of the candidate's CV.
func (s *Server) failDownload(w http.ResponseWriter, err error) {
	writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
}

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
