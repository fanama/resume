package lint

import (
	"fmt"

	"github.com/fanama/resume/atscv/internal/layout"
)

// dated is one entry whose period is checked, named the way the report names it.
type dated struct {
	where      string
	start, end string
}

// checkDates verifies that every period is parseable and consistent.
func (c *checker) checkDates() {
	yearOnly, withMonth := 0, 0
	for _, p := range c.periods() {
		period, ok := c.checkPeriod(p)
		if !ok {
			continue
		}
		// Granularity comes from the parsed date, not from the raw string: a
		// month name such as "January 2023" carries a month even though it has
		// no separator.
		if period.StartMonth == 0 {
			yearOnly++
		} else {
			withMonth++
		}
	}
	if yearOnly > 0 && withMonth > 0 {
		c.add(Info, "dates.granularity", "dates",
			"some entries carry a month and others only a year",
			"keep the same granularity everywhere, a month is the safest")
	}
}

// periods is every dated entry of the resume, in the order the report shows.
func (c *checker) periods() []dated {
	var periods []dated
	for i, e := range c.resume.Experience {
		periods = append(periods, dated{
			where: fmt.Sprintf("experience[%d] %s", i, entryName(e)),
			start: e.Start,
			end:   e.End,
		})
	}
	for i, e := range c.resume.Education {
		periods = append(periods, dated{
			where: fmt.Sprintf("education[%d] %s", i, e.School),
			start: e.Start,
			end:   e.End,
		})
	}
	for i, a := range c.resume.Activities {
		periods = append(periods, dated{
			where: fmt.Sprintf("activities[%d] %s", i, a.Name),
			start: a.Start,
			end:   a.End,
		})
	}
	return periods
}

// checkPeriod reports on one period and returns it once parsed, or ok false when
// it has no start date to judge by.
func (c *checker) checkPeriod(p dated) (layout.Period, bool) {
	if p.start == "" {
		c.add(Warn, "dates.start", p.where, "the period has no start date",
			"an undated entry is invisible to a chronological filter")
		return layout.Period{}, false
	}
	if p.end == "" {
		c.add(Warn, "dates.end", p.where, "the period has no end date",
			"write \"present\" for a job you are still in")
	}
	period := c.layout.ParsePeriod(p.start, p.end)
	if !period.HasStart {
		c.add(Error, "dates.parse", p.where,
			fmt.Sprintf("%q is not a date I can read", p.start),
			"use YYYY, YYYY-MM, MM/YYYY or \"Jan 2023\"")
		return layout.Period{}, false
	}
	if period.HasEnd {
		if months(period.EndYear, period.EndMonth) < months(period.StartYear, period.StartMonth) {
			c.add(Error, "dates.order", p.where,
				fmt.Sprintf("the period ends before it starts (%s → %s)", p.start, p.end), "")
		}
	}
	if period.Present && period.HasEnd {
		c.add(Info, "dates.present", p.where, "both a date and \"present\" are set", "")
	}
	if !period.Present && !period.HasEnd && p.end != "" {
		c.add(Error, "dates.parse", p.where,
			fmt.Sprintf("%q is not a date I can read", p.end),
			"use YYYY, YYYY-MM, MM/YYYY or \"present\"")
	}
	return period, true
}

func months(year, month int) int {
	if month == 0 {
		month = 1
	}
	return year*12 + month
}
