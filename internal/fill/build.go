package fill

import (
	"regexp"
	"strings"

	"github.com/fanama/resume/atscv/internal/layout"
	"github.com/fanama/resume/atscv/internal/model"
)

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

// entries cuts a section into entries. The rule is a period: a line that carries
// one opens an entry, and the lines that follow belong to it until the next
// such line. This is the only rule that works across templates, because the
// period is the one thing every resume states for every position.
//
// A section that states no period at all is read as a single entry, so that a
// list of titles without dates is not lost.
func (f *filler) entries(lines []string) []entry {
	var out []entry
	var cur *entry
	var loose []string
	// pending is a title line that closed an entry: the renderer prints a
	// title above the date of the entry it opens, so it is read while the
	// previous entry is still open. It waits here for that entry to arrive.
	var pending string

	flush := func() {
		if cur != nil {
			// The renderer prints a title above the date line of the entry it
			// belongs to, so the title of the next post arrives while this one
			// is still open and lands at the end of its description. It is
			// taken back here rather than only when the next date line is
			// seen, because the last entry of a section has no next date line
			// and would keep the stray title for good.
			if n := len(cur.notes); n > 1 {
				if last := cur.notes[n-1]; strayTitle(last) {
					// Dropped, not returned to loose: the loose lines are
					// drained back onto the last entry's notes further down,
					// so handing it over there would put it straight back.
					cur.notes = cur.notes[:n-1]
					pending = last
				}
			}
			out = append(out, *cur)
			cur = nil
		}
	}

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if isDecorative(line) && !hasDate(line) {
			continue
		}
		if _, isHeading := sectionOf(line); isHeading {
			// A second heading of the same section, as in "Experience" then
			// "Volunteering" inside one block: close what is open and move on.
			flush()
			continue
		}

		start, end, hasPeriod := splitPeriod(line)
		if hasPeriod {
			// The title this entry announced, held from the line before. It is
			// saved across the flush, which sets pending itself and would
			// otherwise overwrite the title that is already waiting.
			held := pending
			flush()
			if held != "" {
				pending = held
			}
			e := f.buildEntry(line, start, end)
			// Plenty of layouts print the title on its own line and the company
			// and the period under it, which puts a title before the first line
			// that opens an entry. That line is the title of the entry that
			// follows it, and the company is on the line that opened it.
			if e.title == "" {
				if n := lastTitleLine(loose); n >= 0 {
					e.title = loose[n]
					loose = loose[:n]
				}
			}
			// The title the previous entry gave up belongs to this one. It is
			// taken either from the flush above, or straight out of the notes
			// of the previous entry: an entry whose description is a single
			// line keeps its trailing title, because flush leaves a lone note
			// alone rather than risk emptying a real description. The second
			// degree of a FORMATION block is exactly that shape.
			if e.title == "" && pending == "" && len(out) > 0 {
				if prev := &out[len(out)-1]; len(prev.notes) > 0 {
					last := prev.notes[len(prev.notes)-1]
					if strayTitle(last) {
						pending = last
						prev.notes = prev.notes[:len(prev.notes)-1]
					}
				}
			}
			if e.title == "" && pending != "" {
				e.title = pending
			}
			pending = ""
			cur = &e
			continue
		}

		if cur == nil {
			loose = append(loose, line)
			continue
		}
		// A line under a title is a bullet when it is marked as one, and a
		// second title line otherwise: "Acme Corp" under "Engineer | 2019".
		if bulletRe.MatchString(line) {
			stripped := bulletRe.ReplaceAllString(line, "")
			// cleanBullet, not cleanItem: cleanItem is the cleaner for a skill
			// list and throws away anything over 40 characters or six words,
			// which is every achievement a candidate ever wrote. Used here it
			// silently deleted the first line of every marked bullet.
			if b := cleanBullet(stripped); b != "" {
				cur.bullets = append(cur.bullets, b)
				// Whether the bullet wraps is judged on the line as printed,
				// not on the cleaned item: cleanItem removes the full stop
				// that ends it, which would make every finished bullet look
				// like it runs on and swallow the one after it.
				cur.openBullet = continuesSentence(stripped)
			}
			continue
		}
		// "Équipe: ..." and "Technologies: ..." are fields, not achievements.
		// They are what this tool itself prints under an entry, so failing to
		// read them back means the app cannot reimport its own output: the
		// team and the stack came in as two more bullets at the end of the
		// list, and the fields they belong to stayed empty.
		if label, value, ok := fieldLine(line); ok {
			switch label {
			case fieldTeam:
				cur.team = value
			case fieldStack:
				cur.stack = splitItems(value)
			}
			cur.openBullet = false
			// The list is open only if this line stopped on a separator.
			cur.openField = ""
			if strings.HasSuffix(strings.TrimSpace(line), ",") {
				cur.openField = label
			}
			continue
		}
		// A stack long enough to wrap continues on an unlabelled line, and the
		// tail is a list item like the rest, not a highlight. The list is only
		// still open when the line before it was cut between two items, which
		// the trailing separator proves: "OpenAI, GCP," then "Elasticsearch".
		// Without that proof a short line here is the title of the next entry.
		if cur.openField != "" && continuesField(line) {
			switch cur.openField {
			case fieldTeam:
				cur.team = joinWrapped(cur.team, line)
			case fieldStack:
				cur.stack = append(cur.stack, splitItems(line)...)
			}
			// The tail closes the list unless it too was cut on a separator.
			if !strings.HasSuffix(strings.TrimSpace(line), ",") {
				cur.openField = ""
			}
			continue
		}
		cur.openField = ""
		// A title is a title, not merely the first line that turned up. The
		// entry whose date line named the company leaves the title empty, and
		// taking whatever follows made the opening line of the description the
		// job title and left the description empty: the line wraps, so it read
		// as "Éditeur d'une plateforme d'analyse en temps réel de flux vidéo
		// captés par des caméras en".
		if cur.title == "" && isTitleish(line) && !continuesSentence(line) {
			cur.title = line
			continue
		}
		// A title line under an entry that already has one announces the next
		// entry: this tool prints the title above the date it belongs to. It
		// is held until that date line arrives rather than filed as content,
		// which is where the job titles of every post but the first went.
		//
		// Two conditions, each paid for by a bug. The line itself must not be
		// a wrapped one, or the opening line of a description becomes the
		// title of the next job. And the description above must be closed, or
		// its second line is stolen. Requiring a description outright was the
		// first attempt and lost the title of a job whose predecessor ended on
		// its Technologies line.
		if cur.title != "" && !cur.openBullet && strayTitle(line) && !continuesSentence(line) &&
			(len(cur.notes) == 0 || !continuesSentence(cur.notes[len(cur.notes)-1])) {
			pending = line
			continue
		}
		if cur.company == "" && isTitleish(line) {
			cur.company = line
			continue
		}
		// What is left under a title is what the candidate did, and it comes in
		// two shapes. Plenty of templates print the highlights as plain lines,
		// because a PDF keeps no bullet mark, so an unmarked short line is a
		// highlight. But an entry often opens with a paragraph describing the
		// role, and a paragraph is not a list: it wraps, so reading each of its
		// lines as a highlight cuts sentences in half and produces a form full
		// of fragments that stop mid-clause.
		//
		// The two are told apart by how the line reads. Two rules that looked
		// right were tried and dropped: a terminal full stop does not mark
		// prose, since 30 of the 36 highlights in the resumes this tool ships
		// end in one; and position does not either, because plenty of entries
		// are three unmarked achievements in a row with no paragraph at all,
		// and taking everything before the first bullet swallows them.
		//
		// What is left is the shape of the sentence. A paragraph wraps, so a
		// line that stops mid-clause is continued by the next one and the two
		// are put back together. A highlight stands on its own line.
		// A line that continues something goes back onto whatever is open, and
		// the bullet is tested first: the half of a wrapped bullet is long and
		// stops mid-clause, so anything that reads the line on its own merits
		// would call it a paragraph and file it away from the list it belongs
		// to, leaving the bullet a fragment and the description a mess.
		if cur.openBullet && len(cur.bullets) > 0 {
			n := len(cur.bullets) - 1
			cur.bullets[n] = joinWrapped(cur.bullets[n], line)
			cur.openBullet = continuesSentence(line)
			continue
		}
		if len(cur.notes) > 0 && continuesSentence(cur.notes[len(cur.notes)-1]) {
			cur.notes[len(cur.notes)-1] = joinWrapped(cur.notes[len(cur.notes)-1], line)
			continue
		}
		// The description opens the entry, before any bullet, and it is known
		// by the fact that it wraps: the page broke it mid-clause, so the line
		// does not end on a stop. An unmarked achievement, by contrast, is
		// written as one complete line. That single test is what separates the
		// two, and it is the only one that held for both of the real shapes:
		// the paragraph this tool's own renderer emits, and the run of three
		// bare achievement lines a plain template emits.
		if isProse(line) || (len(cur.bullets) == 0 && continuesSentence(line)) {
			if len(cur.notes) < 8 {
				cur.notes = append(cur.notes, strings.TrimSpace(line))
			}
			continue
		}
		if len([]rune(line)) <= 200 {
			if b := cleanBullet(line); b != "" {
				if len(cur.bullets) < 8 {
					cur.bullets = append(cur.bullets, b)
					// An unmarked highlight wraps too, and its second line has
					// no mark either. Judged on the line as printed, for the
					// same reason as above: cleanBullet drops the full stop
					// that proves the highlight was finished.
					cur.openBullet = continuesSentence(line)
				}
				continue
			}
		}
		if len(cur.notes) < 8 {
			cur.notes = append(cur.notes, line)
		}
	}

	flush()

	// No entry carried a period: the section is a list, not a series of posts.
	if len(out) == 0 && len(loose) > 0 {
		return f.looseEntries(loose)
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
	for _, l := range loose {
		if isDecorative(l) {
			continue
		}
		if len(out) == 0 {
			continue
		}
		last := &out[len(out)-1]
		if strayTitle(l) && len(last.notes) > 0 {
			continue
		}
		if len(last.bullets) < 6 {
			last.notes = append(last.notes, l)
		}
	}
	return out
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

// buildEntry reads one line into a title and a company. The formats in the wild
// are: "Title | Company | 2019", "Title, Company (2019)", "2019 - 2021 · Title
// · Company", "Title at Company, 2019". The date is taken out first, then the
// rest is split on the separators, and the longest part is the title because a
// company is usually the shorter of the two.
func (f *filler) buildEntry(line, start, end string) entry {
	e := entry{start: start, end: end}
	// Both halves of the period are removed before the line is split: leaving
	// the wording of an open end in place makes the " - " that surrounded the
	// dates look like a separator, and the word "Present" look like a company.
	rest := strings.TrimSpace(stripPeriod(line))
	if rest == "" {
		// A line that is only a period: the title is on the next line, which
		// the caller stores, and this entry waits for it.
		return e
	}
	rest = strings.Trim(strings.TrimSpace(rest), "|·•-–—,;:()[]")
	rest = strings.TrimSpace(rest)

	// "Title chez Company" and its English form, tried before the separators,
	// because the word is a stronger signal than a pipe.
	for _, sep := range companySplitters {
		if a, b, ok := strings.Cut(rest, sep); ok {
			if isTitleish(a) && isTitleish(b) {
				e.title, e.company = clean(a), clean(b)
				return e
			}
		}
	}

	parts := splitOnSeparators(rest)
	// "Acme, Paris | 2021 - 2024" is a company and a place, not a title and a
	// company, and the second part is the one that names a place. The only
	// reliable way to tell is the document itself: the contact block of a resume
	// states where the candidate is, and a part that repeats it is a location.
	if n := f.placeIndex(parts); n >= 0 {
		e.location = clean(parts[n])
		for i, p := range parts {
			if i != n {
				e.company = clean(p)
				break
			}
		}
		return e
	}
	switch len(parts) {
	case 0:
		e.title = clean(rest)
	case 1:
		e.title = clean(parts[0])
	default:
		// Two parts: the one that names a school or a degree is the school, the
		// other is the title. With two neutral parts, the first is the title.
		if hasWord(parts[0], schoolWords) || hasWord(parts[1], degreeWords) {
			e.title, e.company = clean(parts[1]), clean(parts[0])
			break
		}
		e.title, e.company = clean(parts[0]), clean(parts[1])
	}
	return e
}

var (
	sepRun = regexp.MustCompile(`\s*(?:\||·|•|—|–|\s+-\s+|\s*[,;]\s+|\s+/\s+|\s{2,})\s*`)
	// Trailing punctuation an extraction leaves on the last field of a line.
	trailing = regexp.MustCompile(`^[\s|·•\-–—,;:()\[\]]+|[\s|·•\-–—,;:()\[\]]+$`)
)

// splitOnSeparators breaks a line on the separators a resume uses between
// fields, and drops the parts that carry nothing.
func splitOnSeparators(s string) []string {
	var out []string
	for _, p := range sepRun.Split(s, -1) {
		p = clean(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// clean trims the punctuation around a field and collapses its spaces.
func clean(s string) string {
	return collapseSpaces(strings.TrimSpace(trailing.ReplaceAllString(strings.TrimSpace(s), "")))
}

func layoutFold(s string) string { return layout.Fold(strings.ToLower(s)) }

// cleanBullet turns a line of prose into a highlight: trimmed of its trailing
// stop, which a bullet never carries, and kept whole even when it is long,
// because a highlight is allowed to be a sentence.
func cleanBullet(s string) string {
	s = collapseSpaces(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	return strings.TrimSuffix(s, ".")
}

// The fields an entry can carry besides its title and its period.
const (
	fieldTeam  = "team"
	fieldStack = "stack"
)

// fieldLine reads a "Label: value" line into the field it names. Both languages
// are accepted whatever the resume is in, because a French CV that lists an
// English stack is common and the label costs nothing to recognise.
func fieldLine(line string) (label, value string, ok bool) {
	head, rest, cut := strings.Cut(line, ":")
	if !cut {
		return "", "", false
	}
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return "", "", false
	}
	switch layoutFold(strings.TrimSpace(head)) {
	case layoutFold(model.LangFR.Labels().Team), layoutFold(model.LangEN.Labels().Team):
		return fieldTeam, rest, true
	case layoutFold(model.LangFR.Labels().Technologies), layoutFold(model.LangEN.Labels().Technologies):
		return fieldStack, rest, true
	}
	return "", "", false
}

// proseLead matches the openings of a sentence that describes a role rather
// than claims a result. A highlight starts with a verb in the past — "Livré une
// API", "Reduced latency" — while a description sets a scene first.
var proseLead = regexp.MustCompile(`^(?:` +
	`au sein d|dans le cadre|en tant que|responsable de la|mission principale|` +
	`mon role|mes missions|contexte|l ?equipe|j ?ai |nous avons |` +
	`as part of|within a|in charge of|my role|the team|i was |we were |` +
	`working (?:as|with|on)|responsible for` +
	`)`)

// isProse reports whether a line reads as part of a description rather than as
// a highlight. The test is deliberately conservative: a line only counts as
// prose when it says so clearly, because the cost of the two mistakes is not
// the same. Reading a highlight as prose merges it into a paragraph, which is
// ugly but keeps every word; reading a paragraph as highlights cuts it into
// fragments that stop mid-clause, which is what the form used to show.
func isProse(line string) bool {
	s := strings.TrimSpace(line)
	if s == "" || bulletRe.MatchString(s) {
		return false
	}
	// A line long enough to wrap in the source is a paragraph. The threshold is
	// above the length of a written highlight and below that of a full line of
	// body text in a one column layout.
	if len([]rune(s)) > 120 {
		return true
	}
	if proseLead.MatchString(layoutFold(s)) {
		return true
	}
	// Two sentences on one line is prose by construction: a highlight is one
	// claim, and the mark that separates them is the evidence.
	if strings.Count(s, ". ") >= 1 && len([]rune(s)) > 60 {
		return true
	}
	return false
}

// continuesField reports whether a line is the tail of a field list that ran
// past the width of the page. The renderer breaks such a list on a separator,
// so the tail is short, carries no sentence and names no new field: "OpenAI,
// GCP," then "Elasticsearch". A line that reads as a sentence is not a tail,
// it is the next thing the entry says.
func continuesField(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" || bulletRe.MatchString(s) {
		return false
	}
	if _, _, ok := fieldLine(s); ok {
		return false
	}
	if hasDate(s) || len([]rune(s)) > 90 {
		return false
	}
	// A short line under a field is not automatically its tail: the title of
	// the next job is short too, and reading it as the end of the stack lost
	// the title of every post but the first. The caller decides, by asking
	// whether the field line it came after was actually cut mid-list.
	return true
}

// continuesSentence reports whether a line was broken by the page rather than
// ended by the writer. A line that stops without a terminal mark, and without
// the punctuation a list uses, is the first half of a sentence.
func continuesSentence(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	r := []rune(s)
	switch r[len(r)-1] {
	case '.', '!', '?', ':', ';', '•', ')', ']':
		return false
	}
	// Only a line that is long enough to have been wrapped: a short line that
	// happens to lack a full stop is a heading or a field, not half a sentence.
	return len(r) >= 40
}

// joinWrapped puts back together a sentence the page broke in two. A word split
// across the break carries a hyphen, which is dropped with the space; anything
// else is joined with one space.
func joinWrapped(prev, next string) string {
	prev = strings.TrimSpace(prev)
	next = strings.TrimSpace(next)
	if strings.HasSuffix(prev, "-") {
		return strings.TrimSuffix(prev, "-") + next
	}
	return prev + " " + next
}

// strayTitle reports whether a line that was filed as content is really the
// title of the entry that follows it.
//
// isTitleish carries the whole test. continuesSentence is deliberately not
// consulted: it calls any line of forty characters or more a wrapped sentence,
// and a degree like "Classe préparatoire aux grandes écoles, TSI Maths Sup et
// Spé" is sixty. Asking it here left every long title stuck in the description
// of the entry above it.
func strayTitle(s string) bool { return isTitleish(strings.TrimSpace(s)) }

// lastTitleLine returns the index of the last line of loose that reads as a
// title, or -1. A title is short and carries no mark of its own; a line that
// was marked, or that is long, is content and stays content.
func lastTitleLine(loose []string) int {
	for i := len(loose) - 1; i >= 0; i-- {
		l := strings.TrimSpace(loose[i])
		if l == "" || isDecorative(l) || bulletRe.MatchString(l) {
			continue
		}
		if len([]rune(l)) > 60 {
			return -1
		}
		return i
	}
	return -1
}
