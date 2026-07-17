package mobilecore

import (
	"encoding/json"
	"fmt"

	"github.com/cristim/daily-progress-logger/internal/store"
)

// weeklyGoalJSON is one goal in the weekly plan.
type weeklyGoalJSON struct {
	Text string `json:"text"`
	Done bool   `json:"done"`
}

// weeklyPlanResponseJSON is the response for WeeklyPlanJSON.
type weeklyPlanResponseJSON struct {
	Week    string           `json:"week"`
	Planned bool             `json:"planned"`
	Goals   []weeklyGoalJSON `json:"goals"`
}

// WeeklyPlanJSON returns the weekly plan (goals) for the week containing date.
// date is "YYYY-MM-DD".
func (c *Core) WeeklyPlanJSON(date string) (string, error) {
	week, err := weekFromDate(date)
	if err != nil {
		return "", err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	goals, planned, err := c.store.WeeklyPlan(week)
	if err != nil {
		return "", err
	}
	out := weeklyPlanResponseJSON{
		Week:    week.String(),
		Planned: planned,
		Goals:   make([]weeklyGoalJSON, len(goals)),
	}
	for i, g := range goals {
		out.Goals[i] = weeklyGoalJSON{Text: g.Text, Done: g.State == store.StateDone}
	}
	return toJSON(out)
}

// SetWeeklyPlan sets the weekly plan for the week containing date.
// goalsJSON is a JSON array of {"text":"...","done":false} objects.
// Example: [{"text":"ship mobile core"},{"text":"write tests","done":false}].
//
// Passing "null" or "" returns a BAD_INPUT error: a nil goals list would mark
// the week as planned with zero goals, silently suppressing the weekly-plan
// prompt. Always pass at least an empty array ([]) to clear the plan explicitly.
func (c *Core) SetWeeklyPlan(date, goalsJSON string) error {
	week, err := weekFromDate(date)
	if err != nil {
		return err
	}
	if goalsJSON == "" || goalsJSON == "null" {
		return fmt.Errorf("%s: goalsJSON must be a JSON array (e.g. []), not null or empty"+
			" — a null input would mark the week planned with zero goals,"+
			" silently suppressing the weekly-plan prompt", ErrCodeBadInput)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	var raw []weeklyGoalJSON
	if err := json.Unmarshal([]byte(goalsJSON), &raw); err != nil {
		return fmt.Errorf("parsing goals: %w", err)
	}
	goals := make([]store.Item, len(raw))
	for i, g := range raw {
		st := store.StateTodo
		if g.Done {
			st = store.StateDone
		}
		goals[i] = store.Item{Text: g.Text, State: st}
	}
	return c.store.SetWeeklyPlan(week, goals)
}

// weekReviewCandidatesResponseJSON is the response for WeekReviewCandidatesJSON.
type weekReviewCandidatesResponseJSON struct {
	Week       string   `json:"week"`
	Candidates []string `json:"candidates"`
}

// WeekReviewCandidatesJSON returns the open items to triage at the review of
// the week containing date. Candidates are the week's still-open plan items
// plus the backlog Current list, deduplicated.
func (c *Core) WeekReviewCandidatesJSON(date string) (string, error) {
	week, err := weekFromDate(date)
	if err != nil {
		return "", err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	candidates, err := c.store.WeekReviewCandidates(week)
	if err != nil {
		return "", err
	}
	if candidates == nil {
		candidates = []string{}
	}
	return toJSON(weekReviewCandidatesResponseJSON{Week: week.String(), Candidates: candidates})
}

// reviewDecisionInput is the JSON payload for ApplyWeekReview.
type reviewDecisionInput struct {
	Decisions []struct {
		Text   string `json:"text"`
		Action int    `json:"action"` // 0=keep, 1=postpone, 2=drop
	} `json:"decisions"`
	Rollover bool `json:"rollover"`
}

// ApplyWeekReview records the week review. date is any day in the reviewed week.
// decisionsJSON example:
//
//	{"decisions":[{"text":"old task","action":2}],"rollover":true}
//
// Action values: 0=keep (stays in backlog Current), 1=postpone (move to Next week),
// 2=drop (remove permanently). rollover=true should be set for Monday reviews.
func (c *Core) ApplyWeekReview(date, decisionsJSON string) error {
	week, err := weekFromDate(date)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	var input reviewDecisionInput
	if decisionsJSON != "" && decisionsJSON != "{}" {
		if err := json.Unmarshal([]byte(decisionsJSON), &input); err != nil {
			return fmt.Errorf("parsing review decisions: %w", err)
		}
	}
	decisions := make([]store.ReviewDecision, len(input.Decisions))
	for i, dec := range input.Decisions {
		action, err := parseReviewAction(dec.Action)
		if err != nil {
			return err
		}
		decisions[i] = store.ReviewDecision{Text: dec.Text, Action: action}
	}
	return c.store.ApplyWeekReview(week, decisions, input.Rollover)
}

func parseReviewAction(n int) (store.ReviewAction, error) {
	switch n {
	case 0:
		return store.ReviewKeep, nil
	case 1:
		return store.ReviewPostpone, nil
	case 2:
		return store.ReviewDrop, nil
	}
	return 0, fmt.Errorf("unknown review action %d (0=keep,1=postpone,2=drop)", n)
}

// dayDoneJSON is one day's done items in the weekly summary.
type dayDoneJSON struct {
	Date  string   `json:"date"`
	Items []string `json:"items"`
}

// weeklySummaryJSON is the full weekly summary response.
type weeklySummaryJSON struct {
	Week       string           `json:"week"`
	Start      string           `json:"start"`
	End        string           `json:"end"`
	Summarized bool             `json:"summarized"`
	Reviewed   bool             `json:"reviewed"`
	Goals      []weeklyGoalJSON `json:"goals"`
	DoneByDay  []dayDoneJSON    `json:"done_by_day"`
}

// WeeklySummaryJSON returns the weekly summary for the week containing date,
// including goals, done-by-day breakdown, and reviewed/summarized flags.
// date is "YYYY-MM-DD".
func (c *Core) WeeklySummaryJSON(date string) (string, error) {
	week, err := weekFromDate(date)
	if err != nil {
		return "", err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	dailies, err := c.store.DailiesInWeek(week)
	if err != nil {
		return "", err
	}
	goals, _, err := c.store.WeeklyPlan(week)
	if err != nil {
		return "", err
	}
	reviewed, summarized, err := c.store.WeekFlags(week)
	if err != nil {
		return "", err
	}

	goalItems := make([]weeklyGoalJSON, len(goals))
	for i, g := range goals {
		goalItems[i] = weeklyGoalJSON{Text: g.Text, Done: g.State == store.StateDone}
	}
	dbd := store.DoneByDay(dailies)
	doneByDay := make([]dayDoneJSON, len(dbd))
	for i, dd := range dbd {
		items := dd.Items
		if items == nil {
			items = []string{}
		}
		doneByDay[i] = dayDoneJSON{Date: dd.Date.Format(dateLayout), Items: items}
	}

	out := weeklySummaryJSON{
		Week:       week.String(),
		Start:      week.Start().Format(dateLayout),
		End:        week.End().Format(dateLayout),
		Summarized: summarized,
		Reviewed:   reviewed,
		Goals:      goalItems,
		DoneByDay:  doneByDay,
	}
	return toJSON(out)
}

// MarkWeekSummarized marks the week containing date as summarized.
func (c *Core) MarkWeekSummarized(date string) error {
	week, err := weekFromDate(date)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.store.MarkWeekSummarized(week)
}

// weeklySummaryPendingJSON is the response for WeeklySummaryPendingJSON.
type weeklySummaryPendingJSON struct {
	Pending bool   `json:"pending"`
	Week    string `json:"week"`
}

// WeeklySummaryPendingJSON reports whether any week has a pending (unsummarized)
// weekly summary. date is "YYYY-MM-DD" representing "now".
func (c *Core) WeeklySummaryPendingJSON(date string) (string, error) {
	d, err := parseDate(date)
	if err != nil {
		return "", err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	pendingWeek, pending, err := c.store.WeekSummaryPending(d)
	if err != nil {
		return "", err
	}
	out := weeklySummaryPendingJSON{Pending: pending}
	if pending {
		out.Week = pendingWeek.String()
	}
	return toJSON(out)
}

// unreviewedWeekJSON is the response for UnreviewedWeekJSON.
type unreviewedWeekJSON struct {
	Pending bool   `json:"pending"`
	Week    string `json:"week"`
}

// UnreviewedWeekJSON returns the oldest week with unreviewed data.
// Pending=false when everything has been reviewed. date is "YYYY-MM-DD".
func (c *Core) UnreviewedWeekJSON(date string) (string, error) {
	d, err := parseDate(date)
	if err != nil {
		return "", err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	week, found, err := c.store.UnreviewedWeek(d)
	if err != nil {
		return "", err
	}
	out := unreviewedWeekJSON{Pending: found}
	if found {
		out.Week = week.String()
	}
	return toJSON(out)
}
