// Command atscv generates an Applicant Tracking System friendly resume from a
// JSON file, and checks the result against the rules an ATS applies when it
// extracts a candidate profile.
//
// Usage:
//
//	atscv build  -i resume.json -o out/            # docx, pdf and adoc
//	atscv lint   -i resume.json                    # ATS report
//	atscv text   -i resume.json                    # plain text preview
//	atscv check  -i resume.json                    # data validation only
//	atscv serve                                       # editor on the loopback
//	atscv healthcheck                                 # container probe
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fanama/resume/atscv/internal/layout"
	"github.com/fanama/resume/atscv/internal/lint"
	"github.com/fanama/resume/atscv/internal/model"
	"github.com/fanama/resume/atscv/internal/render/adoc"
	"github.com/fanama/resume/atscv/internal/render/docx"
	"github.com/fanama/resume/atscv/internal/render/pdf"
	"github.com/fanama/resume/atscv/internal/render/txt"
	"github.com/fanama/resume/atscv/internal/web"
)

// version is overridable at build time with -ldflags "-X main.version=...".
var version = "1.0.0"

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "atscv:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		usage(stderr)
		return errors.New("a command is required")
	}
	switch args[0] {
	case "build":
		return runBuild(args[1:], stdout, stderr)
	case "lint":
		return runLint(args[1:], stdout, stderr)
	case "text":
		return runText(args[1:], stdout, stderr)
	case "check":
		return runCheck(args[1:], stdout, stderr)
	case "serve":
		return runServe(args[1:], stdout, stderr)
	case "healthcheck":
		return runHealthcheck(args[1:], stdout)
	case "version", "-v", "--version":
		fmt.Fprintln(stdout, "atscv", version)
		return nil
	case "help", "-h", "--help":
		usage(stdout)
		return nil
	default:
		usage(stderr)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func usage(w io.Writer) {
	fmt.Fprint(w, `atscv - ATS friendly resume generator

Commands:
  build        render the resume to .docx, .pdf, .adoc and .txt
  lint         report what an ATS would complain about, with a readability score
  text         print the plain text an ATS extracts, for debugging
  check        validate the JSON data and the date ranges
  serve        start the web editor and the JSON API, on the loopback interface
  healthcheck  probe a running server, for a container health check

Run "atscv <command> -h" for the flags of a command.
`)
}

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
	fs.StringVar(&c.format, "f", "docx,pdf,adoc", "formats to write: docx,pdf,adoc,txt")
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

func runBuild(args []string, stdout, stderr io.Writer) error {
	var c commonFlags
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	c.register(fs)
	if err := c.parse(fs, args, stderr); err != nil {
		return err
	}
	resume, l, err := c.pipeline()
	if err != nil {
		return err
	}
	formats, err := parseFormats(c.format)
	if err != nil {
		return err
	}
	accentHex, accentRGB, err := parseAccent(c.accent)
	if err != nil {
		return err
	}

	docxFont := c.font
	if docxFont == "" {
		docxFont = "Calibri"
	}
	docxOpts := docx.Options{
		FontFamily:   docxFont,
		FontSize:     pick(c.size, 10.5),
		SectionColor: accentHex,
		SectionRules: c.rules,
		LangTag:      langTag(l),
		Author:       resume.Name,
		Title:        resume.Name,
		Keywords:     keywordString(resume),
	}
	pdfOpts := c.pdfOptions(resume, accentRGB)
	adocOpts := adoc.Options{Lang: l.OptsLang(), SectionRules: c.rules}
	header := fmt.Sprintf("Generated by atscv from %s, single column, no table.", filepath.Base(c.input))

	base := c.outName
	if base == "" {
		base = strings.TrimSuffix(filepath.Base(c.input), filepath.Ext(c.input))
	}
	dir := c.output
	explicit := dir != "" && hasExt(dir) && len(formats) == 1
	if dir != "" && !explicit {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
	}

	written := make([]string, 0, len(formats))
	for _, format := range formats {
		target := c.target(dir, base, format, explicit)
		if err := writeFormat(format, target, l, docxOpts, pdfOpts, adocOpts, header); err != nil {
			return err
		}
		written = append(written, target)
	}
	if c.quiet {
		return nil
	}
	for _, path := range written {
		fmt.Fprintln(stdout, "wrote", path)
	}
	// The page count is measured on the PDF that was just produced, and only
	// said out loud when the document spills: a resume that needs a second page
	// is a content decision, and a silent extra page is how it goes unnoticed.
	if slices.Contains(formats, "pdf") {
		if rep, err := pdf.Measure(l, pdfOpts); err == nil && rep.Pages > 1 {
			fmt.Fprintf(stderr, "note: the PDF runs to %d pages, the last one %.0f%% full; "+
				"the breaks are clean, but fitting on one page means cutting about %.0f mm of content\n",
				rep.Pages, rep.LastPageFill, rep.LastPageFill/100*(297-2*14))
		}
	}
	return nil
}

func (c *commonFlags) target(dir, base, format string, explicit bool) string {
	ext := "." + format
	if format == "adoc" {
		ext = ".adoc"
	}
	if explicit {
		return dir
	}
	if dir == "" {
		return base + ext
	}
	return filepath.Join(dir, base+ext)
}

func writeFormat(format, target string, l *layout.Layout,
	dOpts docx.Options, pOpts pdf.Options, aOpts adoc.Options, header string) error {
	var err error
	switch format {
	case "docx":
		err = writeFile(target, func(w io.Writer) error {
			return docx.Render(w, docx.FromLayout(l), dOpts)
		})
	case "pdf":
		err = writeFile(target, func(w io.Writer) error { return pdf.Render(w, l, pOpts) })
	case "adoc":
		err = writeFile(target, func(w io.Writer) error {
			_, werr := io.WriteString(w, adoc.Render(l, aOpts))
			return werr
		})
	case "txt":
		err = writeFile(target, func(w io.Writer) error {
			_, werr := io.WriteString(w, txt.Render(l))
			return werr
		})
	default:
		err = fmt.Errorf("unknown format %q", format)
	}
	if err != nil {
		return fmt.Errorf("%s: %w", format, err)
	}
	return nil
}

func writeFile(path string, fn func(io.Writer) error) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	if err := fn(f); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func hasExt(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".docx", ".pdf", ".adoc", ".txt", ".asciidoc":
		return true
	}
	return false
}

func parseFormats(s string) ([]string, error) {
	known := map[string]bool{"docx": true, "pdf": true, "adoc": true, "txt": true}
	var out []string
	for _, f := range strings.Split(s, ",") {
		f = strings.ToLower(strings.TrimSpace(f))
		if f == "" {
			continue
		}
		if !known[f] {
			return nil, fmt.Errorf("unknown format %q (want docx, pdf, adoc or txt)", f)
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

func runText(args []string, stdout, stderr io.Writer) error {
	var c commonFlags
	fs := flag.NewFlagSet("text", flag.ContinueOnError)
	c.register(fs)
	if err := c.parse(fs, args, stderr); err != nil {
		return err
	}
	_, l, err := c.pipeline()
	if err != nil {
		return err
	}
	fmt.Fprint(stdout, txt.Render(l))
	return nil
}

// pdfOptions builds the PDF options from the flags, so that the page count
// printed by check and the one produced by build cannot drift apart.
func (c *commonFlags) pdfOptions(resume *model.Resume, accent pdf.RGB) pdf.Options {
	font := c.font
	if font == "" {
		font = "Arial"
	}
	return pdf.Options{
		FontFamily:   font,
		FontDir:      c.dir,
		FontFile:     c.fontFile,
		BoldFontFile: c.boldFontFile,
		FontSize:     pick(c.size, 10.5),
		SectionColor: accent,
		SectionRules: c.rules,
		DimColor:     pdf.RGB{R: 64, G: 64, B: 64},
		Author:       resume.Name,
		Title:        resume.Name,
	}
}

// pageReport measures the document with the real renderer, then turns the
// number into something a writer can act on. A resume that spills onto a second
// page is not a formatting problem: it is a hundred millimetres of content that
// has to go, and the message says so instead of "about 2 pages".
func pageReport(l *layout.Layout, opts pdf.Options) (pdf.Report, string) {
	rep, err := pdf.Measure(l, opts)
	if err != nil {
		return rep, fmt.Sprintf("pages: unknown (%v)", err)
	}
	if rep.Pages <= 1 {
		return rep, fmt.Sprintf("%d page", rep.Pages)
	}
	return rep, fmt.Sprintf("%d pages, the last one %.0f%% full: it would take %.0f mm of content to fit on one",
		rep.Pages, rep.LastPageFill, rep.LastPageFill/100*(297-2*14))
}

func runCheck(args []string, stdout, stderr io.Writer) error {
	var c commonFlags
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	c.register(fs)
	if err := c.parse(fs, args, stderr); err != nil {
		return err
	}
	resume, l, err := c.pipeline()
	if err != nil {
		return err
	}
	report := lint.Run(resume, l)
	var blocking []lint.Finding
	for _, f := range report.SortedFindings() {
		if f.Severity == lint.Error {
			blocking = append(blocking, f)
		}
	}
	if len(blocking) > 0 {
		for _, f := range blocking {
			fmt.Fprintln(stderr, f.String())
		}
		return fmt.Errorf("%d blocking issue(s) found", len(blocking))
	}
	if !c.quiet {
		stats := report.Stats
		_, accent, err := parseAccent(c.accent)
		if err != nil {
			return err
		}
		_, pages := pageReport(l, c.pdfOptions(resume, accent))
		fmt.Fprintf(stdout, "ok: %d words, %d entries, %d skills, %s\n",
			stats.Words, stats.Entries, stats.Skills, pages)
	}
	return nil
}

func runLint(args []string, stdout, stderr io.Writer) error {
	var c commonFlags
	fs := flag.NewFlagSet("lint", flag.ContinueOnError)
	c.register(fs)
	if err := c.parse(fs, args, stderr); err != nil {
		return err
	}
	resume, l, err := c.pipeline()
	if err != nil {
		return err
	}
	report := lint.Run(resume, l)
	printReport(stdout, report)
	if report.HasErrors() {
		return errors.New("the resume has blocking issues")
	}
	return nil
}

func printReport(w io.Writer, report lint.Report) {
	findings := report.SortedFindings()
	if len(findings) == 0 {
		fmt.Fprintln(w, "no finding, the resume is ready to send")
	}
	for _, f := range findings {
		fmt.Fprintln(w, f.String())
	}
	s := report.Stats
	fmt.Fprintln(w)
	fmt.Fprintf(w, "readability %d/100 | ~%d page(s) | %d words | %d entries | %d achievement lines, %d with a figure | %d skills\n",
		s.ATSReadability, s.Pages, s.Words, s.Entries, s.Bullets, s.Metrics, s.Skills)
}

// runServe starts the local editor. The flags mirror the ones of "build" so a
// document downloaded from the browser is the one the command line would have
// written; the address is forced to the loopback interface by the web package,
// which has no authentication.
// runHealthcheck asks the running server whether it is alive. A container image
// has no shell and no curl, so the probe has to be the binary itself. It reads
// the address the server was told to use, which is how it finds the port when
// the platform maps another one onto it.
func runHealthcheck(args []string, stdout io.Writer) error {
	if len(args) > 0 {
		return fmt.Errorf("healthcheck takes no argument, got %q", args[0])
	}
	addr := serveAddr()
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid address %q: %w", addr, err)
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		// A routable address would be a request to the internet from inside the
		// container; the probe stays on the loopback.
		host = "127.0.0.1"
	}
	url := "http://" + net.JoinHostPort(host, port(addr)) + "/healthz"
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("healthcheck %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("healthcheck %s: status %d", url, resp.StatusCode)
	}
	fmt.Fprintln(stdout, "ok")
	return nil
}

func port(addr string) string {
	if _, p, err := net.SplitHostPort(addr); err == nil {
		return p
	}
	return "8080"
}

// serveAddr is the address the server listens on. The environment wins over the
// flag default, so a platform that injects a port does not need to change the
// command line.
func serveAddr() string {
	if v := os.Getenv("ATSCV_ADDR"); v != "" {
		return v
	}
	// PORT is the convention every hosted platform follows: it is set for us,
	// and binding something else means the proxy forwards the traffic into the
	// void. It only decides the port though. Whether to accept that traffic at
	// all stays an explicit decision, so PORT alone does not make the server
	// public: without -public the server still refuses to start.
	if p := os.Getenv("PORT"); p != "" {
		return "0.0.0.0:" + p
	}
	return "127.0.0.1:8080"
}

// envOrFloat is envOr for the flags that carry a number.
func envOrFloat(env string, def float64) float64 {
	if v := os.Getenv(env); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}

func envOr(env, def string) string {
	if v := os.Getenv(env); v != "" {
		return v
	}
	return def
}

func runServe(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	addr := fs.String("addr", serveAddr(), "listen address, must be on the loopback interface unless -public is given")
	public := fs.Bool("public", os.Getenv("ATSCV_PUBLIC") != "", "listen on a routable address: for a container behind a proxy, and the instance then has no authentication")
	lang := fs.String("lang", envOr("ATSCV_LANG", "fr"), "default editor language: fr or en")
	dates := fs.String("dates", envOr("ATSCV_DATES", "numeric"), "date format in the preview and the exports: numeric, month or year")
	accent := fs.String("accent", envOr("ATSCV_ACCENT", defaultAccentHex), "accent color of the documents as RRGGBB")
	fontDir := fs.String("font-dir", os.Getenv("ATSCV_FONT_DIR"), "extra directory searched for TrueType fonts, for the pdf")
	size := fs.Float64("size", envOrFloat("ATSCV_SIZE", 10.5), "body size in points")
	rules := fs.Bool("rules", true, "hairlines under the section headings")
	demo := fs.String("demo", os.Getenv("ATSCV_DEMO"), "resume JSON proposed when the editor opens with no session yet")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	editorLang, err := model.ParseLang(*lang)
	if err != nil {
		return err
	}
	dateFormat, err := parseDateFormat(*dates)
	if err != nil {
		return err
	}
	accentHex, _, err := parseAccent(*accent)
	if err != nil {
		return err
	}
	var demoJSON string
	if *demo != "" {
		raw, err := os.ReadFile(*demo)
		if err != nil {
			return fmt.Errorf("demo %s: %w", *demo, err)
		}
		// A demo that does not load is worse than none: the editor would offer
		// a starting point that turns out to be a broken file.
		if _, err := model.Parse(raw); err != nil {
			return fmt.Errorf("demo %s: %w", *demo, err)
		}
		demoJSON = string(raw)
	}

	srv, err := web.New(web.Options{
		Addr:         *addr,
		Public:       *public,
		DemoJSON:     demoJSON,
		Lang:         editorLang,
		DateFormat:   dateFormat,
		Accent:       accentHex,
		SectionRules: *rules,
		FontSize:     *size,
		FontDir:      *fontDir,
	})
	if err != nil {
		return err
	}
	return srv.ListenAndServe()
}
