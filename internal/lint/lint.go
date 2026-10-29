// Package lint checks a resume against the rules that Applicant Tracking
// Systems apply when they extract a profile: readable section titles, a
// complete contact block, consistent date ranges, quantified achievements and
// no layout element that would hide text.
package lint

import (
	"fmt"
	"sort"

	"github.com/fanama/resume/atscv/internal/layout"
	"github.com/fanama/resume/atscv/internal/model"
)

// Severity classifies a finding.
type Severity int

const (
	// Error marks a defect that will lose information during parsing.
	Error Severity = iota
	// Warn marks a weakness that costs points on a keyword match.
	Warn
	// Info marks a suggestion.
	Info
)

func (s Severity) String() string {
	switch s {
	case Error:
		return "error"
	case Warn:
		return "warning"
	default:
		return "info"
	}
}

// Finding is a single lint result.
type Finding struct {
	Severity Severity
	Rule     string
	Where    string
	Message  string
	Hint     string
}

func (f Finding) String() string {
	s := fmt.Sprintf("%-7s %-22s %-18s %s", f.Severity, f.Rule, f.Where, f.Message)
	if f.Hint != "" {
		s += "\n        → " + f.Hint
	}
	return s
}

// Report is the result of a lint run.
type Report struct {
	Findings []Finding
	Stats    Stats
}

// Stats holds the metrics a resume is judged on.
type Stats struct {
	Pages          int
	Words          int
	Bullets        int
	Metrics        int
	Entries        int
	Skills         int
	Chars          int
	ATSReadability int
}

// HasErrors reports whether the report contains at least one error.
func (r Report) HasErrors() bool {
	for _, f := range r.Findings {
		if f.Severity == Error {
			return true
		}
	}
	return false
}

// Score is a 0 to 100 readability estimate, the number a recruiter's tool
// would give to the extracted text. It is an estimate, not a guarantee.
func (r Report) Score() int {
	score := 100
	for _, f := range r.Findings {
		switch f.Severity {
		case Error:
			score -= 12
		case Warn:
			score -= 4
		default:
			score -= 1
		}
	}
	if score < 0 {
		return 0
	}
	return score
}

// Run checks a resume and its layout.
func Run(r *model.Resume, l *layout.Layout) Report {
	var rep Report
	c := &checker{resume: r, layout: l, rep: &rep}

	c.checkIdentity()
	c.checkContact()
	c.checkSections()
	c.checkExperience()
	c.checkDates()
	c.checkText()
	c.computeStats()
	return rep
}

type checker struct {
	resume *model.Resume
	layout *layout.Layout
	rep    *Report
}

func (c *checker) add(sev Severity, rule, where, msg, hint string) {
	c.rep.Findings = append(c.rep.Findings, Finding{
		Severity: sev, Rule: rule, Where: where, Message: msg, Hint: hint,
	})
}

// SortedFindings returns the findings by decreasing severity.
func (r Report) SortedFindings() []Finding {
	out := append([]Finding(nil), r.Findings...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Severity < out[j].Severity })
	return out
}
