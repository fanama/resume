package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/fanama/resume/atscv/internal/layout"
	"github.com/fanama/resume/atscv/internal/model"
	"github.com/fanama/resume/atscv/internal/render/pdf"
)

// commonFlags are the options shared by every command.
type commonFlags struct {
	input        string
	output       string
	lang         string
	format       string
	dates        string
	ascii        bool
	rules        bool
	font         string
	fontFile     string
	boldFontFile string
	size         float64
	accent       string
	dir          string
	outName      string
	quiet        bool
}

func (c *commonFlags) register(fs *flag.FlagSet) {
	fs.StringVar(&c.input, "i", "", "path to the resume JSON file")
	fs.StringVar(&c.input, "in", "", "alias of -i")
	fs.StringVar(&c.output, "o", "", "output directory, or output file with -f of one format")
	fs.StringVar(&c.lang, "lang", "", "language of the section titles: fr or en (default: the data file)")
	fs.StringVar(&c.format, "f", "docx,pdf,adoc", "formats to write: docx,pdf,adoc,txt,tex")
	fs.StringVar(&c.dates, "dates", "numeric", "date format: numeric, month or year")
	fs.BoolVar(&c.ascii, "ascii", false, "remove the diacritics so a keyword match never misses")
	fs.BoolVar(&c.rules, "rules", true, "hairlines under the section headings and under the header block")
	fs.StringVar(&c.font, "font", "", "typeface: Calibri or Arial for the docx, Helvetica for the pdf")
	fs.Float64Var(&c.size, "size", 0, "body size in points")
	fs.StringVar(&c.accent, "accent", "", "accent color of the headings and labels as RRGGBB (default "+defaultAccentHex+")")
	fs.StringVar(&c.dir, "font-dir", "", "extra directory searched for TrueType fonts, for the pdf")
	fs.StringVar(&c.fontFile, "font-file", "", "explicit TrueType file for the pdf")
	fs.StringVar(&c.boldFontFile, "bold-font-file", "", "explicit bold TrueType file for the pdf")
	fs.StringVar(&c.outName, "name", "", "base name of the generated files")
	fs.BoolVar(&c.quiet, "q", false, "print nothing on success")
}

func (c *commonFlags) parse(fs *flag.FlagSet, args []string, stderr io.Writer) error {
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	if c.input == "" {
		return errors.New("a resume file is required: atscv -i resume.json")
	}
	return nil
}

// pipeline loads the resume and builds the layout every renderer shares.
func (c *commonFlags) pipeline() (*model.Resume, *layout.Layout, error) {
	resume, err := model.Load(c.input)
	if err != nil {
		return nil, nil, err
	}
	lang := resume.Lang
	if c.lang != "" {
		lang, err = model.ParseLang(c.lang)
		if err != nil {
			return nil, nil, err
		}
	}
	dateFormat, err := parseDateFormat(c.dates)
	if err != nil {
		return nil, nil, err
	}
	opts := layout.Options{
		Lang:            lang,
		DateFormat:      dateFormat,
		StripDiacritics: c.ascii,
		SectionRules:    c.rules,
	}
	return resume, layout.New(resume, opts), nil
}

func parseDateFormat(s string) (layout.DateFormat, error) {
	switch layout.DateFormat(strings.ToLower(strings.TrimSpace(s))) {
	case "", layout.DateNumeric:
		return layout.DateNumeric, nil
	case layout.DateMonthName:
		return layout.DateMonthName, nil
	case layout.DateYear:
		return layout.DateYear, nil
	default:
		return "", fmt.Errorf("unknown date format %q (want numeric, month or year)", s)
	}
}

// parseFormats reads the comma separated -f value and rejects an unknown name.
func parseFormats(s string) ([]string, error) {
	known := map[string]bool{"docx": true, "pdf": true, "adoc": true, "txt": true, "tex": true}
	var out []string
	for _, f := range strings.Split(s, ",") {
		f = strings.ToLower(strings.TrimSpace(f))
		if f == "" {
			continue
		}
		if !known[f] {
			return nil, fmt.Errorf("unknown format %q (want docx, pdf, adoc, txt or tex)", f)
		}
		out = append(out, f)
	}
	if len(out) == 0 {
		return nil, errors.New("no output format selected")
	}
	return out, nil
}

// parseAccent reads an RRGGBB color. Without one, the headings keep a dark navy
// accent: a color carries no meaning for a parser, and it is what gives the
// document a hierarchy.
func parseAccent(s string) (string, pdf.RGB, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if s == "" {
		return defaultAccentHex, pdf.RGB{R: 0x1F, G: 0x38, B: 0x64}, nil
	}
	if len(s) != 6 {
		return "", pdf.RGB{}, fmt.Errorf("invalid color %q, want RRGGBB", s)
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return "", pdf.RGB{}, fmt.Errorf("invalid color %q, want RRGGBB", s)
	}
	upper := strings.ToUpper(s)
	return upper, pdf.RGB{R: int(v >> 16), G: int(v>>8) & 0xFF, B: int(v & 0xFF)}, nil
}

// defaultAccentHex is the navy used for the headings, the headline and the field
// labels when -accent is not given.
const defaultAccentHex = "1F3864"

func pick(value, fallback float64) float64 {
	if value <= 0 {
		return fallback
	}
	return value
}

func langTag(l *layout.Layout) string {
	if l.OptsLang() == model.LangEN {
		return "en-US"
	}
	return "fr-FR"
}

func keywordString(r *model.Resume) string {
	return strings.Join(collectKeywords(r), ", ")
}

// collectKeywords gathers the terms an applicant tracking system matches on:
// the title, the technologies and the skill items. The job offer's own wording
// should be copied into those fields.
func collectKeywords(r *model.Resume) []string {
	seen := map[string]bool{}
	var out []string
	add := func(values ...string) {
		for _, v := range values {
			for _, word := range strings.FieldsFunc(v, func(r rune) bool {
				return !(r == '-' || r == '+' || r == '#' || r == '.' || r == '/' || r == '_' ||
					(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'))
			}) {
				if len(word) < 2 {
					continue
				}
				key := strings.ToLower(word)
				if seen[key] {
					continue
				}
				seen[key] = true
				out = append(out, word)
			}
		}
	}
	add(r.Headline)
	for _, e := range r.Experience {
		add(e.Title)
		add(e.Stack...)
	}
	for _, g := range r.Skills {
		add(g.Category)
		add(g.Items...)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return strings.ToLower(out[i]) < strings.ToLower(out[j])
	})
	return out
}

// baseName is the stem of the generated files: -name, or the input file name.
func (c *commonFlags) baseName() string {
	if c.outName != "" {
		return c.outName
	}
	return strings.TrimSuffix(filepath.Base(c.input), filepath.Ext(c.input))
}

func hasExt(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".docx", ".pdf", ".adoc", ".txt", ".tex", ".asciidoc":
		return true
	}
	return false
}
