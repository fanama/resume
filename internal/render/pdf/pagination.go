package pdf

import "github.com/fanama/resume/atscv/internal/layout"

// item is a block with the vertical space it needs, measured before it is
// drawn. A page break taken after the fact is the orphan the reader sees, so the
// renderer measures first and breaks where it decided to, not where the text ran
// out.
type item struct {
	block  layout.Block
	gap    float64 // the space a spacer block left for this one
	height float64
	// entryEnd and entryHeight describe the entry a title opens, so an entry
	// can be kept on one page as a whole.
	entryEnd    int
	entryHeight float64
	// sectionEnd and sectionHeight do the same for a section heading: the whole
	// section, so a rubric that fits on a page is never split, which is what
	// keeps a heading off the bottom of a page.
	sectionEnd    int
	sectionHeight float64
}

// plan turns the block stream into measured items. A spacer is not an item: its
// height becomes the gap of the block that follows, which is how the vertical
// rhythm survives a page break instead of leaving a hole at the top of a page.
func (r *renderer) plan(l *layout.Layout) []item {
	items := make([]item, 0, len(l.Blocks))
	gap := 0.0
	seenSection := false
	for _, b := range l.Blocks {
		if b.Kind == layout.KindSummary && b.Text == "" {
			gap += spaceBeforeEntry
			continue
		}
		it := item{block: b, gap: gap, entryEnd: -1, sectionEnd: -1, height: gap}
		gap = 0
		r.fontOf(b.Kind)
		switch b.Kind {
		case layout.KindSection:
			it.height += spaceBeforeSection + r.lineHeight()
			if r.opts.SectionRules {
				it.height += ruleBelow + spaceAfterSection
				if !seenSection {
					// The first heading also carries the rule that separates
					// the identity from the content.
					it.height += ruleAbove + gapBelowRule
				}
			}
			seenSection = true
		case layout.KindBullet:
			it.height += r.lines(b.Text, r.bulletWidth()) * r.lineHeight()
		case layout.KindField:
			it.height += r.fieldLines(b)
		default:
			it.height += r.lines(b.Text, r.contentWidth()) * r.lineHeight()
		}
		items = append(items, it)
	}
	r.markEntries(items)
	r.markSections(items)
	return items
}

// markEntries records where each entry ends, so the pagination can keep a whole
// job description, degree or activity on one page.
func (r *renderer) markEntries(items []item) {
	for i := range items {
		if items[i].block.Kind != layout.KindEntryTitle {
			continue
		}
		h := items[i].height
		end := i + 1
		for j := i + 1; j < len(items); j++ {
			if items[j].block.Kind == layout.KindEntryTitle || items[j].block.Kind == layout.KindSection {
				break
			}
			h += items[j].height
			end = j + 1
		}
		items[i].entryHeight = h
		items[i].entryEnd = end
	}
}

// markSections records the height of each section, heading included. A section
// that fits on a page of its own is then moved there whole, instead of leaving
// its heading at the bottom of a page with two of its entries and pushing the
// rest overleaf.
func (r *renderer) markSections(items []item) {
	for i := range items {
		if items[i].block.Kind != layout.KindSection {
			continue
		}
		h := items[i].height
		end := i + 1
		for j := i + 1; j < len(items); j++ {
			if items[j].block.Kind == layout.KindSection {
				break
			}
			h += items[j].height
			end = j + 1
		}
		items[i].sectionHeight = h
		items[i].sectionEnd = end
	}
}

// run lays the document out.
func (r *renderer) run(l *layout.Layout) {
	items := r.plan(l)
	first := true
	for i := range items {
		it := items[i]
		switch it.block.Kind {
		case layout.KindName:
			// The identity is a unit: a name alone at the top of a second page,
			// away from the contact details, is worse than a short page.
			r.breakIfNeeded(r.untilSection(items, i))
		case layout.KindSection:
			// A rubric that fits on a page of its own is never split: a heading
			// stranded at the bottom of a page is the title at the end of the
			// page, whether a line follows it or not. A section taller than a
			// page has to split, and then it keeps its rule and its first line.
			if it.sectionHeight > 0 && it.sectionHeight <= r.usableHeight() {
				r.breakIfTail(it.sectionHeight, it.sectionEnd < len(items))
			} else {
				r.breakIfNeeded(it.height + r.nextLine(items, i+1))
			}
			first = false
		case layout.KindEntryTitle:
			// Keep a whole entry together when it fits on a page. When it is
			// taller than one, it has to split, but its header stays with the
			// first line of its body: "Ingénieur, Acme, 2021" followed by an
			// unrelated page is how a resume loses a reader.
			more := it.entryEnd < len(items)
			if it.entryHeight > 0 && it.entryHeight <= r.usableHeight() {
				r.breakIfTail(it.entryHeight, more)
			} else {
				r.breakIfTail(r.withFirstLine(items, i), more)
			}
		}
		r.draw(it, first && it.block.Kind == layout.KindSection)
	}
}

// untilSection is the height of everything from i up to the next section
// heading: the identity block, for the only call there is.
func (r *renderer) untilSection(items []item, i int) float64 {
	h := 0.0
	for j := i; j < len(items); j++ {
		if items[j].block.Kind == layout.KindSection {
			break
		}
		h += items[j].height
	}
	return h
}

// nextLine is the height of the first printed line of the item at i, or a single
// line when the section turns out to be empty.
func (r *renderer) nextLine(items []item, i int) float64 {
	if i < len(items) && items[i].height > 0 {
		return items[i].height
	}
	return r.lineHeight()
}

// withFirstLine is the height of an entry header plus the first line of its
// body.
func (r *renderer) withFirstLine(items []item, i int) float64 {
	h := items[i].height
	for j := i + 1; j < len(items); j++ {
		if items[j].block.Kind == layout.KindEntryTitle || items[j].block.Kind == layout.KindSection {
			break
		}
		h += items[j].height
		if items[j].block.Kind != layout.KindEntryMeta {
			break
		}
	}
	return h
}

// breakIfNeeded starts a new page when the next h millimetres do not fit.
func (r *renderer) breakIfNeeded(h float64) {
	_, height := r.f.GetPageSize()
	if r.f.GetY()+h > height-pageMargin {
		r.f.AddPage()
	}
}

// breakIfTail is breakIfNeeded plus the bottom band. When the document carries
// on, a page may not end inside the last two lines: a break there leaves a
// title under one line of content, or one line of content under a title, and
// both read as an accident rather than as a page. The last block of the
// document is exempt, since nothing follows it and no reader can see a break
// that is not there.
func (r *renderer) breakIfTail(h float64, more bool) {
	_, height := r.f.GetPageSize()
	limit := height - pageMargin
	if r.f.GetY()+h > limit || (more && r.f.GetY()+h > limit-r.tail()) {
		r.f.AddPage()
	}
}

// tail is the height of the bottom band, in lines.
func (r *renderer) tail() float64 {
	return r.lineHeight() * tailLines
}

// usableHeight is the height a page offers between the two margins.
func (r *renderer) usableHeight() float64 {
	_, height := r.f.GetPageSize()
	return height - 2*pageMargin
}
