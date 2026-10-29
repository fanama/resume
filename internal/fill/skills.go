package fill

import (
	"strings"

	"github.com/fanama/resume/atscv/internal/model"
)

// readSkills fills the skills section. A line of the form "Languages: Go, Rust"
// becomes a group with a category; lines that name no category are collected
// into one group instead of one group each.
//
// The grouping is what makes the section readable. A skills block is usually a
// handful of comma separated lines with no category at all, and appending a
// group per line produced a form with a dozen cards all headed "Compétences" —
// the same heading repeated is not a classification, it is the absence of one.
// A PDF also wraps a long line, so consecutive uncategorised lines are often
// one list that the page broke in two, and joining them puts it back together.
func (f *filler) readSkills(s section) {
	// loose holds the items from every line that named no category. They land
	// in a single group at the position of the first such line, so the order of
	// the section is kept.
	var loose []string
	looseAt := -1

	for _, line := range s.lines {
		items := splitItems(line)
		if len(items) == 0 {
			continue
		}
		category, items := skillCategory(line, items)
		if len(items) == 0 {
			continue
		}
		if category == "" {
			if looseAt < 0 {
				looseAt = len(f.resume.Skills)
				// The slot is reserved now and filled once every line is read,
				// so that a category stated after a bare list keeps its place.
				f.resume.Skills = append(f.resume.Skills, model.SkillGroup{})
			}
			loose = append(loose, items...)
			continue
		}
		f.addSkillGroup(category, items)
	}

	if looseAt >= 0 {
		f.resume.Skills[looseAt] = model.SkillGroup{
			Category: skillGroupLabel(f.lang),
			Items:    appendUnique(nil, loose),
		}
	}
	if len(f.resume.Skills) == 0 {
		f.noteUnmapped(s)
	}
}

// addSkillGroup appends a categorised group, joining one that already carries
// the name.
func (f *filler) addSkillGroup(category string, items []string) {
	// A category can be stated twice, once per line, when the extraction
	// split it. The items join the group that already carries the name
	// rather than opening a second one under the same heading.
	if i := indexOfCategory(f.resume.Skills, category); i >= 0 {
		f.resume.Skills[i].Items = appendUnique(f.resume.Skills[i].Items, items)
		return
	}
	f.resume.Skills = append(f.resume.Skills, model.SkillGroup{Category: category, Items: items})
}

// skillCategory reads the optional category of a skill line and the items that
// follow it. "Category: a, b, c" counts only when the part before the colon is
// short enough to be a category and not a sentence; "Languages - Go, Rust" is
// the same shape with a dash.
func skillCategory(line string, items []string) (string, []string) {
	if head, rest, ok := strings.Cut(line, ":"); ok && len(rest) > 0 {
		if len([]rune(strings.TrimSpace(head))) <= 40 && !hasDate(head) {
			return strings.TrimSpace(head), splitItems(rest)
		}
	}
	for _, sep := range []string{" - ", " – ", " — "} {
		if head, rest, ok := strings.Cut(line, sep); ok {
			if len([]rune(strings.TrimSpace(head))) <= 40 && len(splitItems(rest)) > 1 {
				return strings.TrimSpace(head), splitItems(rest)
			}
		}
	}
	return "", items
}

// indexOfCategory finds a group already headed by name, compared the way a
// heading is compared: case and accents are what an extraction damages first.
func indexOfCategory(groups []model.SkillGroup, name string) int {
	key := foldKey(name)
	for i := range groups {
		if groups[i].Category != "" && foldKey(groups[i].Category) == key {
			return i
		}
	}
	return -1
}

// appendUnique adds the items that are not already in the list. A skill named
// twice is a duplicate the candidate would have to delete by hand, and the
// wrapping of a PDF line is a common way to produce one.
func appendUnique(dst []string, items []string) []string {
	seen := make(map[string]bool, len(dst)+len(items))
	for _, s := range dst {
		seen[foldKey(s)] = true
	}
	for _, s := range items {
		k := foldKey(s)
		if k == "" || seen[k] {
			continue
		}
		seen[k] = true
		dst = append(dst, s)
	}
	return dst
}

func skillGroupLabel(lang model.Lang) string {
	if lang == model.LangEN {
		return "Skills"
	}
	return "Compétences"
}

// splitItems breaks a skill line into items, dropping a leading category and
// the noise an extraction leaves on the last one.
func splitItems(line string) []string {
	line = strings.TrimSpace(bulletRe.ReplaceAllString(line, ""))
	if line == "" {
		return nil
	}
	parts := skillSepRe.Split(line, -1)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = cleanItem(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

// cleanItem drops the punctuation an extraction leaves around an item and
// rejects what cannot be a skill: a date, a very long phrase, a line of prose.
func cleanItem(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, " \t•·|-–—,;:/")
	s = strings.TrimSpace(s)
	if s == "" || len([]rune(s)) > 40 {
		return ""
	}
	if hasDate(s) {
		return ""
	}
	// A phrase of six words or more is a sentence, not a skill.
	if len(strings.Fields(s)) > 6 {
		return ""
	}
	return s
}
