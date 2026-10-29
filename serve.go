package main

import (
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/fanama/resume/atscv/internal/model"
	"github.com/fanama/resume/atscv/internal/web"
)

// serveOptions are the raw flags of the serve command, before validation.
type serveOptions struct {
	addr    string
	public  bool
	lang    string
	dates   string
	accent  string
	fontDir string
	size    float64
	rules   bool
	demo    string
}

// parseServeFlags reads the serve flags. The defaults come from the
// environment so a platform can inject them without changing the command line.
func parseServeFlags(args []string, stderr io.Writer) (serveOptions, error) {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var o serveOptions
	fs.StringVar(&o.addr, "addr", serveAddr(), "listen address, must be on the loopback interface unless -public is given")
	fs.BoolVar(&o.public, "public", os.Getenv("ATSCV_PUBLIC") != "", "listen on a routable address: for a container behind a proxy, and the instance then has no authentication")
	fs.StringVar(&o.lang, "lang", envOr("ATSCV_LANG", "fr"), "default editor language: fr or en")
	fs.StringVar(&o.dates, "dates", envOr("ATSCV_DATES", "numeric"), "date format in the preview and the exports: numeric, month or year")
	fs.StringVar(&o.accent, "accent", envOr("ATSCV_ACCENT", defaultAccentHex), "accent color of the documents as RRGGBB")
	fs.StringVar(&o.fontDir, "font-dir", os.Getenv("ATSCV_FONT_DIR"), "extra directory searched for TrueType fonts, for the pdf")
	fs.Float64Var(&o.size, "size", envOrFloat("ATSCV_SIZE", 10.5), "body size in points")
	fs.BoolVar(&o.rules, "rules", true, "hairlines under the section headings")
	fs.StringVar(&o.demo, "demo", os.Getenv("ATSCV_DEMO"), "resume JSON proposed when the editor opens with no session yet")
	if err := fs.Parse(args); err != nil {
		return serveOptions{}, err
	}
	if fs.NArg() > 0 {
		return serveOptions{}, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	return o, nil
}

// webOptions validates the flags and loads the optional demo resume.
func (o serveOptions) webOptions() (web.Options, error) {
	editorLang, err := model.ParseLang(o.lang)
	if err != nil {
		return web.Options{}, err
	}
	dateFormat, err := parseDateFormat(o.dates)
	if err != nil {
		return web.Options{}, err
	}
	accentHex, _, err := parseAccent(o.accent)
	if err != nil {
		return web.Options{}, err
	}
	var demoJSON string
	if o.demo != "" {
		raw, err := os.ReadFile(o.demo)
		if err != nil {
			return web.Options{}, fmt.Errorf("demo %s: %w", o.demo, err)
		}
		// A demo that does not load is worse than none: the editor would offer
		// a starting point that turns out to be a broken file.
		if _, err := model.Parse(raw); err != nil {
			return web.Options{}, fmt.Errorf("demo %s: %w", o.demo, err)
		}
		demoJSON = string(raw)
	}
	return web.Options{
		Addr:         o.addr,
		Public:       o.public,
		DemoJSON:     demoJSON,
		Lang:         editorLang,
		DateFormat:   dateFormat,
		Accent:       accentHex,
		SectionRules: o.rules,
		FontSize:     o.size,
		FontDir:      o.fontDir,
	}, nil
}

// runServe starts the local editor. The flags mirror the ones of "build" so a
// document downloaded from the browser is the one the command line would have
// written; the address is forced to the loopback interface by the web package,
// which has no authentication.
func runServe(args []string, stdout, stderr io.Writer) error {
	o, err := parseServeFlags(args, stderr)
	if err != nil {
		return err
	}
	wo, err := o.webOptions()
	if err != nil {
		return err
	}
	srv, err := web.New(wo)
	if err != nil {
		return err
	}
	return srv.ListenAndServe()
}

// runHealthcheck asks the running server whether it is alive. A container image
// has no shell and no curl, so the probe has to be the binary itself. It reads
// the address the server was told to use, which is how it finds the port when
// the platform maps another one onto it.
func runHealthcheck(args []string, stdout io.Writer) error {
	if len(args) > 0 {
		return fmt.Errorf("healthcheck takes no argument, got %q", args[0])
	}
	addr := serveAddr()
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid address %q: %w", addr, err)
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		// A routable address would be a request to the internet from inside the
		// container; the probe stays on the loopback.
		host = "127.0.0.1"
	}
	url := "http://" + net.JoinHostPort(host, port(addr)) + "/healthz"
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("healthcheck %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("healthcheck %s: status %d", url, resp.StatusCode)
	}
	fmt.Fprintln(stdout, "ok")
	return nil
}

func port(addr string) string {
	if _, p, err := net.SplitHostPort(addr); err == nil {
		return p
	}
	return "8080"
}

// serveAddr is the address the server listens on. The environment wins over the
// flag default, so a platform that injects a port does not need to change the
// command line.
func serveAddr() string {
	if v := os.Getenv("ATSCV_ADDR"); v != "" {
		return v
	}
	// PORT is the convention every hosted platform follows: it is set for us,
	// and binding something else means the proxy forwards the traffic into the
	// void. It only decides the port though. Whether to accept that traffic at
	// all stays an explicit decision, so PORT alone does not make the server
	// public: without -public the server still refuses to start.
	if p := os.Getenv("PORT"); p != "" {
		return "0.0.0.0:" + p
	}
	return "127.0.0.1:8080"
}

// envOrFloat is envOr for the flags that carry a number.
func envOrFloat(env string, def float64) float64 {
	if v := os.Getenv(env); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}

func envOr(env, def string) string {
	if v := os.Getenv(env); v != "" {
		return v
	}
	return def
}
