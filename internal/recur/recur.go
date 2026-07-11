// Package recur parses recurrence tags on tasks (@daily, @weekly, @monthly,
// @weekday, plus optional @mon..@sun / @<day> / @HH:MM) and computes occurrence
// times, so both the desktop and mobile apps can schedule reminders from the
// same rules.
package recur

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Kind is a recurrence cadence.
type Kind int

const (
	// Daily fires every day.
	Daily Kind = iota
	// Weekday fires every Monday through Friday.
	Weekday
	// Weekly fires once a week on Recurrence.Weekday.
	Weekly
	// Monthly fires once a month on Recurrence.MonthDay.
	Monthly
)

// Recurrence is a parsed schedule: a cadence plus the time of day and, for
// weekly/monthly, the target weekday / month-day.
type Recurrence struct {
	Kind       Kind
	Weekday    time.Weekday // for Weekly
	hasWeekday bool
	MonthDay   int // for Monthly (1-31, clamped to the month)
	Hour       int
	Minute     int
}

var weekdayNames = map[string]time.Weekday{
	"sun": time.Sunday, "sunday": time.Sunday,
	"mon": time.Monday, "monday": time.Monday,
	"tue": time.Tuesday, "tuesday": time.Tuesday,
	"wed": time.Wednesday, "wednesday": time.Wednesday,
	"thu": time.Thursday, "thursday": time.Thursday,
	"fri": time.Friday, "friday": time.Friday,
	"sat": time.Saturday, "saturday": time.Saturday,
}

// Parse extracts a trailing run of recurrence @tokens from text and returns the
// remaining clean text plus the parsed Recurrence. ok is false when text has no
// recurrence keyword (it is then a normal task). A missing @HH:MM defaults to
// defHour:defMinute; a weekly with no weekday defaults to Monday; a monthly with
// no day defaults to the 1st.
//
// isID (may be nil) reports whether a token body is a known ID (e.g. a
// project); such a token stops the trailing scan instead of being consumed as
// a day/weekday, so an ID slug shaped like a recurrence token (a bare "15", or
// "mon") stays in the clean text rather than being mistaken for a month-day /
// weekday.
func Parse(text string, defHour, defMinute int, isID func(string) bool) (clean string, rec Recurrence, ok bool) {
	fields := strings.Fields(text)
	rec = Recurrence{Hour: defHour, Minute: defMinute, Weekday: time.Monday, MonthDay: 1}
	hasKind := false

	end := len(fields)
	for end > 0 {
		tok := fields[end-1]
		if !strings.HasPrefix(tok, "@") || !consume(strings.ToLower(tok[1:]), &rec, &hasKind, isID) {
			break
		}
		end--
	}
	if !hasKind {
		return text, Recurrence{}, false
	}
	return strings.Join(fields[:end], " "), rec, true
}

// consume applies one @token body to rec, returning false when it is not a
// recognized recurrence token (which stops the trailing scan). A body that
// isID recognizes is never consumed, so ID tags survive the scan.
func consume(body string, rec *Recurrence, hasKind *bool, isID func(string) bool) bool {
	if isID != nil && isID(body) {
		return false
	}
	switch body {
	case "daily":
		rec.Kind, *hasKind = Daily, true
		return true
	case "weekday", "weekdays":
		rec.Kind, *hasKind = Weekday, true
		return true
	case "weekly":
		rec.Kind, *hasKind = Weekly, true
		return true
	case "monthly":
		rec.Kind, *hasKind = Monthly, true
		return true
	}
	if wd, isDay := weekdayNames[body]; isDay {
		rec.Weekday, rec.hasWeekday = wd, true
		return true
	}
	if h, m, isTime := parseHM(body); isTime {
		rec.Hour, rec.Minute = h, m
		return true
	}
	if n, err := strconv.Atoi(body); err == nil && n >= 1 && n <= 31 {
		rec.MonthDay = n
		return true
	}
	return false
}

func parseHM(s string) (hour, minute int, ok bool) {
	h, m, found := strings.Cut(s, ":")
	if !found {
		return 0, 0, false
	}
	hi, err1 := strconv.Atoi(h)
	mi, err2 := strconv.Atoi(m)
	if err1 != nil || err2 != nil || hi < 0 || hi > 23 || mi < 0 || mi > 59 {
		return 0, 0, false
	}
	return hi, mi, true
}

// Next returns the first occurrence strictly after t.
func (r Recurrence) Next(t time.Time) time.Time {
	for day := dayStart(t); ; day = day.AddDate(0, 0, 1) {
		if occ := r.occurrenceOn(day); r.matches(day) && occ.After(t) {
			return occ
		}
	}
}

// MostRecent returns the latest occurrence at or before t.
func (r Recurrence) MostRecent(t time.Time) time.Time {
	for day := dayStart(t); ; day = day.AddDate(0, 0, -1) {
		if occ := r.occurrenceOn(day); r.matches(day) && !occ.After(t) {
			return occ
		}
	}
}

// matches reports whether the recurrence has an occurrence on day's date.
func (r Recurrence) matches(day time.Time) bool {
	switch r.Kind {
	case Daily:
		return true
	case Weekday:
		wd := day.Weekday()
		return wd != time.Saturday && wd != time.Sunday
	case Weekly:
		return day.Weekday() == r.Weekday
	case Monthly:
		return day.Day() == clampDay(day.Year(), day.Month(), r.MonthDay)
	}
	return false
}

// occurrenceOn returns day's date at the recurrence time of day.
func (r Recurrence) occurrenceOn(day time.Time) time.Time {
	return time.Date(day.Year(), day.Month(), day.Day(), r.Hour, r.Minute, 0, 0, day.Location())
}

// Describe renders a human-readable schedule, e.g. "weekly Mon 09:30".
func (r Recurrence) Describe() string {
	when := fmt.Sprintf("%02d:%02d", r.Hour, r.Minute)
	switch r.Kind {
	case Daily:
		return "daily " + when
	case Weekday:
		return "weekdays " + when
	case Weekly:
		return fmt.Sprintf("weekly %s %s", r.Weekday.String()[:3], when)
	case Monthly:
		return fmt.Sprintf("monthly day %d %s", r.MonthDay, when)
	}
	return when
}

func dayStart(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func clampDay(year int, month time.Month, day int) int {
	last := time.Date(year, month+1, 0, 0, 0, 0, 0, time.Local).Day()
	if day > last {
		return last
	}
	if day < 1 {
		return 1
	}
	return day
}
