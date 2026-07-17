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
// (compared by normalizeText) are omitted. An item that appears on multiple
// days (e.g. a carried-over task completed in two copies) is attributed only
// to the first day — subsequent days' copies are silently dropped. Days with
// no items after deduplication are omitted. The result preserves the order of
// dailies.
func DoneByDay(dailies []*Daily) []DayDone {
	var result []DayDone
	globalSeen := map[string]bool{} // cross-day dedup: first occurrence wins
	for _, d := range dailies {
		daySeen := map[string]bool{} // within-day dedup (plan vs Done bullets)
		var items []string
		for _, item := range d.Plan {
			if item.State == StateDone {
				norm := normalizeText(item.Text)
				if !daySeen[norm] && !globalSeen[norm] {
					daySeen[norm] = true
					globalSeen[norm] = true
					items = append(items, item.Text)
				}
			}
		}
		for _, text := range d.Done {
			norm := normalizeText(text)
			if !daySeen[norm] && !globalSeen[norm] {
				daySeen[norm] = true
				globalSeen[norm] = true
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
	// Planned records that the weekly plan was set (distinct from Plan being
	// empty, so an intentionally empty plan is not re-prompted).
	Planned bool
	// Plan holds the week's "big things" goals as checkbox items (StateTodo /
	// StateDone only). Set Monday, ticked through the week.
	Plan []Item
}

const (
	sectionDropped  = "Dropped at review"
	sectionWeekPlan = "Week plan"
)

// parseWeeklyMeta extracts the reviewed/summarized/planned flags, the dropped
// items and the weekly plan from an existing weekly file. Other sections are
// derived and not parsed.
func parseWeeklyMeta(content string) (weeklyMeta, error) {
	var meta weeklyMeta
	front, body, err := splitFrontmatter(content)
	if err != nil {
		return meta, err
	}
	for key, target := range map[string]*bool{
		"reviewed":   &meta.Reviewed,
		"summarized": &meta.Summarized,
		"planned":    &meta.Planned,
	} {
		if value, ok := front[key]; ok {
			if *target, err = parseBool(key, value); err != nil {
				return meta, err
			}
		}
	}
	section := ""
	for line := range strings.SplitSeq(body, "\n") {
		trimmed := strings.TrimRight(line, " \t")
		switch {
		case strings.HasPrefix(trimmed, "## "):
			section = strings.TrimSpace(trimmed[3:])
		case section == sectionDropped && strings.HasPrefix(trimmed, "- "):
			meta.Dropped = append(meta.Dropped, strings.TrimSpace(trimmed[2:]))
		case section == sectionWeekPlan && strings.HasPrefix(trimmed, "- ["):
			item, err := parseItemLine(trimmed)
			if err != nil {
				return meta, fmt.Errorf("week plan: %w", err)
			}
			// Week-plan goals only ever carry StateTodo/StateDone (the goal
			// dialog offers no other choice); normalize any other marker
			// (e.g. a hand-edited "- [>]" postponed bullet) back to todo so
			// it can never reach the goal dialog's two-state selector with a
			// state it has no button for, which would otherwise leave
			// nothing checked (CheckedId() == -1) on save.
			if item.State != StateTodo && item.State != StateDone {
				item.State = StateTodo
			}
			meta.Plan = append(meta.Plan, item)
		}
	}
	return meta, nil
}

// WeekFlags returns the reviewed and summarized flags for week directly from
// the weekly file's frontmatter. Both flags are false when the file does not
// yet exist. This is a lightweight alternative to WeekSummaryPending /
// UnreviewedWeek when the caller knows which week to query.
func (s *Store) WeekFlags(week WeekID) (reviewed, summarized bool, err error) {
	meta, _, err := s.loadWeeklyMeta(week)
	if err != nil {
		return false, false, err
	}
	return meta.Reviewed, meta.Summarized, nil
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
	fmt.Fprintf(&b, "planned: %t\n", meta.Planned)
	b.WriteString("---\n\n")
	fmt.Fprintf(&b, "# Week %d, %d (%s - %s)\n",
		week.Week, week.Year,
		week.Start().Format("2 Jan"), week.End().Format("2 Jan"))

	if len(meta.Plan) > 0 {
		fmt.Fprintf(&b, "\n## %s\n\n", sectionWeekPlan)
		for _, item := range meta.Plan {
			b.WriteString(item.render() + "\n")
		}
	}

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
