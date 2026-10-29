package tex

import (
	"strconv"
	"strings"
)

// defaultAccent is the navy of the headings when -accent is not given.
const defaultAccent = "1F3864"

// leading is the line spacing of the document, the same 1.25 as the pdf: the
// body size is the reference of one typographic scale, and every size below
// carries its own leading so the rhythm survives a size change.
const leading = 1.25

// The vertical rhythm, in millimetres, copied from the pdf renderer so the
// two documents read alike.
const (
	spaceSection     = "6mm"   // before a section heading
	spaceFirstRule   = "1.5mm" // the hairline under the identity block
	spaceRuleHeading = "8mm"   // that hairline, then the first heading
	spaceHeadingBody = "2.2mm" // a heading and the block it announces
	spaceUnderRule   = "1.2mm" // a heading and the hairline under it
	spaceBetween     = "1.2mm" // a title and its dates, the dates and the body
	spaceEntry       = "3mm"   // between two entries of a section
	ruleThickness    = "0.25mm"
)

// hexColor reads an RRGGBB color for \definecolor, falling back when the
// value is not a color: a crafted accent must not reach the file, where it
// would fail the compile instead of the request.
func hexColor(s, fallback string) string {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) != 6 {
		return fallback
	}
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'f', r >= 'A' && r <= 'F':
		default:
			return fallback
		}
	}
	return strings.ToUpper(s)
}

// num formats a size without trailing zeros, so 10.5 is "10.5" and 15 is "15".
func num(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

// font is the \fontsize pair of one step of the scale: the body is the
// reference and the leading follows it, exactly like the pdf renderer.
func font(size float64) string {
	return `\fontsize{` + num(size) + `}{` + num(size*leading) + `}\selectfont`
}

// escape turns a piece of resume text into LaTeX source. Every character the
// language reads specially is rewritten, so a name like "Rakoto & fils_2"
// typesets instead of breaking the compile.
func escape(s string) string {
	if !strings.ContainsAny(s, `\{}#$%&~^_`) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\textbackslash{}`)
		case '{':
			b.WriteString(`\{`)
		case '}':
			b.WriteString(`\}`)
		case '#':
			b.WriteString(`\#`)
		case '$':
			b.WriteString(`\$`)
		case '%':
			b.WriteString(`\%`)
		case '&':
			b.WriteString(`\&`)
		case '_':
			b.WriteString(`\_`)
		case '~':
			b.WriteString(`\textasciitilde{}`)
		case '^':
			b.WriteString(`\textasciicircum{}`)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
