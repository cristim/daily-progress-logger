package store

import (
	"fmt"
	"time"
)

// WeekID identifies an ISO 8601 week.
type WeekID struct {
	Year int
	Week int
}

// WeekOf returns the ISO week containing t.
func WeekOf(t time.Time) WeekID {
	y, w := t.ISOWeek()
	return WeekID{Year: y, Week: w}
}

// String renders the week as e.g. "2026-W28".
func (w WeekID) String() string {
	return fmt.Sprintf("%d-W%02d", w.Year, w.Week)
}

// Start returns the Monday of the week at midnight local time.
func (w WeekID) Start() time.Time {
	// January 4 is always in ISO week 1 of its year.
	jan4 := time.Date(w.Year, time.January, 4, 0, 0, 0, 0, time.Local)
	weekday := int(jan4.Weekday())
	if weekday == 0 { // Sunday
		weekday = 7
	}
	monday := jan4.AddDate(0, 0, 1-weekday)
	return monday.AddDate(0, 0, (w.Week-1)*7)
}

// End returns the Sunday of the week at midnight local time.
func (w WeekID) End() time.Time {
	return w.Start().AddDate(0, 0, 6)
}

// Before reports whether w is strictly earlier than other.
func (w WeekID) Before(other WeekID) bool {
	if w.Year != other.Year {
		return w.Year < other.Year
	}
	return w.Week < other.Week
}
