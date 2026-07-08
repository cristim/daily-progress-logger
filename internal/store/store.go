// Package store persists daily plans, weekly summaries and the cross-week
// backlog as human-editable markdown files under a single data directory:
//
//	daily/YYYY/MM/YYYY-MM-DD.md   one file per day (Plan checklist + Done)
//	weekly/YYYY/YYYY-Www.md       derived weekly summary
//	backlog.md                    cross-week todo list
//
// All operations are stateless read-modify-write against the files, so the
// user may edit them in any editor while the app is running. Files that fail
// to parse are never overwritten.
package store

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"
)

// Store reads and writes the markdown data files.
type Store struct {
	DataDir string
}

// New returns a store rooted at dataDir.
func New(dataDir string) *Store {
	return &Store{DataDir: dataDir}
}

// DailyPath returns the path of the daily file for date.
func (s *Store) DailyPath(date time.Time) string {
	return filepath.Join(s.DataDir, "daily",
		fmt.Sprintf("%04d", date.Year()),
		fmt.Sprintf("%02d", int(date.Month())),
		date.Format(dateLayout)+".md")
}

// WeeklyPath returns the path of the weekly summary file for week.
func (s *Store) WeeklyPath(week WeekID) string {
	return filepath.Join(s.DataDir, "weekly",
		fmt.Sprintf("%04d", week.Year), week.String()+".md")
}

// BacklogPath returns the path of the cross-week backlog file.
func (s *Store) BacklogPath() string {
	return filepath.Join(s.DataDir, "backlog.md")
}

// LoadDaily reads the daily file for date. exists is false when no file has
// been created for that day yet.
func (s *Store) LoadDaily(date time.Time) (d *Daily, exists bool, err error) {
	content, err := os.ReadFile(s.DailyPath(date))
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("reading daily file: %w", err)
	}
	d, err = parseDaily(string(content))
	if err != nil {
		return nil, true, fmt.Errorf("parsing %s: %w", s.DailyPath(date), err)
	}
	return d, true, nil
}

// loadOrNewDaily returns the existing daily for date or a fresh one.
func (s *Store) loadOrNewDaily(date time.Time) (*Daily, error) {
	d, exists, err := s.LoadDaily(date)
	if err != nil {
		return nil, err
	}
	if !exists {
		d = &Daily{Date: midnight(date)}
	}
	return d, nil
}

// SaveDaily writes the daily file for d.Date.
func (s *Store) SaveDaily(d *Daily) error {
	return writeFile(s.DailyPath(d.Date), d.render())
}

// LoadBacklog reads backlog.md; a missing file is an empty backlog.
func (s *Store) LoadBacklog() (*Backlog, error) {
	content, err := os.ReadFile(s.BacklogPath())
	if os.IsNotExist(err) {
		return &Backlog{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading backlog: %w", err)
	}
	b, err := parseBacklog(string(content))
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", s.BacklogPath(), err)
	}
	return b, nil
}

// SaveBacklog writes backlog.md.
func (s *Store) SaveBacklog(b *Backlog) error {
	return writeFile(s.BacklogPath(), b.render())
}

// DailiesInWeek loads the existing daily files of the week, ordered Monday
// to Sunday.
func (s *Store) DailiesInWeek(week WeekID) ([]*Daily, error) {
	var dailies []*Daily
	for i := range 7 {
		date := week.Start().AddDate(0, 0, i)
		d, exists, err := s.LoadDaily(date)
		if err != nil {
			return nil, err
		}
		if exists {
			dailies = append(dailies, d)
		}
	}
	return dailies, nil
}

// Candidate is a carry-over item offered at the morning check-in.
type Candidate struct {
	Text        string
	FromBacklog bool
}

// MorningCandidates returns carry-over items to offer for today's plan:
// still-open items from earlier days of the current week, then backlog
// Current items; deduplicated and excluding anything already planned today.
func (s *Store) MorningCandidates(today time.Time) ([]Candidate, error) {
	todayDaily, err := s.loadOrNewDaily(today)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, item := range todayDaily.Plan {
		seen[normalizeText(item.Text)] = true
	}

	var candidates []Candidate
	week := WeekOf(today)
	for i := range 7 {
		date := week.Start().AddDate(0, 0, i)
		if !date.Before(midnight(today)) {
			break
		}
		d, exists, err := s.LoadDaily(date)
		if err != nil {
			return nil, err
		}
		if !exists {
			continue
		}
		for _, item := range d.Plan {
			norm := normalizeText(item.Text)
			if item.State == StateTodo && !seen[norm] {
				seen[norm] = true
				candidates = append(candidates, Candidate{Text: item.Text})
			}
		}
	}

	backlog, err := s.LoadBacklog()
	if err != nil {
		return nil, err
	}
	for _, text := range backlog.Current {
		norm := normalizeText(text)
		if !seen[norm] {
			seen[norm] = true
			candidates = append(candidates, Candidate{Text: text, FromBacklog: true})
		}
	}
	return candidates, nil
}

// ApplyMorning records the morning check-in: newItems and the adopted
// carry-over candidates become today's plan, and adopted backlog items are
// removed from the backlog (they now live in the daily file).
func (s *Store) ApplyMorning(today time.Time, newItems []string, adopted []Candidate) error {
	d, err := s.loadOrNewDaily(today)
	if err != nil {
		return err
	}
	for _, c := range adopted {
		if !d.hasPlanItem(c.Text) {
			d.Plan = append(d.Plan, Item{Text: c.Text, State: StateTodo})
		}
	}
	for _, text := range newItems {
		if text != "" && !d.hasPlanItem(text) {
			d.Plan = append(d.Plan, Item{Text: text, State: StateTodo})
		}
	}
	d.MorningDone = true
	if err := s.SaveDaily(d); err != nil {
		return err
	}

	fromBacklog := false
	for _, c := range adopted {
		if c.FromBacklog {
			fromBacklog = true
			break
		}
	}
	if !fromBacklog {
		return nil
	}
	backlog, err := s.LoadBacklog()
	if err != nil {
		return err
	}
	for _, c := range adopted {
		if c.FromBacklog {
			backlog.removeCurrent(c.Text)
		}
	}
	return s.SaveBacklog(backlog)
}

// EveningDecision pairs a plan item's text with the state chosen for it in
// the evening check-in dialog.
type EveningDecision struct {
	Text  string
	State ItemState
}

// findDecisionState looks up the state chosen for itemText in decisions by
// normalized text comparison. It returns the found state and true, or the
// default state and false when no decision matches.
func findDecisionState(decisions []EveningDecision, itemText string) (ItemState, bool) {
	norm := normalizeText(itemText)
	for _, dec := range decisions {
		if normalizeText(dec.Text) == norm {
			return dec.State, true
		}
	}
	return 0, false
}

// ApplyEvening records the evening check-in. Each decision is matched to a
// plan item by normalized text (first match wins); items not mentioned in
// decisions keep their current state; decisions whose text no longer exists
// in the plan are silently ignored (the user may have deleted the item while
// the dialog was open). Postponed items are added to the backlog for next
// week; items leaving the postponed state are removed. extraDone lines are
// appended to the Done section. The weekly summary is regenerated afterwards.
func (s *Store) ApplyEvening(today time.Time, decisions []EveningDecision, extraDone []string) error {
	d, err := s.loadOrNewDaily(today)
	if err != nil {
		return err
	}
	var postponed, unpostponed []string
	for i, item := range d.Plan {
		newState := item.State // default: keep current state unchanged
		if st, ok := findDecisionState(decisions, item.Text); ok {
			newState = st
		}
		if d.Plan[i].State != newState && newState == StatePostponed {
			postponed = append(postponed, item.Text)
		}
		if d.Plan[i].State == StatePostponed && newState != StatePostponed {
			unpostponed = append(unpostponed, item.Text)
		}
		d.Plan[i].State = newState
	}
	for _, text := range extraDone {
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
	d.EveningDone = true
	if err := s.SaveDaily(d); err != nil {
		return err
	}

	if err := s.syncPostponed(postponed, unpostponed); err != nil {
		return err
	}
	return s.RegenerateWeekly(WeekOf(today))
}

// syncPostponed queues newly postponed items in the backlog for next week
// and removes entries for items that left the postponed state.
func (s *Store) syncPostponed(postponed, unpostponed []string) error {
	if len(postponed) == 0 && len(unpostponed) == 0 {
		return nil
	}
	backlog, err := s.LoadBacklog()
	if err != nil {
		return err
	}
	for _, text := range postponed {
		backlog.addNextWeek(text)
	}
	for _, text := range unpostponed {
		backlog.removeNextWeek(text)
	}
	return s.SaveBacklog(backlog)
}

// AddPlanItem appends a new todo to the plan for date.
func (s *Store) AddPlanItem(date time.Time, text string) error {
	d, err := s.loadOrNewDaily(date)
	if err != nil {
		return err
	}
	if text == "" || d.hasPlanItem(text) {
		return nil
	}
	d.Plan = append(d.Plan, Item{Text: text, State: StateTodo})
	return s.SaveDaily(d)
}

// SetPlanItemState updates one plan item's state (e.g. checking it off).
// Moving an item out of the postponed state also removes it from the
// backlog's next-week list, so a reverted postpone leaves no stale entry.
func (s *Store) SetPlanItemState(date time.Time, index int, state ItemState) error {
	d, err := s.loadOrNewDaily(date)
	if err != nil {
		return err
	}
	if index < 0 || index >= len(d.Plan) {
		return fmt.Errorf("plan item index %d out of range (%d items)", index, len(d.Plan))
	}
	unpostponed := d.Plan[index].State == StatePostponed && state != StatePostponed
	d.Plan[index].State = state
	if err := s.SaveDaily(d); err != nil {
		return err
	}
	if !unpostponed {
		return nil
	}
	backlog, err := s.LoadBacklog()
	if err != nil {
		return err
	}
	backlog.removeNextWeek(d.Plan[index].Text)
	return s.SaveBacklog(backlog)
}

// PostponePlanItem marks a plan item postponed and queues it in the backlog
// for next week.
func (s *Store) PostponePlanItem(date time.Time, index int) error {
	d, err := s.loadOrNewDaily(date)
	if err != nil {
		return err
	}
	if index < 0 || index >= len(d.Plan) {
		return fmt.Errorf("plan item index %d out of range (%d items)", index, len(d.Plan))
	}
	d.Plan[index].State = StatePostponed
	if err := s.SaveDaily(d); err != nil {
		return err
	}
	backlog, err := s.LoadBacklog()
	if err != nil {
		return err
	}
	backlog.addNextWeek(d.Plan[index].Text)
	return s.SaveBacklog(backlog)
}

// AdoptFromBacklog adds text to today's plan and removes it from both backlog
// sections. When the item already exists in the plan (e.g. it was postponed),
// its state is reset to StateTodo so it is re-planned for today. The backlog
// is cleaned in either case. If the item is no longer in the backlog (user
// edited the file meanwhile), the removal is a no-op and no error is returned.
func (s *Store) AdoptFromBacklog(today time.Time, text string) error {
	d, err := s.loadOrNewDaily(today)
	if err != nil {
		return err
	}
	norm := normalizeText(text)
	found := false
	for i, item := range d.Plan {
		if normalizeText(item.Text) == norm {
			// Re-plan: reset to todo regardless of the current state.
			d.Plan[i].State = StateTodo
			found = true
			break
		}
	}
	if !found {
		d.Plan = append(d.Plan, Item{Text: text, State: StateTodo})
	}
	if err := s.SaveDaily(d); err != nil {
		return err
	}
	backlog, err := s.LoadBacklog()
	if err != nil {
		return err
	}
	backlog.removeCurrent(text)
	backlog.removeNextWeek(text)
	return s.SaveBacklog(backlog)
}

// ErrBacklogItemNotFound is returned by MoveBacklogItem when the item cannot
// be located in the expected source section (e.g. the file was edited while
// the dialog was open). UI code can test for it with errors.Is to show a
// user-friendly message instead of a raw store error.
var ErrBacklogItemNotFound = fmt.Errorf("backlog item not found")

// MoveBacklogItem moves text between the two backlog sections. When toNextWeek
// is true the item moves from Current to NextWeek; when false from NextWeek to
// Current. addCurrent/addNextWeek guard against duplicate entries. An error
// wrapping ErrBacklogItemNotFound is returned when the item is not found in
// the source section.
func (s *Store) MoveBacklogItem(text string, toNextWeek bool) error {
	backlog, err := s.LoadBacklog()
	if err != nil {
		return err
	}
	if err = backlog.moveItem(text, toNextWeek); err != nil {
		return err
	}
	return s.SaveBacklog(backlog)
}

// moveItem moves text between sections. toNextWeek=true: Current -> NextWeek;
// toNextWeek=false: NextWeek -> Current. Returns an error when the item is not
// found in the source section.
func (b *Backlog) moveItem(text string, toNextWeek bool) error {
	norm := normalizeText(text)
	if toNextWeek {
		return b.moveTo(text, norm, b.Current, "Current", b.removeCurrent, b.addNextWeek)
	}
	return b.moveTo(text, norm, b.NextWeek, "Next week", b.removeNextWeek, b.addCurrent)
}

// moveTo is the generic inner helper for moveItem: it checks that norm exists
// in src, invokes remove and add, returning an error wrapping
// ErrBacklogItemNotFound when the item is absent.
func (b *Backlog) moveTo(text, norm string, src []string, srcName string, remove, add func(string)) error {
	for _, item := range src {
		if normalizeText(item) == norm {
			remove(text)
			add(text)
			return nil
		}
	}
	return fmt.Errorf("item %q not found in the %s backlog section: %w", text, srcName, ErrBacklogItemNotFound)
}

// MoveToBacklog removes a plan item from the day and stores it in the
// backlog's Current list instead.
func (s *Store) MoveToBacklog(date time.Time, index int) error {
	d, err := s.loadOrNewDaily(date)
	if err != nil {
		return err
	}
	if index < 0 || index >= len(d.Plan) {
		return fmt.Errorf("plan item index %d out of range (%d items)", index, len(d.Plan))
	}
	text := d.Plan[index].Text
	d.Plan = slices.Delete(d.Plan, index, index+1)
	if err := s.SaveDaily(d); err != nil {
		return err
	}
	backlog, err := s.LoadBacklog()
	if err != nil {
		return err
	}
	backlog.addCurrent(text)
	return s.SaveBacklog(backlog)
}

// RegenerateWeekly rewrites the weekly summary for week from its daily
// files, preserving the review flag and dropped-items record of an existing
// weekly file. Weeks without any daily data and no existing summary are
// skipped.
func (s *Store) RegenerateWeekly(week WeekID) error {
	return s.regenerateWeekly(week, nil)
}

func (s *Store) regenerateWeekly(week WeekID, mutate func(*weeklyMeta)) error {
	dailies, err := s.DailiesInWeek(week)
	if err != nil {
		return err
	}
	meta, exists, err := s.loadWeeklyMeta(week)
	if err != nil {
		return err
	}
	if len(dailies) == 0 && !exists && mutate == nil {
		return nil
	}
	if mutate != nil {
		mutate(&meta)
	}
	return writeFile(s.WeeklyPath(week), renderWeekly(week, dailies, meta))
}

func (s *Store) loadWeeklyMeta(week WeekID) (meta weeklyMeta, exists bool, err error) {
	content, err := os.ReadFile(s.WeeklyPath(week))
	if os.IsNotExist(err) {
		return weeklyMeta{}, false, nil
	}
	if err != nil {
		return weeklyMeta{}, false, fmt.Errorf("reading weekly file: %w", err)
	}
	meta, err = parseWeeklyMeta(string(content))
	if err != nil {
		return weeklyMeta{}, true, fmt.Errorf("parsing %s: %w", s.WeeklyPath(week), err)
	}
	return meta, true, nil
}

// UnreviewedWeek returns the oldest week before the current one that has
// daily data but has not been through the week review yet. Calling it
// repeatedly (after each review) walks forward through all unreviewed weeks
// in chronological order.
func (s *Store) UnreviewedWeek(now time.Time) (WeekID, bool, error) {
	current := WeekOf(now)
	pattern := filepath.Join(s.DataDir, "daily", "*", "*", "*.md")
	paths, err := filepath.Glob(pattern)
	if err != nil {
		return WeekID{}, false, fmt.Errorf("listing daily files: %w", err)
	}

	// Collect the unique set of past weeks that have at least one daily file.
	weekSet := map[WeekID]bool{}
	for _, path := range paths {
		name := filepath.Base(path)
		d, err := time.ParseInLocation(dateLayout, name[:len(name)-len(".md")], time.Local)
		if err != nil {
			continue // not one of our daily files
		}
		week := WeekOf(d)
		if week.Before(current) {
			weekSet[week] = true
		}
	}

	// Among those weeks, find the oldest that has not yet been reviewed.
	var oldest WeekID
	found := false
	for week := range weekSet {
		meta, _, err := s.loadWeeklyMeta(week)
		if err != nil {
			return WeekID{}, false, err
		}
		if meta.Reviewed {
			continue
		}
		if !found || week.Before(oldest) {
			oldest = week
			found = true
		}
	}
	return oldest, found, nil
}

// ReviewAction is the user's verdict on a leftover item at week review.
type ReviewAction int

const (
	// ReviewKeep keeps the item on the backlog for the new week.
	ReviewKeep ReviewAction = iota
	// ReviewPostpone pushes the item to next week.
	ReviewPostpone
	// ReviewDrop removes the item, recording it in the reviewed week's file.
	ReviewDrop
)

// ReviewDecision pairs a leftover item with the action chosen for it.
type ReviewDecision struct {
	Text   string
	Action ReviewAction
}

// WeekReviewCandidates returns the items to triage at the review of week:
// its still-open plan items plus the backlog's Current list, deduplicated.
func (s *Store) WeekReviewCandidates(week WeekID) ([]string, error) {
	dailies, err := s.DailiesInWeek(week)
	if err != nil {
		return nil, err
	}
	texts := collectByState(dailies, StateTodo)
	backlog, err := s.LoadBacklog()
	if err != nil {
		return nil, err
	}
	for _, text := range backlog.Current {
		norm := normalizeText(text)
		dup := false
		for _, t := range texts {
			if normalizeText(t) == norm {
				dup = true
				break
			}
		}
		if !dup {
			texts = append(texts, text)
		}
	}
	return texts, nil
}

// WeekSummaryPending reports whether now's current week has any daily data
// and has not yet been marked summarized. The WeekID of the current week is
// also returned so callers can open the summary dialog without recomputing it.
func (s *Store) WeekSummaryPending(now time.Time) (WeekID, bool, error) {
	week := WeekOf(now)
	dailies, err := s.DailiesInWeek(week)
	if err != nil {
		return WeekID{}, false, err
	}
	if len(dailies) == 0 {
		return week, false, nil
	}
	meta, _, err := s.loadWeeklyMeta(week)
	if err != nil {
		return WeekID{}, false, err
	}
	return week, !meta.Summarized, nil
}

// MarkWeekSummarized regenerates the weekly file for week with the summarized
// flag set to true, preserving all other meta.
func (s *Store) MarkWeekSummarized(week WeekID) error {
	return s.regenerateWeekly(week, func(meta *weeklyMeta) {
		meta.Summarized = true
	})
}

// ApplyWeekReview records the review of week: if rollover is true, items
// postponed to this week first roll over into Current (correct for the
// scheduled Monday review); then each decision is applied, the dropped items
// are recorded in the weekly file, and the week is marked reviewed.
// Pass rollover=false for on-demand (manual) re-triages so that NextWeek
// items are not prematurely promoted mid-week.
func (s *Store) ApplyWeekReview(week WeekID, decisions []ReviewDecision, rollover bool) error {
	backlog, err := s.LoadBacklog()
	if err != nil {
		return err
	}
	if rollover {
		backlog.rollOver()
	}
	var dropped []string
	for _, dec := range decisions {
		switch dec.Action {
		case ReviewKeep:
			backlog.addCurrent(dec.Text)
		case ReviewPostpone:
			backlog.removeCurrent(dec.Text)
			backlog.addNextWeek(dec.Text)
		case ReviewDrop:
			backlog.removeCurrent(dec.Text)
			// Also remove from NextWeek: an item can appear in both sections
			// (postponed on one day, open as a review candidate from another).
			// Without this, the dropped item survives in NextWeek and
			// resurfaces in Current after the next scheduled rollover (finding 40).
			backlog.removeNextWeek(dec.Text)
			dropped = append(dropped, dec.Text)
		default:
			return fmt.Errorf("unknown review action %d for %q", dec.Action, dec.Text)
		}
	}
	if err := s.SaveBacklog(backlog); err != nil {
		return err
	}
	return s.regenerateWeekly(week, func(meta *weeklyMeta) {
		meta.Reviewed = true
		for _, text := range dropped {
			if !slices.Contains(meta.Dropped, text) {
				meta.Dropped = append(meta.Dropped, text)
			}
		}
	})
}

// midnight truncates t to the start of its day in local time.
func midnight(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
}

// writeFile atomically replaces path with content, creating parent
// directories as needed.
func writeFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(path), err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("replacing %s: %w", path, err)
	}
	return nil
}
