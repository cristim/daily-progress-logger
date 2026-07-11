package store

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// RecycleEntry is a deleted task remembered so it can be restored to its day.
type RecycleEntry struct {
	Date time.Time
	Item Item
}

// RecyclePath returns the path of the recycle-bin file.
func (s *Store) RecyclePath() string {
	return filepath.Join(s.DataDir, "recycle.md")
}

// LoadRecycleBin reads recycle.md; a missing file is an empty bin.
func (s *Store) LoadRecycleBin() ([]RecycleEntry, error) {
	content, err := os.ReadFile(s.RecyclePath())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading recycle bin: %w", err)
	}
	entries, err := parseRecycle(string(content))
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", s.RecyclePath(), err)
	}
	return entries, nil
}

// SaveRecycleBin writes recycle.md.
func (s *Store) SaveRecycleBin(entries []RecycleEntry) error {
	return writeFile(s.RecyclePath(), renderRecycle(entries))
}

// parseRecycle reads the bin: `## <date>` sections holding checkbox items whose
// marker preserves the deleted task's state.
func parseRecycle(content string) ([]RecycleEntry, error) {
	var entries []RecycleEntry
	var cur time.Time
	haveDate := false
	for lineNo, raw := range strings.Split(content, "\n") {
		line := strings.TrimRight(raw, " \t")
		switch {
		case line == "" || strings.HasPrefix(line, "# "):
			// Blank line or the page title.
		case strings.HasPrefix(line, "## "):
			d, err := time.ParseInLocation(dateLayout, strings.TrimSpace(line[3:]), time.Local)
			if err != nil {
				return nil, fmt.Errorf("line %d: invalid date %q: %w", lineNo+1, line[3:], err)
			}
			cur, haveDate = d, true
		case strings.HasPrefix(line, "- ["):
			if !haveDate {
				return nil, fmt.Errorf("line %d: item before any date", lineNo+1)
			}
			item, err := parseItemLine(line)
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", lineNo+1, err)
			}
			entries = append(entries, RecycleEntry{Date: cur, Item: item})
		default:
			return nil, fmt.Errorf("line %d: unexpected content: %q", lineNo+1, line)
		}
	}
	return entries, nil
}

func renderRecycle(entries []RecycleEntry) string {
	var b strings.Builder
	b.WriteString("# Recycle bin\n")
	prev := ""
	for _, e := range entries {
		day := e.Date.Format(dateLayout)
		if day != prev {
			fmt.Fprintf(&b, "\n## %s\n\n", day)
			prev = day
		}
		b.WriteString(e.Item.render() + "\n")
	}
	return b.String()
}

// DeleteTask removes the plan item at index (on date) and files it in the
// recycle bin, keeping its state so a restore is faithful.
func (s *Store) DeleteTask(date time.Time, index int) error {
	d, err := s.loadOrNewDaily(date)
	if err != nil {
		return err
	}
	if index < 0 || index >= len(d.Plan) {
		return fmt.Errorf("plan item index %d out of range (%d items)", index, len(d.Plan))
	}
	removed, rest := extractSubtree(d.Plan, index)
	d.Plan = rest
	if err := s.SaveDaily(d); err != nil {
		return err
	}
	bin, err := s.LoadRecycleBin()
	if err != nil {
		return err
	}
	// Recycle the whole subtree so descendants are not orphaned. Each entry is
	// flattened to a top-level task so restoring it is unambiguous (a deleted
	// subtree's nesting is not rebuilt on restore).
	for _, it := range removed {
		it.Depth = 0
		bin = append(bin, RecycleEntry{Date: midnight(date), Item: it})
	}
	return s.SaveRecycleBin(bin)
}

// RestoreTask returns the recycled task matching displayText on date back to
// that day's plan (with its original state and project tag) and drops it from
// the bin. A missing entry is a no-op.
func (s *Store) RestoreTask(date time.Time, displayText string) error {
	entry, bin, ok, err := s.takeRecycled(date, displayText)
	if err != nil || !ok {
		return err
	}
	d, err := s.loadOrNewDaily(date)
	if err != nil {
		return err
	}
	if !d.hasPlanItem(entry.Item.Text) {
		d.Plan = append(d.Plan, entry.Item)
	}
	if err := s.SaveDaily(d); err != nil {
		return err
	}
	return s.SaveRecycleBin(bin)
}

// PurgeRecycled permanently drops the recycled task matching displayText on date.
func (s *Store) PurgeRecycled(date time.Time, displayText string) error {
	_, bin, ok, err := s.takeRecycled(date, displayText)
	if err != nil || !ok {
		return err
	}
	return s.SaveRecycleBin(bin)
}

// takeRecycled finds the first recycle entry on date whose display text
// (project tag stripped) matches displayText and returns it along with the
// bin minus that entry.
func (s *Store) takeRecycled(date time.Time, displayText string) (RecycleEntry, []RecycleEntry, bool, error) {
	projects, err := s.LoadProjects()
	if err != nil {
		return RecycleEntry{}, nil, false, err
	}
	known := allIDs(projects)
	bin, err := s.LoadRecycleBin()
	if err != nil {
		return RecycleEntry{}, nil, false, err
	}
	norm := normalizeText(displayText)
	for i, e := range bin {
		clean := itemDisplayText(e.Item, known)
		if sameDay(e.Date, date) && normalizeText(clean) == norm {
			return e, slices.Delete(bin, i, i+1), true, nil
		}
	}
	return RecycleEntry{}, bin, false, nil
}

// sameDay reports whether a and b are the same calendar day.
func sameDay(a, b time.Time) bool {
	return a.Format(dateLayout) == b.Format(dateLayout)
}
