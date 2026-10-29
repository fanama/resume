package fill

import (
	"regexp"
	"strings"

	"github.com/fanama/resume/atscv/internal/model"
)

// The contact block is read over the whole document rather than from a fixed
// place, because its position is a layout decision and not a convention: header,
// footer, second column, or under the name. The three patterns below are the
// only contact fields that are recognisable by shape alone.

var (
	// Deliberately conservative: a local part with no dot, no space and no
	// underscore, because a stray "@" in prose is more common than an unusual
	// address, and a wrong email is a candidate who gets no mail.
	emailRe = regexp.MustCompile(`\b[A-Za-z0-9._%+\-]{1,64}@[A-Za-z0-9.\-]{1,255}\.[A-Za-z]{2,24}\b`)

	// Spaced or not, with the usual separators. The international prefix is
	// required to keep a year or a postal code out of the result.
	phoneRe = regexp.MustCompile(`(?:\+|00)\d[\d.\-\s()]{6,20}\d|(?:0[1-9](?:[\s.\-]?\d{2}){4})\b`)

	// A URL, or a bare host that names a person: linkedin.com/in/camille,
	// github.com/camille, camille.fr.
	urlRe = regexp.MustCompile(`(?i)\b(?:https?://|www\.)?[\w\-]+(?:\.[\w\-]+)+(?:/[\w\-./#?=&%+~:@]*)?`)
)

// readContact scans the document for the contact fields and returns the lines
// that still carry content, with the matched fragments taken out rather than the
// whole line dropped. A header packs a name, an email, a phone and a city on one
// line: dropping the line would lose the city, and dropping the fields would
// leave "|" hanging in the text.
func (f *filler) readContact(lines []string) []string {
	residual := make([]string, len(lines))
	for i, line := range lines {
		residual[i] = line
	}

	// Email and phone may sit anywhere, so the whole document is scanned for
	// them, and the first occurrence wins: a resume repeats its email in the
	// header and in the footer at most. The email goes first, because a phone
	// pattern would otherwise match the digits of an address.
	for i := range residual {
		if f.resume.Contact.Email == "" {
			if m := emailRe.FindString(residual[i]); m != "" {
				f.resume.Contact.Email = m
				residual[i] = strings.Replace(residual[i], m, " ", 1)
			}
		}
		if f.resume.Contact.Phone == "" {
			if m := phoneRe.FindString(residual[i]); m != "" {
				f.resume.Contact.Phone = tidyPhone(m)
				residual[i] = strings.Replace(residual[i], m, " ", 1)
			}
		}
	}
	f.readLinks(residual)
	// Only the header states where the author lives. A job line that names a
	// country states where the job was, and reading it as the author's own city
	// both invents a field and deletes the job's location from the line.
	f.readPlace(headerOf(residual))

	out := make([]string, 0, len(residual))
	for _, line := range residual {
		// What is left of a header line is separators once the fields are gone.
		if isSeparatorOnly(line) {
			continue
		}
		out = append(out, collapseSpaces(line))
	}
	return out
}

// readLinks takes the URLs out of the lines, keeping the rest.
func (f *filler) readLinks(residual []string) {
	seen := map[string]bool{}
	for i, line := range residual {
		for _, raw := range urlRe.FindAllString(line, -1) {
			u := tidyURL(raw)
			if u == "" || seen[u] {
				continue
			}
			// A domain alone counts as a link only when it looks personal.
			// Otherwise the employer named in a project line would become a
			// link in the contact block.
			if !strings.Contains(u, "/") && !isPersonalHost(u) {
				continue
			}
			seen[u] = true
			f.resume.Contact.Links = append(f.resume.Contact.Links, model.Link{
				Label: linkLabel(u),
				URL:   u,
			})
			residual[i] = strings.Replace(residual[i], raw, " ", 1)
		}
	}
}

// isSeparatorOnly reports whether a line holds nothing but the separators a
// header uses between its fields, once the fields themselves are gone.
func isSeparatorOnly(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return true
	}
	return strings.Trim(trimmed, "|·•-,;:/\t ") == ""
}

func isPersonalHost(u string) bool {
	host := hostOf(u)
	for _, h := range []string{"linkedin.com", "github.com", "gitlab.com", "behance.net", "dribbble.com", "mastodon.social", "medium.com", "x.com", "twitter.com"} {
		if strings.HasSuffix(host, h) {
			return true
		}
	}
	// A first-name domain such as camille.fr, or a portfolio on a free host.
	parts := strings.Split(host, ".")
	if len(parts) >= 2 && len(parts[0]) <= 3 && parts[0] != "www" {
		return true
	}
	return strings.HasSuffix(host, ".page") || strings.HasSuffix(host, ".dev") || strings.HasSuffix(host, ".me")
}

func hostOf(u string) string {
	u = strings.TrimPrefix(strings.TrimPrefix(u, "https://"), "http://")
	u = strings.TrimPrefix(u, "www.")
	if i := strings.IndexAny(u, "/?#"); i >= 0 {
		u = u[:i]
	}
	return strings.ToLower(u)
}

func linkLabel(u string) string {
	host := hostOf(u)
	switch {
	case strings.Contains(host, "linkedin"):
		return "LinkedIn"
	case strings.Contains(host, "github"):
		return "GitHub"
	case strings.Contains(host, "gitlab"):
		return "GitLab"
	case strings.Contains(host, "behance"):
		return "Behance"
	case strings.Contains(host, "dribbble"):
		return "Dribbble"
	case strings.Contains(host, "mastodon"):
		return "Mastodon"
	}
	return host
}

// tidyURL drops the trailing punctuation an extraction leaves on the last word
// of a line, and a scheme that was never there.
func tidyURL(raw string) string {
	u := strings.TrimSpace(raw)
	u = strings.TrimRight(u, ".,;:!?)]}'\"")
	if u == "" || !strings.Contains(u, ".") {
		return ""
	}
	host := hostOf(u)
	if host == "" || !strings.Contains(host, ".") {
		return ""
	}
	path := strings.TrimPrefix(strings.TrimPrefix(u, "https://"), "http://")
	if !strings.Contains(path, "://") && !strings.HasPrefix(strings.ToLower(raw), "www.") {
		// Bare host without a scheme: keep it as typed, it is still a link, and
		// the schema stores what the CV printed.
		return path
	}
	return path
}

func tidyPhone(raw string) string {
	var b strings.Builder
	for _, r := range raw {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '+' && b.Len() == 0:
			b.WriteRune(r)
		case r == ' ' || r == '\u00a0' || r == '.' || r == '-' || r == '(' || r == ')' || r == '/':
			// dropped: the printed form is what gets stored
		}
	}
	return strings.TrimSpace(b.String())
}
