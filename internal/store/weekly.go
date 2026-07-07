package store

import (
	"fmt"
	"strings"
)

// weeklyMeta is the state carried by a weekly file across regenerations: the
// summary sections are derived from the daily files, but the review flag and
// the record of items dropped at review exist only in the weekly file.
type weeklyMeta struct {
	Reviewed bool
	Dropped  []string
}

const sectionDropped = "Dropped at review"

// parseWeeklyMeta extracts the reviewed flag and dropped items from an
// existing weekly file. Other sections are derived and not parsed.
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
	b.WriteString("---\n\n")
	fmt.Fprintf(&b, "# Week %d, %d (%s - %s)\n",
		week.Week, week.Year,
		week.Start().Format("2 Jan"), week.End().Format("2 Jan"))

	b.WriteString("\n## Done\n")
	for _, d := range dailies {
		var done []string
		for _, item := range d.Plan {
			if item.State == StateDone {
				done = append(done, item.Text)
			}
		}
		done = append(done, d.Done...)
		if len(done) == 0 {
			continue
		}
		fmt.Fprintf(&b, "\n### %s, %d %s\n\n", d.Date.Weekday(), d.Date.Day(), d.Date.Month())
		for _, text := range done {
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
