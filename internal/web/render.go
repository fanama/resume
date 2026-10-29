package web

import (
	"bytes"
	"fmt"
	"net/url"
	"strings"

	"github.com/fanama/resume/atscv/internal/model"
	"github.com/fanama/resume/atscv/internal/render/adoc"
	"github.com/fanama/resume/atscv/internal/render/docx"
	"github.com/fanama/resume/atscv/internal/render/pdf"
	"github.com/fanama/resume/atscv/internal/render/tex"
	"github.com/fanama/resume/atscv/internal/render/txt"
)

// texOptions builds the LaTeX options of both the panel and the download, so
// what the editor shows and what it saves are the same file.
func (s *Server) texOptions(in input) tex.Options {
	return tex.Options{
		Lang:         in.layout.OptsLang(),
		SectionRules: s.opts.SectionRules,
		Accent:       or(in.accent, s.opts.Accent),
		FontSize:     sizeOr(in.fontSize, s.opts.FontSize),
	}
}

// renderBytes produces a document in memory with the same options as the CLI,
// so a download from the browser is the file the command line would have
// written.
func (s *Server) renderBytes(format string, in input) ([]byte, string, error) {
	switch format {
	case "docx":
		return s.renderDocx(in)
	case "pdf":
		return s.renderPDF(in)
	case "adoc":
		opts := adoc.Options{Lang: in.layout.OptsLang(), SectionRules: s.opts.SectionRules}
		return []byte(adoc.Render(in.layout, opts)), "text/plain; charset=utf-8", nil
	case "txt":
		return []byte(txt.Render(in.layout)), "text/plain; charset=utf-8", nil
	case "tex":
		// The LaTeX source is written in pure Go: the server never runs an
		// external tool, and the file it answers with is the one the CLI
		// would have written.
		return []byte(tex.Render(in.layout, s.texOptions(in))), "text/x-tex; charset=utf-8", nil
	default:
		return nil, "", fmt.Errorf("unknown format %q, want docx, pdf, adoc, txt or tex", format)
	}
}

// renderDocx builds the Word document with the options of the request.
func (s *Server) renderDocx(in input) ([]byte, string, error) {
	opts := docx.Options{
		FontFamily:   or(in.font, "Calibri"),
		FontSize:     sizeOr(in.fontSize, s.opts.FontSize),
		SectionColor: or(in.accent, s.opts.Accent),
		SectionRules: s.opts.SectionRules,
		LangTag:      langTag(in.lang),
		Author:       in.resume.Name,
		Title:        in.resume.Name,
		Keywords:     keywordString(in.resume),
	}
	var buf bytes.Buffer
	if err := docx.Render(&buf, docx.FromLayout(in.layout), opts); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), docxContentType, nil
}

// renderPDF builds the PDF with the options of the request.
func (s *Server) renderPDF(in input) ([]byte, string, error) {
	rgb, err := parseAccent(or(in.accent, s.opts.Accent))
	if err != nil {
		return nil, "", err
	}
	opts := pdf.Options{
		FontFamily:   or(in.font, "Arial"),
		FontDir:      s.opts.FontDir,
		FontSize:     sizeOr(in.fontSize, s.opts.FontSize),
		SectionColor: rgb,
		SectionRules: s.opts.SectionRules,
		DimColor:     pdf.RGB{R: 0x40, G: 0x40, B: 0x40},
		Author:       in.resume.Name,
		Title:        in.resume.Name,
	}
	body, err := pdf.Bytes(in.layout, opts)
	if err != nil {
		return nil, "", err
	}
	return body, "application/pdf", nil
}

const docxContentType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"

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
	case "docx", "pdf", "adoc", "txt", "tex":
		return format
	default:
		return "bin"
	}
}

// or keeps the server's option wherever the request left the choice empty: an
// override speaks only where it named something.
func or(requested, server string) string {
	if requested != "" {
		return requested
	}
	return server
}

// sizeOr is or for the body size, where the "unset" value is the zero.
func sizeOr(requested, server float64) float64 {
	if requested != 0 {
		return requested
	}
	return server
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
