package fill

import "strings"

// entry is one job, one school or one project, before it is split across the
// fields of the schema.
type entry struct {
	title    string
	company  string
	location string
	start    string
	end      string
	bullets  []string
	notes    []string

	// openBullet records that the last bullet stopped mid-sentence, so the
	// next unmarked line is its continuation rather than a new highlight.
	openBullet bool

	// team and stack hold the "Équipe:" and "Technologies:" lines the renderer
	// writes under an entry, so that they go back to the fields they came from
	// instead of being filed as achievements.
	team  string
	stack []string

	// openField names the field whose line wrapped, if any. A long stack runs
	// past the width and continues on the next line, which carries no label.
	openField string
}

// sectionBuilder collects the entries of one section while its lines are read.
type sectionBuilder struct {
	filler  *filler
	entries []entry
	current *entry
	loose   []string
	// pending is a title line that closed an entry: the renderer prints a
	// title above the date of the entry it opens, so it is read while the
	// previous entry is still open. It waits here for that entry to arrive.
	pending string
}

// entries cuts a section into entries. The rule is a period: a line that carries
// one opens an entry, and the lines that follow belong to it until the next
// such line. This is the only rule that works across templates, because the
// period is the one thing every resume states for every position.
//
// A section that states no period at all is read as a single entry, so that a
// list of titles without dates is not lost.
func (f *filler) entries(lines []string) []entry {
	b := &sectionBuilder{filler: f}
	for _, raw := range lines {
		b.consume(strings.TrimSpace(raw))
	}
	return b.finish()
}

// consume routes one line: a heading closes the entry, a line with a period
// opens one, and everything else fills the entry that is open.
func (b *sectionBuilder) consume(line string) {
	if isDecorative(line) && !hasDate(line) {
		return
	}
	if _, isHeading := sectionOf(line); isHeading {
		// A second heading of the same section, as in "Experience" then
		// "Volunteering" inside one block: close what is open and move on.
		b.flush()
		return
	}
	if start, end, hasPeriod := splitPeriod(line); hasPeriod {
		b.openPeriodEntry(line, start, end)
		return
	}
	if b.current == nil {
		b.loose = append(b.loose, line)
		return
	}
	b.consumeContent(line)
}

// flush closes the open entry, moving a stray trailing title to pending.
func (b *sectionBuilder) flush() {
	if b.current == nil {
		return
	}
	if n := len(b.current.notes); n > 1 {
		if last := b.current.notes[n-1]; strayTitle(last) {
			// Dropped, not returned to loose: the loose lines are
			// drained back onto the last entry's notes further down,
			// so handing it over there would put it straight back.
			b.current.notes = b.current.notes[:n-1]
			b.pending = last
		}
	}
	b.entries = append(b.entries, *b.current)
	b.current = nil
}

// openPeriodEntry starts the entry a date line announces, recovering the title
// the previous entry gave up.
func (b *sectionBuilder) openPeriodEntry(line, start, end string) {
	// The title this entry announced, held from the line before. It is
	// saved across the flush, which sets pending itself and would
	// otherwise overwrite the title that is already waiting.
	held := b.pending
	b.flush()
	if held != "" {
		b.pending = held
	}
	e := b.filler.buildEntry(line, start, end)
	b.recoverTitle(&e)
	b.pending = ""
	b.current = &e
}

// recoverTitle finds the title an entry line did not carry: the line above it
// when it prints the title first, or the previous entry's trailing title.
func (b *sectionBuilder) recoverTitle(e *entry) {
	// Plenty of layouts print the title on its own line and the company
	// and the period under it, which puts a title before the first line
	// that opens an entry. That line is the title of the entry that
	// follows it, and the company is on the line that opened it.
	if e.title == "" {
		if n := lastTitleLine(b.loose); n >= 0 {
			e.title = b.loose[n]
			b.loose = b.loose[:n]
		}
	}
	// The title the previous entry gave up belongs to this one. It is
	// taken either from the flush above, or straight out of the notes
	// of the previous entry: an entry whose description is a single
	// line keeps its trailing title, because flush leaves a lone note
	// alone rather than risk emptying a real description. The second
	// degree of a FORMATION block is exactly that shape.
	if e.title == "" && b.pending == "" && len(b.entries) > 0 {
		if prev := &b.entries[len(b.entries)-1]; len(prev.notes) > 0 {
			last := prev.notes[len(prev.notes)-1]
			if strayTitle(last) {
				b.pending = last
				prev.notes = prev.notes[:len(prev.notes)-1]
			}
		}
	}
	if e.title == "" && b.pending != "" {
		e.title = b.pending
	}
}

// finish closes the section and attaches the loose lines that preceded the
// first period.
func (b *sectionBuilder) finish() []entry {
	b.flush()

	// No entry carried a period: the section is a list, not a series of posts.
	if len(b.entries) == 0 && len(b.loose) > 0 {
		return b.filler.looseEntries(b.loose)
	}
	// Some lines that came before the first period are still content, and they
	// are attached to the last entry so that they are not lost.
	//
	// A title line is the exception. This tool prints the title above the date
	// of the entry it opens, so the title of a post whose date line never
	// parsed stays here, and attaching it would end the previous job's
	// description with the name of the next one. It names an entry that could
	// not be built; appending it to a different entry states something the CV
	// does not say, so it is dropped.
	for _, l := range b.loose {
		if isDecorative(l) {
			continue
		}
		if len(b.entries) == 0 {
			continue
		}
		last := &b.entries[len(b.entries)-1]
		if strayTitle(l) && len(last.notes) > 0 {
			continue
		}
		if len(last.bullets) < 6 {
			last.notes = append(last.notes, l)
		}
	}
	return b.entries
}

// looseEntries reads a section whose lines carry no period: one entry per line,
// with the bullets under it.
func (f *filler) looseEntries(lines []string) []entry {
	var out []entry
	var cur *entry
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if isDecorative(line) {
			continue
		}
		if bulletRe.MatchString(line) {
			if cur != nil {
				if b := cleanItem(bulletRe.ReplaceAllString(line, "")); b != "" {
					cur.bullets = append(cur.bullets, b)
				}
			}
			continue
		}
		e := f.buildEntry(line, "", "")
		if e.title != "" || e.company != "" {
			out = append(out, e)
			cur = &e
		}
	}
	return out
}
