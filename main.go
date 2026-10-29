// Command atscv generates an Applicant Tracking System friendly resume from a
// JSON file, and checks the result against the rules an ATS applies when it
// extracts a candidate profile.
//
// Usage:
//
//	atscv build  -i resume.json -o out/            # docx, pdf and adoc
//	atscv lint   -i resume.json                    # ATS report
//	atscv text   -i resume.json                    # plain text preview
//	atscv check  -i resume.json                    # data validation only
//	atscv serve                                       # editor on the loopback
//	atscv healthcheck                                 # container probe
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
)

// version is overridable at build time with -ldflags "-X main.version=...".
var version = "1.0.0"

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "atscv:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		usage(stderr)
		return errors.New("a command is required")
	}
	switch args[0] {
	case "build":
		return runBuild(args[1:], stdout, stderr)
	case "lint":
		return runLint(args[1:], stdout, stderr)
	case "text":
		return runText(args[1:], stdout, stderr)
	case "check":
		return runCheck(args[1:], stdout, stderr)
	case "serve":
		return runServe(args[1:], stdout, stderr)
	case "healthcheck":
		return runHealthcheck(args[1:], stdout)
	case "version", "-v", "--version":
		fmt.Fprintln(stdout, "atscv", version)
		return nil
	case "help", "-h", "--help":
		usage(stdout)
		return nil
	default:
		usage(stderr)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func usage(w io.Writer) {
	fmt.Fprint(w, `atscv - ATS friendly resume generator

Commands:
  build        render the resume to .docx, .pdf, .adoc, .txt and .tex
  lint         report what an ATS would complain about, with a readability score
  text         print the plain text an ATS extracts, for debugging
  check        validate the JSON data and the date ranges
  serve        start the web editor and the JSON API, on the loopback interface
  healthcheck  probe a running server, for a container health check

Run "atscv <command> -h" for the flags of a command.
`)
}
