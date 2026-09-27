// Package adoc renders a layout as an AsciiDoc document.
//
// The AsciiDoc version is the ATS-safe rewrite of a two-column, table-driven
// resume: no table, no column, no sidebar, no image. A section is a level
// two heading, an entry is a bold title line followed by an organization and
// date line, and achievements are a plain unordered list. Converted with
// `asciidoctor -b pdf out.adoc` or `asciidoctor -b html5 out.adoc`, the result
// stays a single column of selectable text.
package adoc

import (
	"strings"

	"github.com/fanama/resume/atscv/internal/layout"
	"github.com/fanama/resume/atscv/internal/model"
)

// Options configures the AsciiDoc output.
type Options struct {
	// Lang is the document language attribute.
	Lang model.Lang
	// SectionRules draws a horizontal rule under the section headings.
	SectionRules bool
	// Header is the optional comment printed at the top of the file.
	Header string
}

// Render returns the AsciiDoc source of a layout.
func Render(l *layout.Layout, opts Options) string {
	lang := opts.Lang
	if lang == "" {
		lang = model.LangFR
	}
	title := "Resume"
	for _, b := range l.Blocks {
		if b.Kind == layout.KindName {
			// An empty name still has to produce a valid document title.
			if name := strings.TrimSpace(b.Text); name != "" {
				title = name
			}
			break
		}
	}

	var b strings.Builder
	if opts.Header != "" {
		for _, line := range strings.Split(strings.TrimRight(opts.Header, "\n"), "\n") {
			b.WriteString("// " + line + "\n")
		}
	}
	b.WriteString("= " + title + "\n")
	b.WriteString(":doctype: article\n")
	b.WriteString(":lang: " + string(lang) + "\n")
	b.WriteString(":encoding: utf-8\n")
	b.WriteString(":icons: font\n")
	b.WriteString(":sectanchors:\n")
	b.WriteString(":nofooter:\n")
	b.WriteString(":pdf-page-size: A4\n")
	b.WriteString(":pdf-fonts: embedded\n")
	b.WriteString(":pdf-margin-left: 18mm\n")
	b.WriteString(":pdf-margin-right: 18mm\n")
	b.WriteString(":pdf-margin-top: 16mm\n")
	b.WriteString(":pdf-margin-bottom: 16mm\n")
	b.WriteString(":outfilesuffix: .pdf\n")
	b.WriteString("\n")

	// The name is the document title, so it is not repeated in the body.
	var headline string
	var contacts []string
	for _, blk := range l.Blocks {
		switch blk.Kind {
		case layout.KindHeadline:
			if headline == "" {
				headline = blk.Text
			}
		case layout.KindContact:
			contacts = append(contacts, blk.Text)
		}
	}
	if headline != "" {
		b.WriteString("\n[.text-center]\n*" + headline + "*\n")
	}
	if len(contacts) > 0 {
		b.WriteString("\n[.text-center]\n" + strings.Join(contacts, " +\n") + "\n")
	}

	gap := false
	for _, blk := range l.Blocks {
		switch blk.Kind {
		case layout.KindSummary:
			if blk.Text == "" {
				gap = true
				continue
			}
			b.WriteString("\n" + blk.Text + "\n")
		case layout.KindSection:
			b.WriteString("\n== " + blk.Text + "\n")
			if opts.SectionRules {
				b.WriteString("\n'''\n")
			}
		case layout.KindName, layout.KindHeadline, layout.KindContact:
			// Already written in the header.
			continue
		case layout.KindEntryTitle:
			b.WriteString("\n" + gapLine(gap) + "*" + blk.Text + "*")
			if blk.Dim != "" {
				b.WriteString(" | " + blk.Dim)
			}
			b.WriteString("\n")
		case layout.KindEntryMeta:
			b.WriteString(blk.Text + "\n")
		case layout.KindBullet:
			b.WriteString("* " + blk.Text + "\n")
		case layout.KindField:
			if blk.Bold != "" {
				b.WriteString("\n*" + blk.Bold + "*: " + blk.Text + "\n")
			} else {
				b.WriteString("\n" + blk.Text + "\n")
			}
		}
		gap = false
	}
	return b.String()
}

func gapLine(gap bool) string {
	if gap {
		return "\n"
	}
	return ""
}
