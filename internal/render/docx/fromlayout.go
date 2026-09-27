package docx

import "github.com/fanama/resume/atscv/internal/layout"

// FromLayout maps a layout to the paragraph list of a .docx document. Blocks
// keep their order, so Word's outline is the resume's reading order. The first
// section heading gets a hairline above it, which separates the identity from
// the content without adding a line of text.
func FromLayout(l *layout.Layout) []Paragraph {
	paragraphs := make([]Paragraph, 0, len(l.Blocks)+4)
	var entryStarts []int
	seenSection := false
	for _, b := range l.Blocks {
		switch b.Kind {
		case layout.KindName:
			paragraphs = append(paragraphs, Line(StyleName, b.Text))
		case layout.KindHeadline:
			paragraphs = append(paragraphs, Line(StyleHeadline, b.Text))
		case layout.KindContact:
			paragraphs = append(paragraphs, Line(StyleContact, b.Text))
		case layout.KindSection:
			p := Line(StyleSection, b.Text)
			p.RuleTop = !seenSection
			seenSection = true
			paragraphs = append(paragraphs, p)
		case layout.KindEntryTitle:
			entryStarts = append(entryStarts, len(paragraphs))
			p := Paragraph{Style: StyleEntry, Runs: []Run{{Text: b.Text, Bold: true}}}
			if b.Dim != "" {
				p.Runs = append(p.Runs, Run{Text: " | " + b.Dim, Dim: true})
			}
			paragraphs = append(paragraphs, p)
		case layout.KindEntryMeta:
			paragraphs = append(paragraphs, Line(StyleEntryMeta, b.Text))
		case layout.KindSummary:
			if b.Text == "" {
				// A spacer between two entries: Word needs no empty paragraph,
				// the entry styles already carry a top spacing.
				continue
			}
			paragraphs = append(paragraphs, Line(StyleSummary, b.Text))
		case layout.KindBullet:
			paragraphs = append(paragraphs, Line(StyleBullet, b.Text))
		case layout.KindField:
			if b.Bold != "" {
				paragraphs = append(paragraphs, Labeled(StyleField, b.Bold, b.Text))
			} else {
				paragraphs = append(paragraphs, Line(StyleField, b.Text))
			}
		}
	}
	return chainEntries(paragraphs, entryStarts)
}

// chainEntries marks every paragraph of an entry but its last one with
// keepNext, which is how Word is told to keep an experience, a training or an
// activity whole when it fits on a page. The styles already chain a title to its
// meta and a section to its first line; this extends the chain to the end of
// the entry, and deliberately stops there. Binding the last paragraph to what
// follows would tie two entries together, and three short entries would then
// chase each other down the document until a page could not hold them.
func chainEntries(paragraphs []Paragraph, starts []int) []Paragraph {
	for i, start := range starts {
		end := len(paragraphs)
		if i+1 < len(starts) {
			end = starts[i+1]
		}
		for j := start; j < end-1; j++ {
			paragraphs[j].KeepNext = true
		}
	}
	return paragraphs
}
