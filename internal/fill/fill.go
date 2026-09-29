// Package fill builds a resume draft from the plain text of a PDF.
//
// A PDF is a drawing instruction stream, not a data structure. What comes back
// from a text extraction is a list of positioned glyphs, and everything this
// package relies on — the name, the section boundaries, which dates belong to
// which job — has to be guessed from the shape of the lines. So the output is a
// draft, never an authority: every field it fills is a proposal the candidate
// corrects.
//
// The split of work with the browser is deliberate. The browser decodes the PDF,
// because that is mechanical, and this package decides what the text means,
// because that is where the schema lives and where a test can reach it. The
// server keeps no file, no draft and no state.
//
// Nothing here invents a value. A field that cannot be read confidently is left
// empty, because an empty field is visibly empty and a wrong one is not.
package fill

import (
	"errors"
	"strings"
	"unicode"

	"github.com/fanama/resume/atscv/internal/layout"
	"github.com/fanama/resume/atscv/internal/model"
)

// ErrNoText is returned when the extraction produced nothing readable. A PDF
// made of a scan, or of vector outlines with no text layer, is the usual cause,
// and the remedy is different: OCR, which this tool deliberately does not do.
var ErrNoText = errors.New("the PDF holds no extractable text, so it is probably a scan: it needs OCR, which this tool does not do")

// Result is a draft and an account of what could not be placed.
type Result struct {
	Resume *model.Resume
	Lang   model.Lang
	// Unmapped holds section headings the schema has nowhere to put. They are
	// reported rather than dropped: a heading nobody recognises is often the
	// one the candidate cares about, and silently losing it is how a draft
	// becomes a document full of holes.
	Unmapped []string
}

// Run turns extracted text into a draft resume. lang is the language to read the
// headings in; when it is empty the text is inspected to guess, and French is
// the fallback because that is the tool's default.
func Run(text string, lang model.Lang) (*Result, error) {
	lines := cleanLines(text)
	if !readable(lines) {
		return nil, ErrNoText
	}
	lang = guessLang(lines, lang)
	l := layout.New(&model.Resume{}, layout.Options{Lang: lang})
	f := &filler{lang: lang, layout: l, resume: &model.Resume{}}
	f.run(lines)
	f.resume.Lang = lang
	f.resume.Normalize()
	return &Result{Resume: f.resume, Lang: lang, Unmapped: f.unmapped}, nil
}

// filler carries the state of one run. The layout is only used to read dates:
// layout is the package that owns what a period means, and a date this tool
// cannot parse is a date the candidate will have to retype anyway.
type filler struct {
	lang     model.Lang
	layout   *layout.Layout
	resume   *model.Resume
	seenName bool
	unmapped []string
	// headerFields are the folded fields the header states, read before the
	// sections so that an entry can ask which of its parts is a place.
	headerFields []string
}

func (f *filler) run(lines []string) {
	// Contact first, and over the whole document: a phone number or a link
	// sits in the header in one template and in the footer in another, so the
	// position says nothing. The lines it consumed are removed, so that a phone
	// number is not read a second time as the text of a job.
	// The header is read for its fields before the contact block is taken out
	// of it. readContact consumes the line that states where the candidate
	// lives, so asking afterwards leaves nothing to ask about: the fields came
	// back empty, no entry could recognise its own city, and "SUPINFO Paris,
	// Paris | 2017 - 2020" imported with "Paris" as the degree.
	f.headerFields = headerFields(lines[:firstHeadingIndex(lines)])

	body := f.readContact(lines)

	// Everything before the first heading is the identity block: the name, the
	// title, and sometimes a profile paragraph. Everything from the first
	// heading on is sectioned.
	cut := firstHeadingIndex(body)
	head, rest := body[:cut], body[cut:]
	leftover := f.readIdentity(head)

	// The head lines that carried no identity field are, on a one page CV, the
	// profile. Attach them to the summary rather than lose them.
	if s := joinNonEmpty(leftover); s != "" && f.resume.Summary == "" {
		f.resume.Summary = truncate(s, 1200)
	}

	for _, sec := range splitSections(rest) {
		f.readSection(sec)
	}
	f.dropEmpty()
}

// cleanLines normalises what an extraction hands over. The spaces matter more
// than they look: a PDF written by a word processor is full of non-breaking and
// thin spaces, and a heading split by one of them matches no heading list.
func cleanLines(text string) []string {
	// The spaces a layout engine uses where a normal space would let a line
	// wrap: no-break, figure, narrow no-break, and the two line separators a
	// text run can end with. The soft hyphen is dropped: it is a hint to break
	// a word, and it is invisible once the word is whole.
	text = strings.NewReplacer(
		"\u00a0", " ", // no-break space
		"\u2007", " ", // figure space
		"\u202f", " ", // narrow no-break space
		"\u2009", " ", // thin space
		"\u00ad", "", // soft hyphen
		"\u2028", "\n", // line separator
		"\u2029", "\n", // paragraph separator
		"\r\n", "\n",
		"\r", "\n",
	).Replace(text)

	var out []string
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(collapseSpaces(raw))
		if line == "" {
			continue
		}
		out = append(out, line)
		// A wall of text is a page of a scanned document that a text layer
		// claims but does not deliver. Stop early rather than feed the parser
		// thousands of lines it will misread as sections.
		if len(out) > 4000 {
			break
		}
	}
	return out
}

// readable rejects text that carries no words. A scan produces an empty layer,
// but a PDF written by a scanner with a bad layer produces replacement
// characters and fragments of glyph indexes: lines that are there and hold
// nothing. Counting letters separates both from a one page CV.
func readable(lines []string) bool {
	letters, words := 0, 0
	for _, l := range lines {
		inWord := false
		for _, r := range l {
			switch {
			case unicode.IsLetter(r):
				letters++
				inWord = true
			case unicode.IsDigit(r):
				inWord = true
			default:
				if inWord {
					words++
					inWord = false
				}
			}
		}
		if inWord {
			words++
		}
	}
	// A name and a headline is about four words; asking for ten keeps a
	// one line scan out without rejecting a very short CV.
	return words >= 10 && letters >= 30
}

// collapseSpaces squeezes runs of spaces, which a PDF produces wherever two text
// runs sit next to each other with a rounding error between them.
func collapseSpaces(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	space := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			space = true
			continue
		}
		if space && b.Len() > 0 {
			b.WriteByte(' ')
		}
		space = false
		b.WriteRune(r)
	}
	return b.String()
}

// dropEmpty removes the entries that ended up with nothing in them, so that a
// section is either filled or absent. The editor shows an empty card otherwise,
// and a candidate cannot tell a blank card from a mistake.
func (f *filler) dropEmpty() {
	keep := func(items []model.Experience) []model.Experience {
		out := make([]model.Experience, 0, len(items))
		for _, e := range items {
			if e.Title != "" || e.Company != "" || e.Summary != "" || len(e.Highlights) > 0 {
				out = append(out, e)
			}
		}
		return out
	}
	f.resume.Experience = keep(f.resume.Experience)
	edu := make([]model.Education, 0, len(f.resume.Education))
	for _, e := range f.resume.Education {
		if e.Degree != "" || e.School != "" || e.Summary != "" {
			edu = append(edu, e)
		}
	}
	f.resume.Education = edu
	if f.resume.Name == "" {
		f.resume.Headline = ""
	}
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len([]rune(s)) <= n {
		return s
	}
	r := []rune(s)
	return strings.TrimSpace(string(r[:n])) + "…"
}

func joinNonEmpty(lines []string) string {
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return strings.Join(out, " ")
}
