package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"syscall"
	"time"
)

// The job endpoint answers a fetch the analysis panel makes: it downloads a
// job-offer page server side, extracts its readable text, and hands it back so
// the panel can keep it beside the resume. The resume never rides along — only
// the URL leaves the browser — so fetching an offer is not a leak of the
// draft. It has to go through the server because a browser would stop at the
// CORS wall of every job board it tried to read directly.
//
// The endpoint is also the widest door the server has ever opened: it issues
// requests to an address of the requester's choosing. The guards below, in
// order: a URL that parses and says http or https, a host that resolves to
// addresses that are not local, a dialer that re-checks the address it is
// actually connecting to (so a redirect to an inner IP is refused at the
// socket), one hard size limit, and a content type that is HTML or plain text.

// maxOfferBody caps what a job offer may weigh. An offer page is text; a
// multi-megabyte answer is something else, and a draft the panel compares
// against has no use for it.
const maxOfferBody = 2 << 20

// maxOfferText caps the text actually kept and stored: the model that will
// compare the resume to the offer reads thousands of tokens, not megabytes.
const maxOfferText = 20000

type jobRequest struct {
	URL string `json:"url"`
}

// jobResponse is what the panel stores, and exactly what it stores: a local
// copy of the offer stays beside the draft, never leaves the browser again,
// and carries the address it came from and the moment it was read.
type jobResponse struct {
	URL       string `json:"url"`
	Title     string `json:"title"`
	Text      string `json:"text"`
	FetchedAt string `json:"fetchedAt"`
}

// handleJob answers POST /api/job. The request is a JSON body {"url": "…"} —
// or the "url" form field, so a curl keeps working the way the rest of the API
// reads.
func (s *Server) handleJob(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	var req jobRequest
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: fmt.Sprintf("read body: %v", err)})
			return
		}
		if len(bytes.TrimSpace(body)) > 0 {
			if err := json.Unmarshal(body, &req); err != nil {
				writeJSON(w, http.StatusBadRequest, errorResponse{Error: `bad JSON: the body must be {"url": "…"}`})
				return
			}
		}
	} else {
		if err := r.ParseForm(); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: fmt.Sprintf("read form: %v", err)})
			return
		}
		req.URL = r.PostFormValue("url")
	}
	req.URL = strings.TrimSpace(req.URL)
	if req.URL == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: `no URL in the request: send {"url": "…"}`})
		return
	}
	offer, err := fetchOffer(req.URL)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, offer)
}

// jobAllow refuses the targets no local server should dial: a loopback
// address, a private one, a link-local one. One guard at all levels — the URL
// check and the dialer's check both read it — so a redirect to an inner IP is
// refused even where it never calls fetchOffer again. The variable exists so
// the tests can point the fetcher at an httptest server, which itself listens
// on loopback.
var jobAllow = defaultAllow

// defaultAllow resolves the host and refuses it when any address it stands
// for is local. A name that only ever resolved outward passes; one that
// resolves inward, however it is spelled, does not.
func defaultAllow(host string) error {
	if ip := net.ParseIP(host); ip != nil {
		return guardIP(ip)
	}
	addrs, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("l'hôte %q n'a pas pu être résolu : %v", host, err)
	}
	if len(addrs) == 0 {
		return fmt.Errorf("l'hôte %q ne résout vers aucune adresse", host)
	}
	for _, ip := range addrs {
		if err := guardIP(ip); err != nil {
			return err
		}
	}
	return nil
}

// guardIP is the rule one address must satisfy: not loopback, not a private
// network, not link-local, not unspecified. Anything the fetcher could reach
// in a container, a VM or the machine itself is on one of these.
func guardIP(ip net.IP) error {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return fmt.Errorf("URL refusée : l'hôte %s est une adresse locale, pas une page d'offre", ip)
	}
	return nil
}

// jobTransport is an ordinary transport whose dialer has one rule: whatever
// address it is about to connect to, resolved here and now, must pass the
// same guard as the host in the URL. A redirect to an inner IP never gets to
// open its socket — dialing, not parsing, is where the refusal happens.
func jobTransport() *http.Transport {
	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			host, _, splitErr := net.SplitHostPort(address)
			if splitErr != nil {
				host = address
			}
			return jobAllow(strings.Trim(host, "[]"))
		},
	}
	return &http.Transport{
		DialContext:         dialer.DialContext,
		DisableKeepAlives:   true,
		TLSHandshakeTimeout: 10 * time.Second,
	}
}

// jobUserAgent is what the fetcher presents itself as. Job boards refuse the
// requests of a library with a bare "Go-http-client" identity: the page is
// public, only its reader is not a browser. Naming a browser keeps the read
// honest — no cookies, no session, just the HTML every visitor gets.
const jobUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) " +
	"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0 Safari/537.36"

// fetchOfferPage issues one GET with the identity and the acceptance a
// browser asks with. The dialer's Control already refuses any inner address
// the redirect chain could land on.
func fetchOfferPage(client *http.Client, u *url.URL) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("URL invalide : %v", err)
	}
	req.Header.Set("User-Agent", jobUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,*/*;q=0.8")
	req.Header.Set("Accept-Language", "fr,en;q=0.8")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("la page n'a pas pu être atteinte : %v", err)
	}
	return resp, nil
}

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

// fetchOffer downloads the page and answers the readable text. Every error is
// one sentence the panel can show; none of them carries the draft, because no
// draft is involved.
func fetchOffer(raw string) (jobResponse, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return jobResponse{}, fmt.Errorf("URL invalide : %q", raw)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return jobResponse{}, fmt.Errorf("URL refusée : le schéma %q n'est pas http ou https", u.Scheme)
	}
	if u.Hostname() == "" {
		return jobResponse{}, fmt.Errorf("URL refusée : pas d'hôte dans %q", raw)
	}
	if err := jobAllow(u.Hostname()); err != nil {
		return jobResponse{}, err
	}
	client := &http.Client{
		Timeout:   30 * time.Second,
		Jar:       nil, // an offer page is read, not visited: no cookies are kept
		Transport: jobTransport(),
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("trop de redirections : au moins 5 pour atteindre la page")
			}
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return fmt.Errorf("redirection vers le schéma %q refusée", req.URL.Scheme)
			}
			return nil
		},
	}
	// The public twin is the shorter road. Some boards — LinkedIn in
	// practice — refuse their search and landing pages outright from a client
	// that does not run their own code (a bare 999, no redirect, no wall,
	// nothing to follow), while the very same posting reads fine as
	// /jobs/view/<id>. When the asked URL carries a posting id and names a
	// board with a public twin, go to the twin first: the search page never
	// had anything to give the fetcher.
	if public := jobFallback(u); public != nil && public.String() != u.String() {
		u = public
	}
	resp, err := fetchOfferPage(client, u)
	if err != nil {
		return jobResponse{}, err
	}
	defer resp.Body.Close()
	// Some boards serve their search or their landing pages only behind an
	// account, while the offer itself is public. When the page we landed on
	// is a login wall and the asked URL names a posting, swap to that public
	// page — once, and only for the boards where the public twin is known to
	// exist. The first body is closed before the second target is asked.
	if behindLoginWall(resp.Request.URL) {
		fallback := jobFallback(u)
		if fallback == nil || fallback.String() == finalURL(resp) {
			return jobResponse{}, fmt.Errorf(
				"cette page demande une connexion (mur %q) : essayez l'URL publique de l'offre, du type /jobs/view/…",
				resp.Request.URL.Host)
		}
		resp.Body.Close()
		second, err := fetchOfferPage(client, fallback)
		if err != nil {
			return jobResponse{}, err
		}
		resp = second
		if behindLoginWall(resp.Request.URL) {
			return jobResponse{}, fmt.Errorf(
				"la page publique de l'offre (hôte %q) est restée derrière un mur de connexion",
				resp.Request.URL.Host)
		}
	}
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == 999 {
			return jobResponse{}, fmt.Errorf(
				"la page refuse les lecteurs extérieurs (999) : essayez l'URL publique de l'offre, du type /jobs/view/…")
		}
		return jobResponse{}, fmt.Errorf("la page a répondu %d %s : ce n'est pas une offre lisible",
			resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	media := strings.ToLower(strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0]))
	if media != "text/html" && media != "text/plain" {
		return jobResponse{}, fmt.Errorf("la page n'est pas lisible comme offre : type %q, want HTML ou texte",
			resp.Header.Get("Content-Type"))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxOfferBody+1))
	if err != nil {
		return jobResponse{}, fmt.Errorf("read page: %v", err)
	}
	if len(body) > maxOfferBody {
		return jobResponse{}, fmt.Errorf("la page dépasse %d octets : ce n'est pas une offre", maxOfferBody)
	}
	if media == "text/plain" {
		text := strings.TrimSpace(string(body))
		if text == "" {
			return jobResponse{}, fmt.Errorf("la page est vide")
		}
		return jobResponse{
			URL:       finalURL(resp),
			Title:     firstLine(text),
			Text:      capText(text),
			FetchedAt: time.Now().UTC().Format(time.RFC3339),
		}, nil
	}
	title, text := extractOfferText(string(body))
	if text == "" {
		return jobResponse{}, fmt.Errorf("aucun texte d'offre n'a pu être extrait de la page")
	}
	return jobResponse{
		URL:       finalURL(resp),
		Title:     title,
		Text:      capText(text),
		FetchedAt: time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// finalURL is where the fetcher actually landed, after every redirect: that is
// the address worth keeping, not the one that was asked for and may have been
// a shortener.
func finalURL(resp *http.Response) string {
	if resp.Request != nil && resp.Request.URL != nil {
		return resp.Request.URL.String()
	}
	return ""
}

// capText trims the extracted text to what the panel stores.
func capText(s string) string {
	runes := []rune(strings.TrimSpace(s))
	if len(runes) <= maxOfferText {
		return string(runes)
	}
	return string(runes[:maxOfferText]) + "…"
}

// firstLine is the first non-empty line, as a plain-text fallback for the
// title of an offer that arrives as text/plain.
func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			return t
		}
	}
	return ""
}
