package docx

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"time"
)

// Render writes a .docx file made of the given paragraphs.
func Render(w io.Writer, paragraphs []Paragraph, opts Options) error {
	opts = normalizeOptions(opts)
	// A fixed timestamp keeps the archive byte-for-byte reproducible, which
	// makes it reviewable in git.
	modTime := time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC)

	zw := zip.NewWriter(w)
	for _, part := range documentParts(paragraphs, opts) {
		if err := writePart(zw, part, modTime); err != nil {
			return err
		}
	}
	if err := zw.Close(); err != nil {
		return fmt.Errorf("docx: close archive: %w", err)
	}
	return nil
}

// Bytes renders a .docx into memory.
func Bytes(paragraphs []Paragraph, opts Options) ([]byte, error) {
	var buf bytes.Buffer
	if err := Render(&buf, paragraphs, opts); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// normalizeOptions fills the defaults the CLI may not have provided.
func normalizeOptions(opts Options) Options {
	if opts.FontFamily == "" {
		opts.FontFamily = "Calibri"
	}
	if opts.FontSize <= 0 {
		opts.FontSize = 10.5
	}
	if opts.LangTag == "" {
		opts.LangTag = "fr-FR"
	}
	if opts.SectionColor == "" {
		opts.SectionColor = defaultAccent
	}
	if opts.Title == "" {
		opts.Title = "Resume"
	}
	return opts
}

// part is one file of the OOXML archive.
type part struct {
	name string
	body string
}

// documentParts builds every part of the archive, in the order it is written.
func documentParts(paragraphs []Paragraph, opts Options) []part {
	return []part{
		{"[Content_Types].xml", contentTypes()},
		{"_rels/.rels", rootRels()},
		{"docProps/core.xml", coreProps(opts)},
		{"docProps/app.xml", appProps(opts)},
		{"word/_rels/document.xml.rels", documentRels()},
		{"word/document.xml", documentXML(paragraphs, opts)},
		{"word/styles.xml", stylesXML(opts)},
		{"word/numbering.xml", numberingXML()},
		{"word/settings.xml", settingsXML(opts)},
		{"word/fontTable.xml", fontTableXML(opts.FontFamily)},
	}
}

// writePart writes one deflated part to the archive.
func writePart(zw *zip.Writer, p part, modTime time.Time) error {
	hdr := &zip.FileHeader{Name: p.name, Method: zip.Deflate, Modified: modTime}
	fw, err := zw.CreateHeader(hdr)
	if err != nil {
		return fmt.Errorf("docx: create %s: %w", p.name, err)
	}
	if _, err := io.WriteString(fw, p.body); err != nil {
		return fmt.Errorf("docx: write %s: %w", p.name, err)
	}
	return nil
}
