package store

import (
	"fmt"
	"strings"
	"time"
)

// DayDone holds the deduplicated done items for one day.
// Checked plan items come first, followed by Done-section bullets; when the
// same text appears in both it is emitted exactly once.
type DayDone struct {
	Date  time.Time
	Items []string
}

// DoneByDay computes per-day done items across dailies. Within each day,
// checked plan items are listed first, then Done-section bullets; duplicates
// (compared by normalizeText) are omitted. Days with no items are omitted.
// The result preserves the order of dailies.
func DoneByDay(dailies []*Daily) []DayDone {
	var result []DayDone
	for _, d := range dailies {
		seen := map[string]bool{}
		var items []string
		for _, item := range d.Plan {
			if item.State == StateDone {
				norm := normalizeText(item.Text)
				if !seen[norm] {
					seen[norm] = true
					items = append(items, item.Text)
				}
			}
		}
		for _, text := range d.Done {
			norm := normalizeText(text)
			if !seen[norm] {
				seen[norm] = true
				items = append(items, text)
			}
		}
		if len(items) > 0 {
			result = append(result, DayDone{Date: d.Date, Items: items})
		}
	}
	return result
}

// weeklyMeta is the state carried by a weekly file across regenerations: the
// summary sections are derived from the daily files, but the review flag, the
// summarized flag, and the record of items dropped at review exist only in the
// weekly file.
type weeklyMeta struct {
	Reviewed   bool
	Summarized bool
	Dropped    []string
}

const sectionDropped = "Dropped at review"

// parseWeeklyMeta extracts the reviewed/summarized flags and dropped items
// from an existing weekly file. Other sections are derived and not parsed.
func parseWeeklyMeta(content string) (weeklyMeta, error) {
	var meta weeklyMeta
	front, body, err := splitFrontmatter(content)
	if err != nil {
		return meta, err
	}
	if value, ok := front["reviewed"]; ok {
		meta.Reviewed, err = parseBool("reviewed", value)
		if err != nil {
			return meta, err
		}
	}
	if value, ok := front["summarized"]; ok {
		meta.Summarized, err = parseBool("summarized", value)
		if err != nil {
			return meta, err
		}
	}
	inDropped := false
	for line := range strings.SplitSeq(body, "\n") {
		trimmed := strings.TrimRight(line, " \t")
		switch {
		case strings.HasPrefix(trimmed, "## "):
			inDropped = strings.TrimSpace(trimmed[3:]) == sectionDropped
		case inDropped && strings.HasPrefix(trimmed, "- "):
			meta.Dropped = append(meta.Dropped, strings.TrimSpace(trimmed[2:]))
		}
	}
	return meta, nil
}

// renderWeekly produces the weekly summary markdown from the week's daily
// files plus the carried-over meta.
func renderWeekly(week WeekID, dailies []*Daily, meta weeklyMeta) string {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "week: %s\n", week)
	fmt.Fprintf(&b, "start: %s\n", week.Start().Format(dateLayout))
	fmt.Fprintf(&b, "end: %s\n", week.End().Format(dateLayout))
	fmt.Fprintf(&b, "reviewed: %t\n", meta.Reviewed)
	fmt.Fprintf(&b, "summarized: %t\n", meta.Summarized)
	b.WriteString("---\n\n")
	fmt.Fprintf(&b, "# Week %d, %d (%s - %s)\n",
		week.Week, week.Year,
		week.Start().Format("2 Jan"), week.End().Format("2 Jan"))

	b.WriteString("\n## Done\n")
	for _, dd := range DoneByDay(dailies) {
		fmt.Fprintf(&b, "\n### %s, %d %s\n\n", dd.Date.Weekday(), dd.Date.Day(), dd.Date.Month())
		for _, text := range dd.Items {
			fmt.Fprintf(&b, "- %s\n", text)
		}
	}

	writeWeeklySection(&b, "Not completed", collectByState(dailies, StateTodo))
	writeWeeklySection(&b, "Postponed", collectByState(dailies, StatePostponed))
	writeWeeklySection(&b, sectionDropped, meta.Dropped)
	return b.String()
}

func collectByState(dailies []*Daily, state ItemState) []string {
	var texts []string
	seen := map[string]bool{}
	for _, d := range dailies {
		for _, item := range d.Plan {
			norm := normalizeText(item.Text)
			if item.State == state && !seen[norm] {
				seen[norm] = true
				texts = append(texts, item.Text)
			}
		}
	}
	return texts
}

func writeWeeklySection(b *strings.Builder, title string, items []string) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(b, "\n## %s\n\n", title)
	for _, text := range items {
		fmt.Fprintf(b, "- %s\n", text)
	}
}
