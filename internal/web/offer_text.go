package web

import (
	"html"
	"regexp"
	"strings"
)

// extractOfferText turns a fetched HTML page into the text a panel can keep:
// the <title>, and the body without its machinery. Job boards are not well
// formed — half of them carry their offer between scripts and measurement
// pixels — so the extractor reads the page the crude way a plain-text reader
// would: it drops the blocks that carry no offer, breaks the structure where
// the markup says a line ends, then squeezes out the whitespace. Everything
// survives as plain text or not at all: an entity is decoded once, and no
// tag, attribute or script residue reaches the store.
var (
	offerTitleTag = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	// Go's regexp has no backreference, so the dropped blocks are one pattern
	// per tag, built once: a <script> closes on </script>, not on whatever
	// comes first.
	offerDropped = []*regexp.Regexp{
		blockPattern("script"), blockPattern("style"), blockPattern("noscript"),
		blockPattern("svg"), blockPattern("template"), blockPattern("iframe"),
		// A job board is a page around the offer: its chrome never carries a
		// duty, a stack or a salary, only links and boilerplate.
		blockPattern("nav"), blockPattern("footer"),
	}
	offerComments   = regexp.MustCompile(`(?s)<!--.*?-->`)
	offerHead       = regexp.MustCompile(`(?i)</head\s*>`)
	offerBreaks     = regexp.MustCompile(`(?is)<br\s*/?>`)
	offerLineEnds   = regexp.MustCompile(`(?i)</(p|div|h[1-6]|tr|section|article|header|footer|table|blockquote|option)\s*>`)
	offerListMarks  = regexp.MustCompile(`(?i)<(li|td|th)(\s[^>]*)?>`)
	offerAnyTag     = regexp.MustCompile(`(?s)<[^>]*>`)
	offerSpaces     = regexp.MustCompile(`[ \t\r\f\v]+`)
	offerBlankLines = regexp.MustCompile(`\n{3,}`)
)

// blockPattern drops a whole element whose text an offer never reads: the
// opening tag, the text until its own close, and the close itself.
func blockPattern(tag string) *regexp.Regexp {
	return regexp.MustCompile(`(?is)<` + tag + `[^>]*>.*?</` + tag + `\s*>`)
}

// extractOfferText answers the title of the page and its readable body. A
// page without a <title> answers an empty title, and the first line of the
// body stays where it is: the panel shows what it got, nothing invented.
func extractOfferText(page string) (string, string) {
	title := ""
	if m := offerTitleTag.FindStringSubmatch(page); m != nil {
		title = strings.TrimSpace(html.UnescapeString(m[1]))
	}
	// The head carries nothing an offer reads from: styles, metas, links. Cut
	// everything up to its close in one jump, then drop the blocks whose whole
	// text is machinery.
	if headEnd := offerHead.FindStringIndex(page); headEnd != nil {
		page = page[headEnd[1]:]
	}
	for _, drop := range offerDropped {
		page = drop.ReplaceAllString(page, " ")
	}
	page = offerComments.ReplaceAllString(page, " ")
	// Structure: where the markup says a block or a line ends, the text gets a
	// newline; a list or table cell starts on its own short line.
	page = offerBreaks.ReplaceAllString(page, "\n")
	page = offerLineEnds.ReplaceAllString(page, "\n")
	page = offerListMarks.ReplaceAllString(page, "\n• ")
	page = offerAnyTag.ReplaceAllString(page, "")
	page = html.UnescapeString(page)
	lines := strings.Split(page, "\n")
	for i, line := range lines {
		line = strings.TrimSpace(offerSpaces.ReplaceAllString(line, " "))
		lines[i] = line
	}
	text := offerBlankLines.ReplaceAllString(strings.Join(lines, "\n"), "\n\n")
	return title, strings.TrimSpace(text)
}
