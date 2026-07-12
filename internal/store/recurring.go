package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/cristim/daily-progress-logger/internal/recur"
)

const (
	recurringFileName = "recurring.md"
	// firedStateFile records the last-fired occurrence per template so each
	// occurrence notifies once. It is a dotfile and is not synced across devices
	// (reminders fire independently on each machine).
	firedStateFile = ".recurring-fired.json"
	// materializedStateFile records, per (template, day), whether that
	// occurrence has already been created in the day's plan, so a since-deleted
	// materialized task is never re-added. It is a dotfile and is not synced
	// across devices (each machine materializes its own plan files).
	materializedStateFile = ".recurring-materialized.json"
)

// RecurringTask is a parsed recurring template: its clean display text
// (project and recurrence tags stripped), its project ID (or ""), the parsed
// schedule, and the raw stored line for round-tripping and firing state.
type RecurringTask struct {
	Text    string
	Project string
	Rec     recur.Recurrence
	Raw     string
}

func (s *Store) recurringPath() string  { return filepath.Join(s.DataDir, recurringFileName) }
func (s *Store) firedStatePath() string { return filepath.Join(s.DataDir, firedStateFile) }
func (s *Store) materializedStatePath() string {
	return filepath.Join(s.DataDir, materializedStateFile)
}

// loadRecurringRaws returns the raw text of each stored recurring template, in
// file order. A missing file is not an error (no recurring tasks yet).
func (s *Store) loadRecurringRaws() ([]string, error) {
	data, err := os.ReadFile(s.recurringPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", s.recurringPath(), err)
	}
	var raws []string
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimRight(line, "\r")
		if !strings.HasPrefix(line, "- [") {
			continue
		}
		item, perr := parseItemLine(line)
		if perr != nil {
			continue
		}
		raws = append(raws, item.Text)
	}
	return raws, nil
}

// saveRecurringRaws rewrites recurring.md with the given templates.
func (s *Store) saveRecurringRaws(raws []string) error {
	var b strings.Builder
	b.WriteString("# Recurring\n\n")
	for _, r := range raws {
		b.WriteString(Item{Text: r, State: StateTodo}.render())
		b.WriteByte('\n')
	}
	return writeFile(s.recurringPath(), b.String())
}

// RecurringTasks returns the parsed recurring templates. Lines that no longer
// carry a recurrence keyword (e.g. hand-edited) are skipped.
func (s *Store) RecurringTasks() ([]RecurringTask, error) {
	raws, err := s.loadRecurringRaws()
	if err != nil {
		return nil, err
	}
	if len(raws) == 0 {
		return nil, nil
	}
	projects, err := s.LoadProjects()
	if err != nil {
		return nil, err
	}
	known := allIDs(projects)
	isKnownID := func(id string) bool { return known[id] }
	out := make([]RecurringTask, 0, len(raws))
	for _, raw := range raws {
		clean, rec, ok := recur.Parse(raw, s.defReminderHour, s.defReminderMinute, isKnownID)
		if !ok {
			continue
		}
		text, project := splitProjectTag(clean, known)
		out = append(out, RecurringTask{Text: text, Project: project, Rec: rec, Raw: raw})
	}
	return out, nil
}

// AddRecurring stores text as a recurring template (deduplicated). It errors if
// text carries no recurrence keyword.
func (s *Store) AddRecurring(text string) error {
	text = strings.TrimSpace(text)
	projects, err := s.LoadProjects()
	if err != nil {
		return err
	}
	known := allIDs(projects)
	clean, _, ok := recur.Parse(text, s.defReminderHour, s.defReminderMinute, func(id string) bool { return known[id] })
	if !ok {
		return fmt.Errorf("no recurrence tag in %q", text)
	}
	if strings.TrimSpace(clean) == "" {
		return fmt.Errorf("recurring task %q has no description", text)
	}
	raws, err := s.loadRecurringRaws()
	if err != nil {
		return err
	}
	if slices.Contains(raws, text) {
		return nil
	}
	return s.saveRecurringRaws(append(raws, text))
}

// RemoveRecurring deletes the first template whose raw text matches text. It also
// drops the template's firing state so a re-added template baselines afresh.
func (s *Store) RemoveRecurring(text string) error {
	text = strings.TrimSpace(text)
	raws, err := s.loadRecurringRaws()
	if err != nil {
		return err
	}
	out := make([]string, 0, len(raws))
	removed := false
	for _, r := range raws {
		if r == text && !removed {
			removed = true
			continue
		}
		out = append(out, r)
	}
	if !removed {
		return nil
	}
	if err := s.saveRecurringRaws(out); err != nil {
		return err
	}
	fired, err := s.loadFiredState()
	if err != nil {
		return err
	}
	if _, ok := fired[text]; ok {
		delete(fired, text)
		return s.saveFiredState(fired)
	}
	return nil
}

// RecurringDue returns the templates that have a new occurrence at or before now
// that has not fired yet, updating the persisted firing state so each occurrence
// fires exactly once (even across restarts, where a missed occurrence fires as a
// catch-up). A template seen for the first time is baselined to its current
// occurrence without firing, so adding a task after its time of day does not
// immediately trigger the reminder.
func (s *Store) RecurringDue(now time.Time) ([]RecurringTask, error) {
	tasks, err := s.RecurringTasks()
	if err != nil {
		return nil, err
	}
	fired, err := s.loadFiredState()
	if err != nil {
		return nil, err
	}
	var due []RecurringTask
	next := make(map[string]string, len(tasks))
	for _, t := range tasks {
		occ := t.Rec.MostRecent(now)
		occStr := occ.Format(time.RFC3339)
		last, seen := fired[t.Raw]
		switch {
		case !seen:
			next[t.Raw] = occStr // baseline, do not fire on first sight
		case occStr != last && occ.After(parseFiredTime(last)):
			due = append(due, t)
			next[t.Raw] = occStr
		default:
			next[t.Raw] = last
		}
	}
	if changedState(fired, next, due) {
		if err := s.saveFiredState(next); err != nil {
			return nil, err
		}
	}
	return due, nil
}

// MaterializeRecurring creates a real plan item for every recurring template
// that occurs on date and has not already been materialized for that day,
// returning the tasks actually added. It never touches a day before today
// (local midnight compare), so navigating to a past date cannot inject a task
// into it. Each (template, day) pair is materialized at most once, tracked in
// a persisted state file: once recorded, a since-deleted materialized task is
// not re-added even though it no longer matches hasPlanItem. A template whose
// clean text already appears in the day's plan (e.g. added by hand before
// this feature, or by a previous run that crashed after writing the plan but
// before recording state) is treated as already materialized rather than
// duplicated. Cheap when nothing is due: only the templates and state are
// loaded, and the state file is rewritten only when something changed.
func (s *Store) MaterializeRecurring(date time.Time) (added []RecurringTask, err error) {
	if midnight(date).Before(midnight(time.Now())) {
		return nil, nil
	}
	templates, err := s.RecurringTasks()
	if err != nil {
		return nil, err
	}
	if len(templates) == 0 {
		return nil, nil
	}
	materialized, err := s.loadMaterializedState()
	if err != nil {
		return nil, err
	}
	dateKey := date.Format(dateLayout)
	changed := false
	for _, t := range templates {
		if !t.Rec.OccursOn(date) {
			continue
		}
		key := t.Raw + "\n" + dateKey
		if materialized[key] {
			continue
		}
		d, derr := s.loadOrNewDaily(date)
		if derr != nil {
			return added, derr
		}
		if d.hasPlanItem(t.Text) {
			materialized[key] = true
			changed = true
			continue
		}
		if aerr := s.materializeOne(date, t); aerr != nil {
			return added, aerr
		}
		materialized[key] = true
		changed = true
		added = append(added, t)
	}
	if changed {
		if serr := s.saveMaterializedState(materialized); serr != nil {
			return added, serr
		}
	}
	return added, nil
}

// materializeOne adds t's occurrence to date's plan, keeping its project tag
// when the project still exists and otherwise falling back to an untagged
// item so the occurrence still lands somewhere.
func (s *Store) materializeOne(date time.Time, t RecurringTask) error {
	if t.Project != "" {
		if err := s.AddTaggedTask(date, t.Text, t.Project); err == nil {
			return nil
		}
	}
	return s.AddPlanItem(date, t.Text)
}

// changedState reports whether the firing state needs rewriting: a fired
// occurrence, a new/removed template, or a baselined template.
func changedState(prev, next map[string]string, due []RecurringTask) bool {
	if len(due) > 0 || len(prev) != len(next) {
		return true
	}
	for k, v := range next {
		if prev[k] != v {
			return true
		}
	}
	return false
}

func (s *Store) loadFiredState() (map[string]string, error) {
	data, err := os.ReadFile(s.firedStatePath())
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", s.firedStatePath(), err)
	}
	m := map[string]string{}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", s.firedStatePath(), err)
	}
	return m, nil
}

func (s *Store) saveFiredState(m map[string]string) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding recurring fired state: %w", err)
	}
	return writeFile(s.firedStatePath(), string(data))
}

// parseFiredTime parses a stored RFC3339 timestamp, returning the zero time on a
// malformed value (which then reads as "older than any real occurrence").
func parseFiredTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// loadMaterializedState returns the set of (template raw text, day) pairs
// already materialized into a plan file. A missing file means nothing has
// been materialized yet.
func (s *Store) loadMaterializedState() (map[string]bool, error) {
	data, err := os.ReadFile(s.materializedStatePath())
	if errors.Is(err, os.ErrNotExist) {
		return map[string]bool{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", s.materializedStatePath(), err)
	}
	m := map[string]bool{}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", s.materializedStatePath(), err)
	}
	return m, nil
}

func (s *Store) saveMaterializedState(m map[string]bool) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding recurring materialized state: %w", err)
	}
	return writeFile(s.materializedStatePath(), string(data))
}
