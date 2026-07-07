package store

import (
	"fmt"
	"slices"
	"strings"
)

// Backlog is the cross-week todo list. Current items are offered as
// candidates at every morning check-in; NextWeek items were postponed and
// move into Current at the next week review.
type Backlog struct {
	Current  []string
	NextWeek []string
}

const (
	sectionCurrent  = "Current"
	sectionNextWeek = "Next week"
)

func parseBacklog(content string) (*Backlog, error) {
	b := &Backlog{}
	section := sectionNone
	for lineNo, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimRight(line, " \t")
		switch {
		case trimmed == "" || strings.HasPrefix(trimmed, "# "):
			// Blank line or the page title.
		case strings.HasPrefix(trimmed, "## "):
			section = strings.TrimSpace(trimmed[3:])
			if section != sectionCurrent && section != sectionNextWeek {
				return nil, fmt.Errorf("line %d: unknown section %q", lineNo+1, section)
			}
		case section == sectionCurrent || section == sectionNextWeek:
			item, err := parseItemLine(trimmed)
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", lineNo+1, err)
			}
			if section == sectionCurrent {
				b.Current = append(b.Current, item.Text)
			} else {
				b.NextWeek = append(b.NextWeek, item.Text)
			}
		default:
			return nil, fmt.Errorf("line %d: unexpected content outside sections: %q", lineNo+1, trimmed)
		}
	}
	return b, nil
}

func (b *Backlog) render() string {
	var sb strings.Builder
	sb.WriteString("# Backlog\n")
	sb.WriteString("\n## Current\n")
	writeItems(&sb, b.Current)
	sb.WriteString("\n## Next week\n")
	writeItems(&sb, b.NextWeek)
	return sb.String()
}

func writeItems(sb *strings.Builder, items []string) {
	if len(items) == 0 {
		return
	}
	sb.WriteString("\n")
	for _, text := range items {
		fmt.Fprintf(sb, "- [ ] %s\n", text)
	}
}

// addCurrent appends text to Current unless a normalized match is already present.
func (b *Backlog) addCurrent(text string) {
	norm := normalizeText(text)
	for _, s := range b.Current {
		if normalizeText(s) == norm {
			return
		}
	}
	b.Current = append(b.Current, text)
}

// addNextWeek appends text to NextWeek unless a normalized match is already present.
func (b *Backlog) addNextWeek(text string) {
	norm := normalizeText(text)
	for _, s := range b.NextWeek {
		if normalizeText(s) == norm {
			return
		}
	}
	b.NextWeek = append(b.NextWeek, text)
}

// removeCurrent drops entries from Current whose normalized text matches text.
func (b *Backlog) removeCurrent(text string) {
	norm := normalizeText(text)
	b.Current = slices.DeleteFunc(b.Current, func(s string) bool { return normalizeText(s) == norm })
}

// removeNextWeek drops entries from NextWeek whose normalized text matches text.
func (b *Backlog) removeNextWeek(text string) {
	norm := normalizeText(text)
	b.NextWeek = slices.DeleteFunc(b.NextWeek, func(s string) bool { return normalizeText(s) == norm })
}

// rollOver moves every NextWeek item into Current; called at week review, at
// which point "next week" has arrived.
func (b *Backlog) rollOver() {
	for _, text := range b.NextWeek {
		b.addCurrent(text)
	}
	b.NextWeek = nil
}
