// Package txt renders a layout as plain text. It doubles as the debugging
// view of what an ATS parser sees once the layout is extracted.
package txt

import (
	"strings"

	"github.com/fanama/resume/atscv/internal/layout"
)

// Render writes the layout as fixed-width plain text.
func Render(l *layout.Layout) string {
	var b strings.Builder
	blank := 0 // consecutive newlines already written
	write := func(s string) {
		b.WriteString(s)
		blank = 0
		if strings.HasSuffix(s, "\n") {
			blank = 1
		}
	}
	for _, blk := range l.Blocks {
		switch blk.Kind {
		case layout.KindSummary:
			if blk.Text == "" {
				write("\n")
				continue
			}
			write(wrap(blk.Text, 92) + "\n")
		case layout.KindBullet:
			write("- " + wrap(blk.Text, 90) + "\n")
		case layout.KindField:
			if blk.Bold != "" {
				write(wrap(blk.Bold+": "+blk.Text, 92) + "\n")
			} else {
				write(wrap(blk.Text, 92) + "\n")
			}
		case layout.KindEntryTitle:
			line := blk.Text
			if blk.Dim != "" {
				line += " | " + blk.Dim
			}
			write(wrap(line, 92) + "\n")
		case layout.KindSection:
			// A heading always starts a fresh paragraph, with a blank line
			// above it unless the document just began.
			if b.Len() > 0 && blank == 0 {
				write("\n")
			}
			write(blk.Text + "\n")
		default:
			write(wrap(blk.Text, 92) + "\n")
		}
	}
	return b.String()
}

// wrap hard-wraps a paragraph on word boundaries.
func wrap(s string, width int) string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return ""
	}
	lines := make([]string, 0, len(words))
	current := words[0]
	for _, w := range words[1:] {
		if len([]rune(current))+1+len([]rune(w)) > width {
			lines = append(lines, current)
			current = w
			continue
		}
		current += " " + w
	}
	return strings.Join(append(lines, current), "\n")
}
