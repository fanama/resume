package web

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// fetchOffer downloads the page and answers the readable text. Every error is
// one sentence the panel can show; none of them carries the draft, because no
// draft is involved.
func fetchOffer(raw string) (jobResponse, error) {
	u, err := offerURL(raw)
	if err != nil {
		return jobResponse{}, err
	}
	client := offerClient()
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
	resp, err = followLoginWall(client, resp, u)
	if err != nil {
		return jobResponse{}, err
	}
	defer resp.Body.Close()
	return readOffer(resp)
}

// offerURL parses and guards the address the request named.
func offerURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("URL invalide : %q", raw)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("URL refusée : le schéma %q n'est pas http ou https", u.Scheme)
	}
	if u.Hostname() == "" {
		return nil, fmt.Errorf("URL refusée : pas d'hôte dans %q", raw)
	}
	if err := jobAllow(u.Hostname()); err != nil {
		return nil, err
	}
	return u, nil
}

// offerClient is the HTTP client of a fetch: no cookies, a bounded redirect
// chain, and the guarded transport.
func offerClient() *http.Client {
	return &http.Client{
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
}

// followLoginWall swaps a login wall for the public twin of the posting, once.
// It closes the first body when it swaps; the caller owns whichever response
// comes back.
func followLoginWall(client *http.Client, resp *http.Response, asked *url.URL) (*http.Response, error) {
	if !behindLoginWall(resp.Request.URL) {
		return resp, nil
	}
	fallback := jobFallback(asked)
	if fallback == nil || fallback.String() == finalURL(resp) {
		return nil, fmt.Errorf(
			"cette page demande une connexion (mur %q) : essayez l'URL publique de l'offre, du type /jobs/view/…",
			resp.Request.URL.Host)
	}
	resp.Body.Close()
	second, err := fetchOfferPage(client, fallback)
	if err != nil {
		return nil, err
	}
	if behindLoginWall(second.Request.URL) {
		return nil, fmt.Errorf(
			"la page publique de l'offre (hôte %q) est restée derrière un mur de connexion",
			second.Request.URL.Host)
	}
	return second, nil
}

// readOffer turns a successful response into the stored offer.
func readOffer(resp *http.Response) (jobResponse, error) {
	if resp.StatusCode != http.StatusOK {
		return jobResponse{}, statusError(resp)
	}
	media := mediaType(resp)
	if media != "text/html" && media != "text/plain" {
		return jobResponse{}, fmt.Errorf("la page n'est pas lisible comme offre : type %q, want HTML ou texte",
			resp.Header.Get("Content-Type"))
	}
	body, err := readOfferBody(resp)
	if err != nil {
		return jobResponse{}, err
	}
	if media == "text/plain" {
		return plainOffer(resp, body)
	}
	return htmlOffer(resp, body)
}

func statusError(resp *http.Response) error {
	if resp.StatusCode == 999 {
		return fmt.Errorf(
			"la page refuse les lecteurs extérieurs (999) : essayez l'URL publique de l'offre, du type /jobs/view/…")
	}
	return fmt.Errorf("la page a répondu %d %s : ce n'est pas une offre lisible",
		resp.StatusCode, http.StatusText(resp.StatusCode))
}

func mediaType(resp *http.Response) string {
	return strings.ToLower(strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0]))
}

func readOfferBody(resp *http.Response) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxOfferBody+1))
	if err != nil {
		return nil, fmt.Errorf("read page: %v", err)
	}
	if len(body) > maxOfferBody {
		return nil, fmt.Errorf("la page dépasse %d octets : ce n'est pas une offre", maxOfferBody)
	}
	return body, nil
}

// plainOffer handles a page served as text/plain: the text is already readable.
func plainOffer(resp *http.Response, body []byte) (jobResponse, error) {
	text := strings.TrimSpace(string(body))
	if text == "" {
		return jobResponse{}, fmt.Errorf("la page est vide")
	}
	return newOffer(resp, firstLine(text), text), nil
}

// htmlOffer extracts the readable text and the title of an HTML page.
func htmlOffer(resp *http.Response, body []byte) (jobResponse, error) {
	title, text := extractOfferText(string(body))
	if text == "" {
		return jobResponse{}, fmt.Errorf("aucun texte d'offre n'a pu être extrait de la page")
	}
	return newOffer(resp, title, text), nil
}

func newOffer(resp *http.Response, title, text string) jobResponse {
	return jobResponse{
		URL:       finalURL(resp),
		Title:     title,
		Text:      capText(text),
		FetchedAt: time.Now().UTC().Format(time.RFC3339),
	}
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
