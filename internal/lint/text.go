package lint

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/fanama/resume/atscv/internal/layout"
)

// checkText looks for the characters and shapes that break a text extraction.
func (c *checker) checkText() {
	for _, b := range c.layout.Blocks {
		where := blockWhere(b)
		text := strings.Join([]string{b.Text, b.Bold, b.Dim}, " ")
		if m := glyphRe.FindString(text); m != "" {
			c.add(Warn, "glyphs", where,
				fmt.Sprintf("the line holds the decorative glyph %q", m),
				"use a plain dash or nothing at all: a glyph is dropped or turned into a box")
		}
		if emojiRe.MatchString(text) {
			c.add(Error, "emoji", where, "the line holds an emoji",
				"an emoji has no glyph in the PDF core fonts, it becomes a black box")
		}
		if strings.Contains(text, "\t") {
			c.add(Warn, "tabs", where, "the line holds a tabulation",
				"a tab is a column for a parser, use spaces")
		}
		if b.Kind == layout.KindBullet {
			words := strings.Fields(b.Text)
			if len(words) == 0 {
				continue
			}
			if articles[strings.ToLower(strings.Trim(words[0], ".,;:!?\"'()"))] {
				c.add(Info, "bullet.verb", where,
					fmt.Sprintf("the line starts with the article %q", words[0]),
					"start with a verb in the imperative or the past tense: \"Reduced\", \"Deployed\"")
			}
			if n := utf8.RuneCountInString(b.Text); n > 240 {
				c.add(Info, "bullet.length", where,
					fmt.Sprintf("the line is %d characters long", n),
					"aim for a single line of about 120 characters")
			}
		}
	}
}

func blockWhere(b layout.Block) string {
	switch b.Kind {
	case layout.KindName:
		return "header"
	case layout.KindHeadline, layout.KindContact:
		return "header"
	case layout.KindSection:
		return "section " + b.Text
	default:
		if b.Text == "" {
			return "body"
		}
		head := b.Text
		if len([]rune(head)) > 28 {
			head = string([]rune(head)[:28]) + "…"
		}
		return "body " + head
	}
}

// computeStats fills the metrics of the report.
func (c *checker) computeStats() {
	s := &c.rep.Stats
	raw := c.layoutText()
	s.Chars = len([]rune(raw))
	s.Words = len(wordRe.FindAllString(raw, -1))
	c.countBlocks(s)
	for _, g := range c.resume.Skills {
		s.Skills += len(g.Items)
	}
	// One A4 page holds about 3 400 characters of body text at 10.5 pt.
	s.Pages = (s.Chars + 3399) / 3400
	if s.Pages == 0 {
		s.Pages = 1
	}
	s.ATSReadability = c.readability(s)
}

// layoutText is every block of the layout, text and label together.
func (c *checker) layoutText() string {
	var text strings.Builder
	for _, b := range c.layout.Blocks {
		text.WriteString(b.Text)
		text.WriteString(" ")
		text.WriteString(b.Bold)
		text.WriteString(" ")
	}
	return text.String()
}

// countBlocks counts the achievement lines, how many carry a figure, and the
// entries.
func (c *checker) countBlocks(s *Stats) {
	for _, b := range c.layout.Blocks {
		switch b.Kind {
		case layout.KindBullet:
			s.Bullets++
			if metricRe.MatchString(b.Text) {
				s.Metrics++
			}
		case layout.KindEntryTitle:
			s.Entries++
		}
	}
}

// readability starts at 100 and subtracts the number of quantified lines, then
// the length. The result is a comparable number between two versions of the same
// resume, not an absolute grade.
func (c *checker) readability(s *Stats) int {
	score := c.rep.Score()
	if s.Bullets > 0 {
		ratio := float64(s.Metrics) / float64(s.Bullets)
		score -= int(ratio * 30)
	}
	if s.Pages > 2 {
		score -= (s.Pages - 2) * 10
	}
	if s.Words < 200 {
		score -= 10
	}
	if score < 0 {
		score = 0
	}
	return score
}
