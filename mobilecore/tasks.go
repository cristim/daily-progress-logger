package mobilecore

import "fmt"

// AddTask adds a task to date's plan. When projectID is non-empty the task is
// tagged to that project.
func (c *Core) AddTask(date, text, projectID string) error {
	d, err := parseDate(date)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if projectID == "" {
		return c.store.AddPlanItem(d, text)
	}
	return codeStoreErr(c.store.AddTaggedTask(d, text, projectID))
}

// SetTaskState sets a task's state ("todo", "done", or "postponed") by its
// index in the day's plan. expectedText is the display text (project tag
// stripped) captured when the caller last fetched TreeJSON; the core verifies
// it matches before acting. Returns ErrCASMismatch when the tree is stale.
//
// This operation is non-destructive (changes state only, does not remove the
// item) so the index guard fails open when the projects file cannot be read —
// matching Qt's taskIndexValid contract.
func (c *Core) SetTaskState(date string, index int, expectedText, state string) error {
	d, err := parseDate(date)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.verifyIndex(d, index, expectedText); err != nil {
		return err
	}
	st, err := parseState(state)
	if err != nil {
		return err
	}
	return c.store.SetPlanItemState(d, index, st)
}

// DeleteTask moves the plan item at index to the recycle bin. expectedText
// guards against acting on the wrong item when the tree is stale.
//
// Destructive op: uses the strict (fail-closed) index guard so a corrupt
// projects file never causes the wrong item to be deleted.
func (c *Core) DeleteTask(date string, index int, expectedText string) error {
	d, err := parseDate(date)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.verifyIndexStrict(d, index, expectedText); err != nil {
		return err
	}
	return c.store.DeleteTask(d, index)
}

// EditTaskText replaces the display text of the task at index, preserving its
// project tag. expectedText guards against stale-tree mutations.
func (c *Core) EditTaskText(date string, index int, expectedText, newText string) error {
	d, err := parseDate(date)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.verifyIndex(d, index, expectedText); err != nil {
		return err
	}
	return c.store.EditTaskText(d, index, newText)
}

// PostponeToNextDay moves the task at index to tomorrow's plan, removing it
// from today. expectedText guards against stale-tree mutations.
//
// Destructive op: uses the strict (fail-closed) index guard.
func (c *Core) PostponeToNextDay(date string, index int, expectedText string) error {
	d, err := parseDate(date)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.verifyIndexStrict(d, index, expectedText); err != nil {
		return err
	}
	return c.store.PostponeToNextDay(d, index)
}

// PostponeToNextWeek marks the task at index postponed ("[>]") and queues it
// in the next-week backlog. The item stays in today's plan.
// expectedText guards against stale-tree mutations.
func (c *Core) PostponeToNextWeek(date string, index int, expectedText string) error {
	d, err := parseDate(date)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.verifyIndex(d, index, expectedText); err != nil {
		return err
	}
	return c.store.PostponePlanItem(d, index)
}

// MoveTaskToBacklog removes the task at index from the day and adds it to the
// backlog's Current list. expectedText guards against stale-tree mutations.
//
// Destructive op: uses the strict (fail-closed) index guard.
func (c *Core) MoveTaskToBacklog(date string, index int, expectedText string) error {
	d, err := parseDate(date)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.verifyIndexStrict(d, index, expectedText); err != nil {
		return err
	}
	return c.store.MoveToBacklog(d, index)
}

// ReorderTask moves the task at srcIndex to sit immediately before (below=false)
// or after (below=true) the task at refIndex. expectedSrcText guards the source
// task; expectedRefText guards the reference task.
//
// Returns a BAD_INPUT error when the reorder would create a cycle (refIndex is
// srcIndex itself or a descendant of srcIndex).
func (c *Core) ReorderTask(date string, srcIndex int, expectedSrcText string, refIndex int, expectedRefText string, below bool) error {
	d, err := parseDate(date)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.verifyIndex(d, srcIndex, expectedSrcText); err != nil {
		return err
	}
	if err := c.verifyIndex(d, refIndex, expectedRefText); err != nil {
		return err
	}

	// Cycle guard: moving src relative to itself or one of its own descendants
	// is a programming error on the host side, not a stale-tree issue.
	// The store silently ignores it; we surface it as an explicit error so
	// touch-drag UIs get a reliable signal instead of an unexplained ghost revert.
	daily, exists, loaderr := c.store.LoadDaily(d)
	if loaderr != nil || !exists {
		// Cannot load plan — defer cycle check to the store (which will no-op).
		return c.store.ReorderTask(d, srcIndex, refIndex, below)
	}
	plan := daily.Plan
	if srcIndex == refIndex {
		return fmt.Errorf("%s: cannot reorder a task relative to itself", ErrCodeBadInput)
	}
	if srcIndex >= 0 && srcIndex < len(plan) {
		srcDepth := plan[srcIndex].Depth
		for i := srcIndex + 1; i < len(plan); i++ {
			if plan[i].Depth <= srcDepth {
				break // left srcIndex's subtree
			}
			if i == refIndex {
				return fmt.Errorf("%s: cannot reorder a task relative to its own"+
					" descendant (would create a cycle)", ErrCodeBadInput)
			}
		}
	}

	return c.store.ReorderTask(d, srcIndex, refIndex, below)
}

// MoveTaskToProject re-homes the task at index under projectID (empty string
// clears the tag, leaving the task in Unfiled). expectedText guards against
// stale-tree mutations.
func (c *Core) MoveTaskToProject(date string, index int, expectedText, projectID string) error {
	d, err := parseDate(date)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.verifyIndex(d, index, expectedText); err != nil {
		return err
	}
	return codeStoreErr(c.store.MoveTaskToProject(d, index, projectID))
}

// AssignTaskProject is a convenience alias for MoveTaskToProject with a
// non-empty projectID.
func (c *Core) AssignTaskProject(date string, index int, expectedText, projectID string) error {
	d, err := parseDate(date)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.verifyIndex(d, index, expectedText); err != nil {
		return err
	}
	return codeStoreErr(c.store.MoveTaskToProject(d, index, projectID))
}

// UnassignTaskProject removes the project tag from the task at index, moving
// it to Unfiled.
func (c *Core) UnassignTaskProject(date string, index int, expectedText string) error {
	d, err := parseDate(date)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.verifyIndex(d, index, expectedText); err != nil {
		return err
	}
	return c.store.MoveTaskToProject(d, index, "")
}

// AddSubtask inserts a new todo as the last child of the task at parentIndex.
// expectedParentText guards against stale-tree mutations.
func (c *Core) AddSubtask(date string, parentIndex int, expectedParentText, text string) error {
	d, err := parseDate(date)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.verifyIndex(d, parentIndex, expectedParentText); err != nil {
		return err
	}
	return c.store.AddSubtask(d, parentIndex, text)
}

// MakeSubtask nests the task at childIndex as the last child of parentIndex.
// Both expectedChildText and expectedParentText guard against stale-tree mutations.
func (c *Core) MakeSubtask(date string, childIndex int, expectedChildText string, parentIndex int, expectedParentText string) error {
	d, err := parseDate(date)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.verifyIndex(d, childIndex, expectedChildText); err != nil {
		return err
	}
	if err := c.verifyIndex(d, parentIndex, expectedParentText); err != nil {
		return err
	}
	return c.store.MakeSubtask(d, childIndex, parentIndex)
}
