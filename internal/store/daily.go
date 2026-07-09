package store

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// normalizeText returns a canonical form of s for deduplication: lower-cased
// and with all internal runs of whitespace collapsed to a single space. The
// original text is always what gets stored or rendered; only comparisons use
// the normalized form.
func normalizeText(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

// NormalizeItemText is the exported form of normalizeText for callers outside
// the store package (e.g. the UI) that need the same canonical comparison.
func NormalizeItemText(s string) string { return normalizeText(s) }

const dateLayout = "2006-01-02"

// Daily is one day's log: the plan checklist filled in each morning and the
// list of accomplishments recorded in the evening.
type Daily struct {
	Date        time.Time // midnight local time
	MorningDone bool
	EveningDone bool
	Plan        []Item
	Done        []string
}

// section names inside a daily file.
const (
	sectionNone = ""
	sectionPlan = "Plan"
	sectionDone = "Done"
)

// parseDaily parses the full markdown content of a daily file.
func parseDaily(content string) (*Daily, error) {
	front, body, err := splitFrontmatter(content)
	if err != nil {
		return nil, err
	}
	d := &Daily{}
	if err := d.applyFrontmatter(front); err != nil {
		return nil, err
	}
	if err := d.parseBody(body); err != nil {
		return nil, err
	}
	return d, nil
}

func (d *Daily) applyFrontmatter(front map[string]string) error {
	var err error
	for key, value := range front {
		switch key {
		case "date":
			d.Date, err = time.ParseInLocation(dateLayout, value, time.Local)
			if err != nil {
				return fmt.Errorf("invalid date %q: %w", value, err)
			}
		case "morning_done":
			d.MorningDone, err = parseBool(key, value)
		case "evening_done":
			d.EveningDone, err = parseBool(key, value)
		case "day", "week":
			// Derived fields, regenerated on save.
		default:
			return fmt.Errorf("unknown frontmatter key %q", key)
		}
		if err != nil {
			return err
		}
	}
	if d.Date.IsZero() {
		return fmt.Errorf("missing required frontmatter key %q", "date")
	}
	return nil
}

func (d *Daily) parseBody(body string) error {
	section := sectionNone
	for lineNo, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimRight(line, " \t")
		switch {
		case trimmed == "" || strings.HasPrefix(trimmed, "# "):
			// Blank line or the page title.
		case strings.HasPrefix(trimmed, "## "):
			section = strings.TrimSpace(trimmed[3:])
			if section != sectionPlan && section != sectionDone {
				return fmt.Errorf("line %d: unknown section %q", lineNo+1, section)
			}
		case section == sectionPlan:
			item, err := parseItemLine(trimmed)
			if err != nil {
				return fmt.Errorf("line %d: %w", lineNo+1, err)
			}
			d.Plan = append(d.Plan, item)
		case section == sectionDone:
			text, err := parseDoneBullet(trimmed)
			if err != nil {
				return fmt.Errorf("line %d: %w", lineNo+1, err)
			}
			d.Done = append(d.Done, text)
		default:
			return fmt.Errorf("line %d: unexpected content outside sections: %q", lineNo+1, trimmed)
		}
	}
	return nil
}

// parseDoneBullet parses a plain "- text" bullet from the Done section.
func parseDoneBullet(line string) (string, error) {
	if !strings.HasPrefix(line, "- ") {
		return "", fmt.Errorf("expected a bullet in Done section, got %q", line)
	}
	text := strings.TrimSpace(line[2:])
	if text == "" {
		return "", errors.New("empty Done bullet")
	}
	return text, nil
}

// render produces the canonical markdown for the day.
func (d *Daily) render() string {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "date: %s\n", d.Date.Format(dateLayout))
	fmt.Fprintf(&b, "day: %s\n", d.Date.Weekday())
	fmt.Fprintf(&b, "week: %s\n", WeekOf(d.Date))
	fmt.Fprintf(&b, "morning_done: %t\n", d.MorningDone)
	fmt.Fprintf(&b, "evening_done: %t\n", d.EveningDone)
	b.WriteString("---\n\n")
	fmt.Fprintf(&b, "# %s, %d %s %d\n", d.Date.Weekday(), d.Date.Day(), d.Date.Month(), d.Date.Year())

	b.WriteString("\n## Plan\n")
	if len(d.Plan) > 0 {
		b.WriteString("\n")
		for _, item := range d.Plan {
			b.WriteString(item.render() + "\n")
		}
	}

	b.WriteString("\n## Done\n")
	if len(d.Done) > 0 {
		b.WriteString("\n")
		for _, text := range d.Done {
			fmt.Fprintf(&b, "- %s\n", text)
		}
	}
	return b.String()
}

// appendUniqueDone appends each non-empty entry of extra to d.Done, skipping
// any whose normalized text already appears there.
func (d *Daily) appendUniqueDone(extra []string) {
	for _, text := range extra {
		if text == "" {
			continue
		}
		norm := normalizeText(text)
		dup := false
		for _, s := range d.Done {
			if normalizeText(s) == norm {
				dup = true
				break
			}
		}
		if !dup {
			d.Done = append(d.Done, text)
		}
	}
}

// hasPlanItem reports whether the plan already contains an item whose
// normalized text matches text, in any state.
func (d *Daily) hasPlanItem(text string) bool {
	norm := normalizeText(text)
	for _, item := range d.Plan {
		if normalizeText(item.Text) == norm {
			return true
		}
	}
	return false
}

// splitFrontmatter separates a leading "---" delimited YAML-ish block of
// "key: value" lines from the markdown body.
func splitFrontmatter(content string) (map[string]string, string, error) {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimRight(lines[0], " \t") != "---" {
		return nil, "", errors.New("missing frontmatter opening delimiter")
	}
	front := map[string]string{}
	for i := 1; i < len(lines); i++ {
		line := strings.TrimRight(lines[i], " \t")
		if line == "---" {
			return front, strings.Join(lines[i+1:], "\n"), nil
		}
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			return nil, "", fmt.Errorf("line %d: malformed frontmatter line %q", i+1, line)
		}
		front[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return nil, "", errors.New("missing frontmatter closing delimiter")
}

func parseBool(key, value string) (bool, error) {
	switch value {
	case "true":
		return true, nil
	case "false":
		return false, nil
	}
	return false, fmt.Errorf("invalid boolean %q for key %q", value, key)
}
