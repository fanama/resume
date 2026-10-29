package fill

import "strings"

// consumeContent fills the open entry with one line that carries no period.
func (b *sectionBuilder) consumeContent(line string) {
	if b.consumeBullet(line) {
		return
	}
	if b.consumeField(line) {
		return
	}
	if b.consumeFieldTail(line) {
		return
	}
	b.current.openField = ""
	if b.consumeTitle(line) {
		return
	}
	if b.consumePendingTitle(line) {
		return
	}
	if b.consumeCompany(line) {
		return
	}
	b.consumeNote(line)
}

// consumeBullet files a marked line as an achievement.
func (b *sectionBuilder) consumeBullet(line string) bool {
	if !bulletRe.MatchString(line) {
		return false
	}
	stripped := bulletRe.ReplaceAllString(line, "")
	// cleanBullet, not cleanItem: cleanItem is the cleaner for a skill
	// list and throws away anything over 40 characters or six words,
	// which is every achievement a candidate ever wrote. Used here it
	// silently deleted the first line of every marked bullet.
	if bullet := cleanBullet(stripped); bullet != "" {
		b.current.bullets = append(b.current.bullets, bullet)
		// Whether the bullet wraps is judged on the line as printed,
		// not on the cleaned item: cleanItem removes the full stop
		// that ends it, which would make every finished bullet look
		// like it runs on and swallow the one after it.
		b.current.openBullet = continuesSentence(stripped)
	}
	return true
}

// consumeField files a "Équipe:" or "Technologies:" line into its field.
func (b *sectionBuilder) consumeField(line string) bool {
	label, value, ok := fieldLine(line)
	if !ok {
		return false
	}
	// "Équipe: ..." and "Technologies: ..." are fields, not achievements.
	// They are what this tool itself prints under an entry, so failing to
	// read them back means the app cannot reimport its own output: the
	// team and the stack came in as two more bullets at the end of the
	// list, and the fields they belong to stayed empty.
	switch label {
	case fieldTeam:
		b.current.team = value
	case fieldStack:
		b.current.stack = splitItems(value)
	}
	b.current.openBullet = false
	// The list is open only if this line stopped on a separator.
	b.current.openField = ""
	if strings.HasSuffix(strings.TrimSpace(line), ",") {
		b.current.openField = label
	}
	return true
}

// consumeFieldTail reads the unlabelled continuation of a wrapped field list.
func (b *sectionBuilder) consumeFieldTail(line string) bool {
	if b.current.openField == "" || !continuesField(line) {
		return false
	}
	// A stack long enough to wrap continues on an unlabelled line, and the
	// tail is a list item like the rest, not a highlight. The list is only
	// still open when the line before it was cut between two items, which
	// the trailing separator proves: "OpenAI, GCP," then "Elasticsearch".
	// Without that proof a short line here is the title of the next entry.
	switch b.current.openField {
	case fieldTeam:
		b.current.team = joinWrapped(b.current.team, line)
	case fieldStack:
		b.current.stack = append(b.current.stack, splitItems(line)...)
	}
	// The tail closes the list unless it too was cut on a separator.
	if !strings.HasSuffix(strings.TrimSpace(line), ",") {
		b.current.openField = ""
	}
	return true
}

// consumeTitle takes the first line under a date line as the title.
func (b *sectionBuilder) consumeTitle(line string) bool {
	// A title is a title, not merely the first line that turned up. The
	// entry whose date line named the company leaves the title empty, and
	// taking whatever follows made the opening line of the description the
	// job title and left the description empty: the line wraps, so it read
	// as "Éditeur d'une plateforme d'analyse en temps réel de flux vidéo
	// captés par des caméras en".
	if b.current.title != "" || !isTitleish(line) || continuesSentence(line) {
		return false
	}
	b.current.title = line
	return true
}

// consumePendingTitle holds a title line that announces the next entry.
func (b *sectionBuilder) consumePendingTitle(line string) bool {
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
	cur := b.current
	if cur.title == "" || cur.openBullet || !strayTitle(line) || continuesSentence(line) {
		return false
	}
	if len(cur.notes) > 0 && continuesSentence(cur.notes[len(cur.notes)-1]) {
		return false
	}
	b.pending = line
	return true
}

// consumeCompany files a short line as the company of the entry.
func (b *sectionBuilder) consumeCompany(line string) bool {
	if b.current.company != "" || !isTitleish(line) {
		return false
	}
	b.current.company = line
	return true
}

// consumeNote files what is left: a wrapped bullet, a wrapped note, a
// description, or an unmarked achievement.
func (b *sectionBuilder) consumeNote(line string) {
	cur := b.current
	// A line that continues something goes back onto whatever is open, and
	// the bullet is tested first: the half of a wrapped bullet is long and
	// stops mid-clause, so anything that reads the line on its own merits
	// would call it a paragraph and file it away from the list it belongs
	// to, leaving the bullet a fragment and the description a mess.
	if cur.openBullet && len(cur.bullets) > 0 {
		b.continueBullet(line)
		return
	}
	if len(cur.notes) > 0 && continuesSentence(cur.notes[len(cur.notes)-1]) {
		cur.notes[len(cur.notes)-1] = joinWrapped(cur.notes[len(cur.notes)-1], line)
		return
	}
	// The description opens the entry, before any bullet, and it is known
	// by the fact that it wraps: the page broke it mid-clause, so the line
	// does not end on a stop. An unmarked achievement, by contrast, is
	// written as one complete line. That single test is what separates the
	// two, and it is the only one that held for both of the real shapes:
	// the paragraph this tool's own renderer emits, and the run of three
	// bare achievement lines a plain template emits.
	if isProse(line) || (len(cur.bullets) == 0 && continuesSentence(line)) {
		b.appendNote(strings.TrimSpace(line))
		return
	}
	if len([]rune(line)) <= 200 {
		if bullet := cleanBullet(line); bullet != "" {
			b.appendHighlight(bullet, line)
			return
		}
	}
	b.appendNote(line)
}

// continueBullet joins the wrapped half of an achievement to its first line.
func (b *sectionBuilder) continueBullet(line string) {
	cur := b.current
	n := len(cur.bullets) - 1
	cur.bullets[n] = joinWrapped(cur.bullets[n], line)
	cur.openBullet = continuesSentence(line)
}

// appendHighlight keeps an unmarked achievement, capped per entry.
func (b *sectionBuilder) appendHighlight(bullet, printed string) {
	cur := b.current
	if len(cur.bullets) >= 8 {
		return
	}
	cur.bullets = append(cur.bullets, bullet)
	// An unmarked highlight wraps too, and its second line has no mark
	// either. Judged on the line as printed, for the same reason as above:
	// cleanBullet drops the full stop that proves the highlight was finished.
	cur.openBullet = continuesSentence(printed)
}

// appendNote keeps a line of description, capped per entry.
func (b *sectionBuilder) appendNote(line string) {
	cur := b.current
	if len(cur.notes) >= 8 {
		return
	}
	cur.notes = append(cur.notes, line)
}
