// Package shared holds the value types and helpers every domain package uses.
package shared

import (
	"strings"
	"time"
)

// LocalDate is a calendar date without time or timezone, serialized as
// `YYYY-MM-DD`. All date math in the domain goes through these helpers so
// rules stay deterministic and independent of the server's clock.
//
// Dates are compared lexically, which matches chronological order.
type LocalDate string

const dateLayout = "2006-01-02"

// ParseLocalDate validates `value` and returns it as a LocalDate.
func ParseLocalDate(value string) (LocalDate, error) {
	t, err := time.Parse(dateLayout, strings.TrimSpace(value))
	if err != nil {
		return "", err
	}
	return LocalDate(t.Format(dateLayout)), nil
}

// IsValid reports whether the date is a real `YYYY-MM-DD` calendar date.
func (d LocalDate) IsValid() bool {
	t, err := time.Parse(dateLayout, string(d))
	return err == nil && t.Format(dateLayout) == string(d)
}

// DateOf returns the calendar date of `t` in its own location.
func DateOf(t time.Time) LocalDate {
	return LocalDate(t.Format(dateLayout))
}

func (d LocalDate) String() string { return string(d) }

// utc anchors the date at midnight UTC, which has no DST gaps, so day
// arithmetic is exact.
func (d LocalDate) utc() time.Time {
	t, _ := time.Parse(dateLayout, string(d))
	return t
}

// In returns midnight of the date in `loc`.
func (d LocalDate) In(loc *time.Location) time.Time {
	t := d.utc()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
}

// AddDays moves the date by `days` (negative goes back).
func (d LocalDate) AddDays(days int) LocalDate {
	return DateOf(d.utc().AddDate(0, 0, days))
}

// AddMonths moves the date by whole months, normalizing overflow like
// JavaScript's Date#setMonth.
func (d LocalDate) AddMonths(months int) LocalDate {
	return DateOf(d.utc().AddDate(0, months, 0))
}

// DaysBetween returns whole days from `from` to `to` (positive when `to` is later).
func DaysBetween(from, to LocalDate) int {
	return int(to.utc().Sub(from.utc()).Hours() / 24)
}

// Weekday returns 0 = Monday … 6 = Sunday (ISO order, which is how people plan weeks).
func (d LocalDate) Weekday() int {
	return (int(d.utc().Weekday()) + 6) % 7
}

// StartOfWeek returns the Monday of the ISO week containing the date.
func (d LocalDate) StartOfWeek() LocalDate {
	return d.AddDays(-d.Weekday())
}

// StartOfMonth returns the first day of the date's month.
func (d LocalDate) StartOfMonth() LocalDate {
	return LocalDate(string(d)[:7] + "-01")
}

// Within reports whether the date falls in [from, to].
func (d LocalDate) Within(from, to LocalDate) bool {
	return d >= from && d <= to
}

// DateRange lists every date from `from` to `to`, both included.
func DateRange(from, to LocalDate) []LocalDate {
	out := make([]LocalDate, 0, max(0, DaysBetween(from, to)+1))
	for d := from; d <= to; d = d.AddDays(1) {
		out = append(out, d)
	}
	return out
}

// MinDate returns the earlier of two dates.
func MinDate(a, b LocalDate) LocalDate {
	if a < b {
		return a
	}
	return b
}
