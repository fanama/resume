package main

import (
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/fanama/resume/atscv/internal/layout"
	"github.com/fanama/resume/atscv/internal/lint"
	"github.com/fanama/resume/atscv/internal/model"
	"github.com/fanama/resume/atscv/internal/render/pdf"
	"github.com/fanama/resume/atscv/internal/render/txt"
)

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
