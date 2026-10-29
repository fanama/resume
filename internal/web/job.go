package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
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
	req, err := readJobRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
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

// readJobRequest decodes the URL from a JSON body or from the "url" form field.
func readJobRequest(r *http.Request) (jobRequest, error) {
	var req jobRequest
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			return jobRequest{}, fmt.Errorf("read body: %v", err)
		}
		if len(bytes.TrimSpace(body)) > 0 {
			if err := json.Unmarshal(body, &req); err != nil {
				return jobRequest{}, fmt.Errorf(`bad JSON: the body must be {"url": "…"}`)
			}
		}
	} else {
		if err := r.ParseForm(); err != nil {
			return jobRequest{}, fmt.Errorf("read form: %v", err)
		}
		req.URL = r.PostFormValue("url")
	}
	req.URL = strings.TrimSpace(req.URL)
	return req, nil
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
