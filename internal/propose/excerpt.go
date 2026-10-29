package propose

import (
	"strings"

	"github.com/fanama/resume/atscv/internal/fill"
	"github.com/fanama/resume/atscv/internal/layout"
)

// lines splits the document the way the draft was built from it: one line each,
// trimmed, the empty ones gone. internal/fill does a heavier cleaning, because it
// has to match headings; this only has to find the same lines again.
func lines(text string) []string {
	var out []string
	for _, raw := range strings.Split(text, "\n") {
		line := strings.Join(strings.Fields(raw), " ")
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// headerBlock is the identity block: the lines before the first heading, capped
// so that a document written without a heading at all cannot hand the model a
// whole page. Reading a section heading as the headline is the failure this
// avoids — and the reason the heading test is borrowed from internal/fill rather
// than written again here.
func headerBlock(lines []string) []string {
	var out []string
	for _, line := range lines {
		if fill.IsHeading(line) || len(out) == headLines {
			break
		}
		out = append(out, line)
	}
	return out
}

// entryExcerpt returns the lines of one entry: the line that names it, decided
// by names, and the lines under it up to the next entry of the same section,
// decided by anchors — the values the deterministic pass read for the whole
// section.
//
// The window stops at the next entry rather than at a fixed length, because an
// entry with no description of its own must not be handed the description of the
// one below it. It returns "" when there is nothing but the naming line: a field
// with no lines to read from is a field that has to stay empty, and a model asked
// about nothing answers with something.
func entryExcerpt(lines []string, names, anchors []string) string {
	start := entryStart(lines, names)
	if start < 0 {
		return ""
	}
	end := start + 1
	for end < len(lines) && end-start <= entryLines {
		// The window ends at the next entry — a line the section's anchors
		// already know, or the shape every entry shares, a period on the naming
		// line — and at the next section heading, which the anchors know nothing
		// about: without it the last entry of a section would be handed the
		// skills and the languages that follow it.
		if containsAny(lines[end], anchors) || isEntryAnchor(lines[end]) || fill.IsHeading(lines[end]) {
			break
		}
		end++
	}
	if end == start+1 {
		return ""
	}
	// The naming line is left out on purpose. Its values -- the title, the
	// company, the place, the dates -- are the ones the deterministic pass has
	// already read, and a model offered the line it is not being asked about
	// answers with it: asked for the mission sentence of a job, both entries came
	// back as their own title line, dates included.
	return strings.Join(lines[start+1:end], "\n")
}

// isEntryAnchor reports whether a line starts a new entry rather than continuing
// the one above it. An entry opens with a line that carries a period — a job, a
// diploma, a project all state when they happened — and the shape is the only
// signal available once the page has been flattened.
func isEntryAnchor(line string) bool {
	return strings.Contains(line, " - ") && hasYear(line) ||
		strings.Contains(line, " – ") && hasYear(line) ||
		strings.Contains(line, "|") && hasYear(line)
}

// hasYear is deliberately loose: it decides where a window ends, and a wrong
// guess costs one line of context rather than a wrong value.
func hasYear(line string) bool {
	for _, field := range strings.Fields(line) {
		digits := 0
		for _, r := range field {
			if r >= '0' && r <= '9' {
				digits++
			}
		}
		if digits == 4 {
			return true
		}
	}
	return false
}

// entryStart finds the line that opens an entry.
//
// The first line naming the entry is not enough, and this is not a theory: on a
// CV whose headline is "Développeuse back-end" and whose second job is also a
// "Développeuse back-end", the loose search started at the top of the page and
// handed the model the contact line as the job's mission. An entry line states at
// least two of the things that name it -- the title and the employer, or one of
// them and a period -- so a second pass keeps the loose match only as a fallback.
func entryStart(lines []string, names []string) int {
	for i, line := range lines {
		if containsAny(line, names) && (countAny(line, names) >= 2 || hasYear(line)) {
			return i
		}
	}
	for i, line := range lines {
		if containsAny(line, names) {
			return i
		}
	}
	return -1
}

// countAny counts how many of the anchors the line states.
func countAny(line string, anchors []string) int {
	folded := fold(line)
	seen := 0
	for _, a := range anchors {
		if a != "" && strings.Contains(folded, fold(a)) {
			seen++
		}
	}
	return seen
}

// containsAny reports whether the line states one of the anchors, compared the
// way a heading is compared: an extraction damages accents and case first.
func containsAny(line string, anchors []string) bool {
	folded := fold(line)
	for _, a := range anchors {
		if a == "" {
			continue
		}
		if strings.Contains(folded, fold(a)) {
			return true
		}
	}
	return false
}

func fold(s string) string {
	return layout.Fold(strings.ToLower(strings.TrimSpace(s)))
}
