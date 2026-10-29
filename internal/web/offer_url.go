package web

import (
	"net/url"
	"regexp"
	"strings"
)

// behindLoginWall reads where the fetch actually ended: a page a board serves
// only to its members carries the request to sign in, and its text is not an
// offer. The paths are the shapes the big boards write their walls under.
func behindLoginWall(u *url.URL) bool {
	if u == nil {
		return false
	}
	p := u.Path
	return strings.Contains(p, "/uas/login") || strings.Contains(p, "/authwall") ||
		strings.Contains(p, "/checkpoint") || p == "/login" ||
		strings.Contains(p, "/access-signin")
}

// jobFallback derives the public twin of a posting behind a login wall.
// Swappable for the tests: a real board is named in the function itself, and
// a test points it at its own server.
var jobFallback = linkedinPublicOffer

// linkedinPublicOffer finds the one id a job-board URL carries and answers
// the page that, without an account, still reads like a posting. The search
// and the company pages put the posting in currentJobId or in
// originToLandingJobPostings; an already public /jobs/view URL carries the id
// in its path. Only the public twin is answered, never the private API a
// member session would open.
func linkedinPublicOffer(u *url.URL) *url.URL {
	if u == nil {
		return nil
	}
	host := strings.TrimPrefix(u.Hostname(), "www.")
	if host != "linkedin.com" {
		return nil
	}
	id := jobIDFromQuery(u)
	if id == "" {
		return nil
	}
	return &url.URL{Scheme: "https", Host: "www.linkedin.com", Path: "/jobs/view/" + id}
}

// jobIDFromQuery digs the posting id out of the query parameters the boards
// write, in the asked URL or in the login redirect that quotes it back.
func jobIDFromQuery(u *url.URL) string {
	wanted := regexp.MustCompile(`^\d+$`)
	for _, source := range []*url.URL{u, redirectTarget(u)} {
		if source == nil {
			continue
		}
		q := source.Query()
		if wanted.MatchString(q.Get("currentJobId")) {
			return q.Get("currentJobId")
		}
		for _, v := range q["originToLandingJobPostings"] {
			if wanted.MatchString(v) {
				return v
			}
		}
	}
	return ""
}

// redirectTarget is the page the login wall quotes it is coming back to, if
// the URL carries one: session_redirect=%2Fjobs%2F…
func redirectTarget(u *url.URL) *url.URL {
	target := u.Query().Get("session_redirect")
	if target == "" {
		return nil
	}
	parsed, err := url.Parse(target)
	if err != nil {
		return nil
	}
	return parsed
}
