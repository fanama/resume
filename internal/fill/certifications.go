package fill

import (
	"regexp"
	"strings"

	"github.com/fanama/resume/atscv/internal/model"
)

// readCertifications fills the certifications section. A line is an
// organisation and a date, which is most of what the schema holds.
func (f *filler) readCertifications(s section) {
	for _, line := range s.lines {
		name, issuer, date := parseCertification(line)
		if name == "" {
			continue
		}
		f.resume.Certifications = append(f.resume.Certifications, model.Certification{
			Name:   name,
			Issuer: issuer,
			Date:   date,
		})
	}
	if len(f.resume.Certifications) == 0 {
		f.noteUnmapped(s)
	}
}

// parseCertification reads "Name, Issuer, 2020" and its variants. The issuer is
// only what sits after a comma, never a guess: a certification without an
// issuer is incomplete but honest.
func parseCertification(line string) (name, issuer, date string) {
	s := strings.TrimSpace(bulletRe.ReplaceAllString(line, ""))
	if s == "" {
		return "", "", ""
	}
	if d, _, ok := splitPeriod(s); ok {
		date = d
		s = strings.TrimSpace(stripPeriod(s))
	}
	parts := strings.Split(s, ",")
	name = cleanItem(parts[0])
	if len(parts) > 1 {
		issuer = cleanItem(parts[1])
	}
	if name == "" || len([]rune(name)) > 60 {
		return "", "", ""
	}
	return name, issuer, date
}

// stripPeriod removes everything a period is made of, leaving the text around
// it: the dates, the words that say the period is still open, and the connector
// between a start and an end. All three have to go together, or the " - " that
// surrounded the dates is left behind and reads as a field separator.
func stripPeriod(s string) string {
	// Decided before anything is removed: once the dates are gone there is
	// nothing left to tell a connector from an ordinary word.
	hadDate := len(dateSpans(s)) > 0
	for _, span := range dateSpans(s) {
		if i := strings.Index(s, span); i >= 0 {
			s = s[:i] + " " + s[i+len(span):]
		}
	}
	// The connectors are only removed once a date was found, so that a word
	// like "to" inside a job title survives.
	if hadDate {
		s = periodConnector.ReplaceAllString(s, " ")
	}
	return strings.TrimSpace(strings.Join(strings.Fields(s), " "))
}

// periodConnector matches the words that join a start to an end, in both
// languages, including the open ended wording.
var periodConnector = regexp.MustCompile(`(?i)\b(?:de|a|à|au|from|to|until|through|depuis|jusqu|aujourd'hui|aujourdhui|present|presentement?|en cours|actuel|actuelle|current|now|ongoing)\b`)

// description joins a note block into the paragraph the schema holds.
//
// It used to return the first line and drop the rest, which lost the body of
// every description longer than one line: the candidate wrote three sentences
// about a job and the form showed one. The block is the description, so the
// block is what is kept, with the lines the page broke rejoined.
func description(lines []string) string {
	var out []string
	for _, l := range lines {
		l = strings.TrimSpace(bulletRe.ReplaceAllString(l, ""))
		if l == "" {
			continue
		}
		// A line the previous one runs into is the rest of that sentence.
		if n := len(out); n > 0 && continuesSentence(out[n-1]) {
			out[n-1] = joinWrapped(out[n-1], l)
			continue
		}
		out = append(out, l)
	}
	return strings.Join(out, " ")
}
