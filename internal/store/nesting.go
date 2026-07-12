package store

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// AddSubtask inserts a new todo as the last child of date's plan item at
// parentIndex, immediately after the parent's current subtree. Subtasks never
// carry a project tag: they inherit their depth-0 ancestor's effective
// project (see effectiveProjects).
func (s *Store) AddSubtask(date time.Time, parentIndex int, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	d, err := s.loadOrNewDaily(date)
	if err != nil {
		return err
	}
	if parentIndex < 0 || parentIndex >= len(d.Plan) {
		return fmt.Errorf("parent index %d out of range (%d items)", parentIndex, len(d.Plan))
	}
	_, end := subtreeSpan(d.Plan, parentIndex)
	item := Item{Text: text, State: StateTodo, Depth: d.Plan[parentIndex].Depth + 1}
	d.Plan = slices.Insert(d.Plan, end, item)
	return s.SaveDaily(d)
}

// MakeSubtask nests date's plan item at childIndex (and its whole subtree) as
// the last child of parentIndex: the subtree is depth-shifted to sit one
// level under the parent, the extracted top item's own project tag is
// stripped (it now inherits the parent's), and the block is re-inserted right
// after the parent's subtree. Same-day only. Returns an error (and leaves the
// plan unchanged) when parentIndex is childIndex itself or falls inside
// childIndex's own subtree, which would create a cycle.
func (s *Store) MakeSubtask(date time.Time, childIndex, parentIndex int) error {
	d, err := s.loadOrNewDaily(date)
	if err != nil {
		return err
	}
	if childIndex < 0 || childIndex >= len(d.Plan) {
		return fmt.Errorf("child index %d out of range (%d items)", childIndex, len(d.Plan))
	}
	if parentIndex < 0 || parentIndex >= len(d.Plan) {
		return fmt.Errorf("parent index %d out of range (%d items)", parentIndex, len(d.Plan))
	}
	childStart, childEnd := subtreeSpan(d.Plan, childIndex)
	if parentIndex == childIndex || (parentIndex >= childStart && parentIndex < childEnd) {
		return fmt.Errorf("cannot make item %d a subtask of itself or its own descendant", childIndex)
	}
	known, err := s.knownProjectIDs()
	if err != nil {
		return err
	}

	extracted := append([]Item(nil), d.Plan[childStart:childEnd]...)
	shift := (d.Plan[parentIndex].Depth + 1) - d.Plan[childIndex].Depth
	for i := range extracted {
		extracted[i].Depth += shift
	}
	extracted[0].Text, _ = splitProjectTag(extracted[0].Text, known)

	plan := slices.Delete(append([]Item(nil), d.Plan...), childStart, childEnd)
	// Indices at or after childEnd shifted left by the removed span's length;
	// the cycle guard above already rules out parentIndex landing inside it.
	adjustedParent := parentIndex
	if childStart < parentIndex {
		adjustedParent -= childEnd - childStart
	}
	_, insertAt := subtreeSpan(plan, adjustedParent)
	plan = slices.Insert(plan, insertAt, extracted...)

	d.Plan = plan
	return s.SaveDaily(d)
}

// ReorderTask moves date's plan item at srcIndex (and its subtree) to sit
// immediately before refIndex (below=false) or immediately after refIndex's
// whole subtree (below=true). The moved top item adopts refIndex's Depth: a
// depth-0 landing spot retags it to refIndex's own effective project
// (stripped if refIndex is itself Unfiled), while any deeper landing spot
// strips its own tag since it now inherits its new parent. Child depths
// stay relative to the moved top. refIndex landing inside srcIndex's own
// subtree (or equal to it) would create a cycle; that request is silently
// ignored (plan left unchanged) rather than corrupting the plan.
func (s *Store) ReorderTask(date time.Time, srcIndex, refIndex int, below bool) error {
	d, err := s.loadOrNewDaily(date)
	if err != nil {
		return err
	}
	if srcIndex < 0 || srcIndex >= len(d.Plan) {
		return fmt.Errorf("src index %d out of range (%d items)", srcIndex, len(d.Plan))
	}
	if refIndex < 0 || refIndex >= len(d.Plan) {
		return fmt.Errorf("ref index %d out of range (%d items)", refIndex, len(d.Plan))
	}
	srcStart, srcEnd := subtreeSpan(d.Plan, srcIndex)
	if refIndex == srcIndex || (refIndex >= srcStart && refIndex < srcEnd) {
		return nil // no-op: would-be cycle
	}
	known, err := s.knownProjectIDs()
	if err != nil {
		return err
	}
	refItem := d.Plan[refIndex]

	extracted, rest := extractSubtree(d.Plan, srcIndex)
	for i := range extracted {
		extracted[i].Depth += refItem.Depth
	}
	if refItem.Depth == 0 {
		clean, _ := splitProjectTag(extracted[0].Text, known)
		_, refTag := splitProjectTag(refItem.Text, known)
		if refTag != "" {
			clean = strings.TrimSpace(clean + " @" + refTag)
		}
		extracted[0].Text = clean
	} else {
		extracted[0].Text, _ = splitProjectTag(extracted[0].Text, known)
	}

	// Indices at or after srcEnd shifted left by the removed span's length;
	// the cycle guard above already rules out refIndex landing inside it. When
	// src was nested inside ref's own subtree, refIndex itself precedes it and
	// so is unaffected; recomputing the "below" insertion point via
	// subtreeSpan on rest (rather than a stored old end) then correctly
	// reflects ref's shrunk subtree either way (see MakeSubtask for the same
	// technique).
	adjustedRef := refIndex
	if srcStart < refIndex {
		adjustedRef -= srcEnd - srcStart
	}
	insertAt := adjustedRef
	if below {
		_, insertAt = subtreeSpan(rest, adjustedRef)
	}
	d.Plan = slices.Insert(rest, insertAt, extracted...)
	return s.SaveDaily(d)
}

// MoveTaskToProject re-homes date's plan item at index (and its subtree) to
// the top level, tagging it to projectID (empty projectID clears the tag,
// leaving the task Unfiled), and moves the block to the end of the plan.
func (s *Store) MoveTaskToProject(date time.Time, index int, projectID string) error {
	d, err := s.loadOrNewDaily(date)
	if err != nil {
		return err
	}
	if index < 0 || index >= len(d.Plan) {
		return fmt.Errorf("plan item index %d out of range (%d items)", index, len(d.Plan))
	}
	known, err := s.knownProjectIDs()
	if err != nil {
		return err
	}
	if projectID != "" && !known[projectID] {
		return fmt.Errorf("project %q not found", projectID)
	}

	start, end := subtreeSpan(d.Plan, index)
	extracted := append([]Item(nil), d.Plan[start:end]...)
	shift := -d.Plan[index].Depth
	for i := range extracted {
		extracted[i].Depth += shift
	}
	clean, _ := splitProjectTag(extracted[0].Text, known)
	if projectID != "" {
		clean = strings.TrimSpace(clean + " @" + projectID)
	}
	extracted[0].Text = clean

	plan := slices.Delete(append([]Item(nil), d.Plan...), start, end)
	plan = append(plan, extracted...)

	d.Plan = plan
	return s.SaveDaily(d)
}

// extractSubtree removes the whole subtree rooted at index from plan, returning
// the removed items re-rooted so the top item is at depth 0 (relative child
// depths preserved) and the plan with that span deleted. Delete / postpone /
// move-to-backlog use this so a task's descendants travel with it instead of
// being left behind and silently reparented under a neighbouring task.
func extractSubtree(plan []Item, index int) (removed, rest []Item) {
	start, end := subtreeSpan(plan, index)
	base := plan[start].Depth
	removed = make([]Item, end-start)
	for i, it := range plan[start:end] {
		it.Depth -= base
		removed[i] = it
	}
	rest = slices.Delete(append([]Item(nil), plan...), start, end)
	return removed, rest
}

// appendSubtree appends a re-rooted subtree (top item at depth 0) to date's
// plan, preserving its nesting. It is a no-op when the top item's text is
// already present, so a repeated postpone does not duplicate the task.
func (s *Store) appendSubtree(date time.Time, items []Item) error {
	if len(items) == 0 {
		return nil
	}
	d, err := s.loadOrNewDaily(date)
	if err != nil {
		return err
	}
	if d.hasPlanItem(items[0].Text) {
		return nil
	}
	d.Plan = append(d.Plan, items...)
	return s.SaveDaily(d)
}

// knownProjectIDs loads the current project ID set.
func (s *Store) knownProjectIDs() (map[string]bool, error) {
	projects, err := s.LoadProjects()
	if err != nil {
		return nil, err
	}
	return allIDs(projects), nil
}
