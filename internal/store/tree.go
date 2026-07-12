package store

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// TreeTask is one daily plan item (a top-level task or a subtask nested under
// it) shown in the main window's tree. Text has any project tag stripped;
// Index is the item's position in that day's flat Daily.Plan (its stable
// identity for store operations); Depth mirrors Item.Depth; Done is the
// display-rolled-up completion state: a leaf uses its own checkbox state,
// while a task with children is done only once every child is.
type TreeTask struct {
	Index    int
	Depth    int
	Text     string
	State    ItemState
	Date     time.Time
	Done     bool
	Project  string // project display name; set for recycle-bin entries
	Children []TreeTask
}

// TreeProject is an open project with the viewed day's top-level tasks
// (each carrying its nested subtasks) and its derived global done state.
type TreeProject struct {
	ID    string
	Name  string
	Done  bool
	Tasks []TreeTask
}

// ProjectTree is the display model for the main window: open projects with
// the viewed day's tasks, that day's untagged tasks (Unfiled), and all
// deleted tasks (Recycled).
type ProjectTree struct {
	Projects  []TreeProject
	Unfiled   []TreeTask
	Recycled  []TreeTask
	Recurring []RecurringTask
}

// BuildProjectTree builds the tree for the given day: each open project shows
// that day's tasks (nested subtasks included, open first then done at the
// bottom of each sibling group), while the derived done state (strikethrough)
// is global — a project is done when it has tasks and none remain open, on
// any day. Untagged top-level tasks of the day collect under Unfiled; closed
// projects are dropped.
func (s *Store) BuildProjectTree(date time.Time) (*ProjectTree, error) {
	projects, err := s.LoadProjects()
	if err != nil {
		return nil, err
	}
	known := allIDs(projects)
	agg, err := s.scanProjectState(known) // all days, for the global done state
	if err != nil {
		return nil, err
	}
	dayByProject, dayUnfiled, err := s.dayTasks(date, known) // the selected day, for display
	if err != nil {
		return nil, err
	}
	recycled, err := s.recycledTasks(known, projectNames(projects))
	if err != nil {
		return nil, err
	}
	recurring, err := s.RecurringTasks()
	if err != nil {
		return nil, err
	}
	return &ProjectTree{
		Projects:  openProjectTree(projects, agg, dayByProject),
		Unfiled:   dayUnfiled,
		Recycled:  recycled,
		Recurring: recurring,
	}, nil
}

// KnownProjectIDs returns the set of current project IDs, for UI code (e.g.
// the check-in dialogs) that needs to strip project tags for display.
func (s *Store) KnownProjectIDs() (map[string]bool, error) {
	return s.knownProjectIDs()
}

// DisplayText returns item's text with its project tag stripped (subtasks are
// returned unchanged), using the given known-ID set. Exported wrapper of
// itemDisplayText for the UI layer.
func DisplayText(item Item, known map[string]bool) string {
	return itemDisplayText(item, known)
}

// itemDisplayText returns item's text with its project tag stripped, for
// depth-0 items only (subtasks never carry a tag, so their text is returned
// unchanged).
func itemDisplayText(item Item, known map[string]bool) string {
	if item.Depth != 0 {
		return item.Text
	}
	clean, _ := splitProjectTag(item.Text, known)
	return clean
}

// effectiveProjects returns, for each item in plan, its effective project ID:
// a depth-0 item's own tag (via splitProjectTag), or a deeper item's nearest
// shallower ancestor's effective project. The empty string means Unfiled.
// plan must have normalized depths (see Daily.parseBody) so no item is more
// than one level deeper than its predecessor.
func effectiveProjects(plan []Item, known map[string]bool) []string {
	out := make([]string, len(plan))
	var stack []string // stack[d] = effective project of the most recent item at depth d
	for i, item := range plan {
		d := item.Depth
		var pid string
		switch {
		case d == 0:
			_, pid = splitProjectTag(item.Text, known)
		case d-1 < len(stack):
			pid = stack[d-1]
		}
		if d < len(stack) {
			stack = stack[:d]
		}
		stack = append(stack, pid)
		out[i] = pid
	}
	return out
}

// subtreeSpan returns [start, end) covering plan[i] and all of its
// descendants: end is the first index after i whose depth is <= plan[i]'s (or
// len(plan) when i's subtree runs to the end of the plan).
func subtreeSpan(plan []Item, i int) (start, end int) {
	depth := plan[i].Depth
	end = i + 1
	for end < len(plan) && plan[end].Depth > depth {
		end++
	}
	return i, end
}

// rollupDone computes, for every item in plan, whether it displays as done: a
// leaf uses its own checkbox state; a task with children is done when every
// direct child (itself already resolved bottom-up) is done.
func rollupDone(plan []Item) []bool {
	done := make([]bool, len(plan))
	fillRollupDone(plan, 0, len(plan), done)
	return done
}

func fillRollupDone(plan []Item, start, end int, done []bool) {
	for i := start; i < end; {
		_, subEnd := subtreeSpan(plan, i)
		if subEnd > end {
			subEnd = end
		}
		fillRollupDone(plan, i+1, subEnd, done)

		hasChildren, allChildrenDone := false, true
		for c := i + 1; c < subEnd; {
			hasChildren = true
			if !done[c] {
				allChildrenDone = false
			}
			_, cEnd := subtreeSpan(plan, c)
			if cEnd > subEnd {
				cEnd = subEnd
			}
			c = cEnd
		}
		if hasChildren {
			done[i] = allChildrenDone
		} else {
			done[i] = plan[i].State == StateDone
		}
		i = subEnd
	}
}

// buildSubtree recursively nests display[start:end) — a contiguous run of
// siblings at the same depth — into TreeTasks, using each item's own subtree
// span (against display, whose Depth mirrors the source plan) to bound its
// descendants. done[i] is the precomputed rollup state for display[i].
func buildSubtree(display []Item, done []bool, start, end int, date time.Time) []TreeTask {
	var nodes []TreeTask
	i := start
	for i < end {
		_, subEnd := subtreeSpan(display, i)
		if subEnd > end {
			subEnd = end
		}
		nodes = append(nodes, TreeTask{
			Index: i, Depth: display[i].Depth, Text: display[i].Text, State: display[i].State,
			Date: date, Done: done[i],
			Children: buildSubtree(display, done, i+1, subEnd, date),
		})
		i = subEnd
	}
	return nodes
}

// sortTaskLevel orders one level of tasks open-first then done (ties broken
// by original plan order), recursing into Children so every nesting level
// gets the same treatment.
func sortTaskLevel(tasks []TreeTask) {
	sort.SliceStable(tasks, func(i, j int) bool {
		if tasks[i].Done != tasks[j].Done {
			return !tasks[i].Done
		}
		return tasks[i].Index < tasks[j].Index
	})
	for i := range tasks {
		sortTaskLevel(tasks[i].Children)
	}
}

// dayTasks builds date's plan as nested TreeTasks (subtasks under their
// parent), grouped by each top-level task's effective project (or Unfiled).
func (s *Store) dayTasks(date time.Time, known map[string]bool) (byProject map[string][]TreeTask, unfiled []TreeTask, err error) {
	d, exists, err := s.LoadDaily(date)
	if err != nil {
		return nil, nil, err
	}
	byProject = map[string][]TreeTask{}
	if !exists {
		return byProject, nil, nil
	}

	effProj := effectiveProjects(d.Plan, known)
	display := make([]Item, len(d.Plan))
	for i, item := range d.Plan {
		item.Text = itemDisplayText(item, known)
		display[i] = item
	}
	done := rollupDone(d.Plan)

	for i, item := range d.Plan {
		if item.Depth != 0 {
			continue // nested into its top-level ancestor's subtree below
		}
		_, end := subtreeSpan(display, i)
		node := TreeTask{
			Index: i, Depth: 0, Text: display[i].Text, State: item.State, Date: date, Done: done[i],
			Children: buildSubtree(display, done, i+1, end, date),
		}
		if effProj[i] == "" {
			unfiled = append(unfiled, node)
		} else {
			byProject[effProj[i]] = append(byProject[effProj[i]], node)
		}
	}
	sortTaskLevel(unfiled)
	for p := range byProject {
		sortTaskLevel(byProject[p])
	}
	return byProject, unfiled, nil
}

// recycledTasks maps every recycle-bin entry to a TreeTask (project tag
// stripped for display, original day and state kept).
func (s *Store) recycledTasks(known map[string]bool, names map[string]string) ([]TreeTask, error) {
	bin, err := s.LoadRecycleBin()
	if err != nil {
		return nil, err
	}
	out := make([]TreeTask, 0, len(bin))
	for _, e := range bin {
		var project string
		if e.Item.Depth == 0 {
			if _, pid := splitProjectTag(e.Item.Text, known); pid != "" {
				project = names[pid]
			}
		}
		out = append(out, TreeTask{
			Text: itemDisplayText(e.Item, known), State: e.Item.State, Date: e.Date, Project: project,
		})
	}
	return out, nil
}

// projectNames maps each project ID to its display name.
func projectNames(projects []Project) map[string]string {
	m := make(map[string]string, len(projects))
	for _, p := range projects {
		if p.ID != "" {
			m[p.ID] = p.Name
		}
	}
	return m
}

// taskKey identifies a logical task by effective project and normalized
// text, so the same task carried across several days collapses to a single
// aggregate entry.
type taskKey struct{ project, norm string }

// projectAggregate is the cross-day per-project state used to derive the
// global Done flag: whether the project has ever had a task/subtask (seen)
// and whether any of its deduped (latest-occurrence-wins) tasks/subtasks
// remain open (not done, after rollup).
type projectAggregate struct {
	seen map[string]bool
	open map[string]bool
}

// scanProjectState walks every daily file and attributes each item (at any
// depth) to its effective project, deduping by (project, normalized text) so
// a task carried across days (still open in each day's file) counts once,
// with its latest occurrence's rolled-up done state winning (glob order is
// chronological, so a later file overwrites an earlier one).
func (s *Store) scanProjectState(known map[string]bool) (projectAggregate, error) {
	agg := projectAggregate{seen: map[string]bool{}, open: map[string]bool{}}
	paths, err := filepath.Glob(filepath.Join(s.DataDir, "daily", "*", "*", "*.md"))
	if err != nil {
		return agg, fmt.Errorf("listing daily files: %w", err)
	}

	type entry struct {
		project string
		done    bool
	}
	latest := map[taskKey]entry{}
	for _, path := range paths {
		date, perr := time.ParseInLocation(dateLayout, strings.TrimSuffix(filepath.Base(path), ".md"), time.Local)
		if perr != nil {
			continue // not one of our daily files
		}
		d, exists, lerr := s.LoadDaily(date)
		if lerr != nil {
			return agg, lerr
		}
		if !exists {
			continue
		}
		effProj := effectiveProjects(d.Plan, known)
		done := rollupDone(d.Plan)
		for i, item := range d.Plan {
			if effProj[i] == "" {
				continue // unfiled items do not affect any project's done state
			}
			key := taskKey{effProj[i], normalizeText(itemDisplayText(item, known))}
			latest[key] = entry{project: effProj[i], done: done[i]}
		}
	}

	for _, e := range latest {
		agg.seen[e.project] = true
		if !e.done {
			agg.open[e.project] = true
		}
	}
	return agg, nil
}

// openProjectTree assembles the open projects, showing dayByProject tasks
// (the selected day) under each while deriving done state globally from agg
// (all days).
func openProjectTree(projects []Project, agg projectAggregate, dayByProject map[string][]TreeTask) []TreeProject {
	var out []TreeProject
	for _, p := range projects {
		if p.Status == StatusClosed {
			continue
		}
		out = append(out, TreeProject{
			ID: p.ID, Name: p.Name, Tasks: dayByProject[p.ID],
			Done: agg.seen[p.ID] && !agg.open[p.ID],
		})
	}
	return out
}
