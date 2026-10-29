package layout

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/fanama/resume/atscv/internal/model"
)

// Period is a parsed date range.
type Period struct {
	StartYear, StartMonth int // StartMonth is 0 when only a year is known
	EndYear, EndMonth     int
	Present               bool
	HasStart, HasEnd      bool
	RawStart, RawEnd      string
}

var presentWords = map[string]bool{
	"present": true, "now": true, "current": true, "ongoing": true,
	"aujourd'hui": true, "aujourd hui": true, "présent": true,
	"en cours": true, "actuel": true, "actuelle": true,
}

// ParsePeriod reads a start/end pair. Accepted inputs: "2023", "2023-01",
// "2023/01", "Jan 2023", "January 2023", "present".
func (l *Layout) ParsePeriod(start, end string) Period {
	p := Period{RawStart: start, RawEnd: end}
	if y, m, ok := parseDate(start); ok {
		p.StartYear, p.StartMonth, p.HasStart = y, m, true
	}
	if presentWords[strings.ToLower(strings.TrimSpace(end))] {
		p.Present = true
	} else if y, m, ok := parseDate(end); ok {
		p.EndYear, p.EndMonth, p.HasEnd = y, m, true
	}
	return p
}

func parseDate(s string) (year, month int, ok bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, 0, false
	}
	if y, m, ok := parseNumericDate(s); ok {
		return y, m, true
	}
	fields := strings.Fields(s)
	if len(fields) < 2 {
		return 0, 0, false
	}
	// "Jan 2023", "January 2023", "01 Jan 2023", "Jan. 2023"
	for _, f := range fields {
		if m, ok := monthNumber(f); ok {
			month = m
		} else if y, err := strconv.Atoi(f); err == nil && y > 1900 {
			year = y
		}
	}
	if year == 0 {
		return 0, 0, false
	}
	return year, month, true
}

func parseNumericDate(s string) (year, month int, ok bool) {
	clean := strings.NewReplacer("-", "/", ".", "/", " ", "/").Replace(s)
	parts := strings.Split(clean, "/")
	nums := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return 0, 0, false
		}
		nums = append(nums, n)
	}
	switch len(nums) {
	case 1:
		if nums[0] < 1900 {
			return 0, 0, false
		}
		return nums[0], 0, true
	case 2:
		a, b := nums[0], nums[1]
		switch {
		case a >= 1000: // 2023/01
			if b >= 1 && b <= 12 {
				return a, b, true
			}
		case b >= 1000: // 01/2023
			if a >= 1 && a <= 12 {
				return b, a, true
			}
		}
	}
	return 0, 0, false
}

var monthNames = map[string]int{
	"jan": 1, "january": 1, "janv": 1, "janvier": 1, "jan.": 1,
	"feb": 2, "february": 2, "fevr": 2, "février": 2, "fev": 2, "fev.": 2, "février.": 2,
	"mar": 3, "march": 3, "mars": 3,
	"apr": 4, "april": 4, "avr": 4, "avril": 4,
	"may": 5, "mai": 5,
	"jun": 6, "june": 6, "juin": 6,
	"jul": 7, "july": 7, "juil": 7, "juillet": 7,
	"aug": 8, "august": 8, "aout": 8, "août": 8,
	"sep": 9, "sept": 9, "september": 9, "septembre": 9,
	"oct": 10, "october": 10, "octobre": 10,
	"nov": 11, "november": 11, "novembre": 11,
	"dec": 12, "december": 12, "decembre": 12, "décembre": 12, "déc.": 12,
}

func monthNumber(token string) (int, bool) {
	t := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(token), "."))
	if t == "" {
		return 0, false
	}
	if n, err := strconv.Atoi(t); err == nil && n >= 1 && n <= 12 {
		return n, true
	}
	if m, ok := monthNames[t]; ok {
		return m, true
	}
	return 0, false
}

var monthLabels = map[model.Lang][]string{
	model.LangFR: {"", "janv.", "févr.", "mars", "avr.", "mai", "juin", "juil.", "août", "sept.", "oct.", "nov.", "déc."},
	model.LangEN: {"", "Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"},
}

// FormatMonth writes a month/year pair according to the date format. A date
// that only carries a year stays a year: printing 01/2023 for a 2023 start
// would invent a month the candidate never gave.
func (l *Layout) FormatMonth(year, month int) string {
	if month == 0 || l.opts.DateFormat == DateYear {
		return strconv.Itoa(year)
	}
	if l.opts.DateFormat == DateMonthName {
		return fmt.Sprintf("%s %d", monthLabels[l.opts.Lang][month], year)
	}
	return fmt.Sprintf("%02d/%d", month, year)
}

// period renders a date range such as "03/2022 - 08/2022" or "01/2023 - Present".
func (l *Layout) period(start, end string) string {
	p := l.ParsePeriod(start, end)
	var parts []string
	switch {
	case p.HasStart && p.Present:
		parts = []string{l.FormatMonth(p.StartYear, p.StartMonth), l.labels().Present}
	case p.HasStart && p.HasEnd:
		parts = []string{l.FormatMonth(p.StartYear, p.StartMonth), l.FormatMonth(p.EndYear, p.EndMonth)}
	case p.HasStart:
		parts = []string{l.FormatMonth(p.StartYear, p.StartMonth)}
	case p.Present:
		parts = []string{l.labels().Present}
	case p.HasEnd:
		parts = []string{l.FormatMonth(p.EndYear, p.EndMonth)}
	}
	if len(parts) == 0 {
		if start != "" && end != "" {
			return start + " - " + end
		}
		return strings.TrimSpace(start + " " + end)
	}
	if len(parts) == 1 {
		// A one sided range: an open start ("2020 - ") would look like a
		// mistake to a recruiter, so the known bound is printed alone.
		return parts[0]
	}
	return parts[0] + " - " + parts[1]
}
