package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"
)

// ItemStatus is the lifecycle state of a Project or Story.
type ItemStatus int

const (
	// StatusOpen is an active project/story.
	StatusOpen ItemStatus = iota
	// StatusClosed is an archived project/story, hidden from the active tree.
	StatusClosed
)

// String renders the status as it appears in projects.md.
func (s ItemStatus) String() string {
	if s == StatusClosed {
		return "closed"
	}
	return "open"
}

func parseStatus(value string) (ItemStatus, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "open", "":
		return StatusOpen, nil
	case "closed":
		return StatusClosed, nil
	}
	return StatusOpen, fmt.Errorf("unknown status %q, want open or closed", value)
}

// Story is a mid-level container of daily tasks, nested under a Project.
type Story struct {
	ID     string
	Name   string
	Status ItemStatus
}

// Project is a top-level container of Stories.
type Project struct {
	ID      string
	Name    string
	Status  ItemStatus
	Stories []Story
}

// ProjectsPath returns the path of the cross-week projects file.
func (s *Store) ProjectsPath() string {
	return filepath.Join(s.DataDir, "projects.md")
}

// LoadProjects reads projects.md; a missing file is an empty list. IDs absent
// from the file (e.g. hand-added entries) are assigned stable unique slugs.
func (s *Store) LoadProjects() ([]Project, error) {
	content, err := os.ReadFile(s.ProjectsPath())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading projects: %w", err)
	}
	projects, err := parseProjects(string(content))
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", s.ProjectsPath(), err)
	}
	if err := assignMissingIDs(projects); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", s.ProjectsPath(), err)
	}
	return projects, nil
}

// SaveProjects writes projects.md.
func (s *Store) SaveProjects(projects []Project) error {
	return writeFile(s.ProjectsPath(), renderProjects(projects))
}

// parseProjects reads the markdown: `## Name` is a project, `### Name` a story
// nested under the most recent project; `id:` / `status:` lines apply to the
// most recent heading.
func parseProjects(content string) ([]Project, error) {
	var projects []Project
	pi, si := -1, -1 // indexes of the current project / story (-1 = none)
	for lineNo, raw := range strings.Split(content, "\n") {
		line := strings.TrimRight(raw, " \t")
		switch {
		case line == "" || strings.HasPrefix(line, "# "):
			// Blank line or the page title.
		case strings.HasPrefix(line, "### "):
			if pi < 0 {
				return nil, fmt.Errorf("line %d: story before any project", lineNo+1)
			}
			projects[pi].Stories = append(projects[pi].Stories, Story{Name: strings.TrimSpace(line[4:])})
			si = len(projects[pi].Stories) - 1
		case strings.HasPrefix(line, "## "):
			projects = append(projects, Project{Name: strings.TrimSpace(line[3:])})
			pi, si = len(projects)-1, -1
		case strings.Contains(line, ":"):
			if pi < 0 {
				return nil, fmt.Errorf("line %d: field before any project: %q", lineNo+1, line)
			}
			key, value, _ := strings.Cut(line, ":")
			if err := applyProjectField(projects, pi, si, strings.TrimSpace(key), strings.TrimSpace(value)); err != nil {
				return nil, fmt.Errorf("line %d: %w", lineNo+1, err)
			}
		default:
			return nil, fmt.Errorf("line %d: unexpected content outside a heading: %q", lineNo+1, line)
		}
	}
	return projects, nil
}

// applyProjectField sets an id/status field on the current project (si < 0) or
// story (si >= 0).
func applyProjectField(projects []Project, pi, si int, key, value string) error {
	id, status := &projects[pi].ID, &projects[pi].Status
	if si >= 0 {
		id, status = &projects[pi].Stories[si].ID, &projects[pi].Stories[si].Status
	}
	switch key {
	case "id":
		*id = value
	case "status":
		st, err := parseStatus(value)
		if err != nil {
			return err
		}
		*status = st
	default:
		return fmt.Errorf("unknown key %q", key)
	}
	return nil
}

func renderProjects(projects []Project) string {
	var b strings.Builder
	b.WriteString("# Projects\n")
	for _, p := range projects {
		fmt.Fprintf(&b, "\n## %s\nid: %s\nstatus: %s\n", p.Name, p.ID, p.Status)
		for _, st := range p.Stories {
			fmt.Fprintf(&b, "\n### %s\nid: %s\nstatus: %s\n", st.Name, st.ID, st.Status)
		}
	}
	return b.String()
}

// slugify converts a name into a lowercase hyphenated slug (ascii alphanumerics
// only). Empty results fall back to "item".
func slugify(name string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(name) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			prevDash = false
			continue
		}
		if b.Len() > 0 && !prevDash {
			b.WriteByte('-')
			prevDash = true
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		return "item"
	}
	return slug
}

// uniqueSlug returns base, or base-2, base-3, ... until one is not in taken.
func uniqueSlug(base string, taken map[string]bool) string {
	if !taken[base] {
		return base
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if !taken[candidate] {
			return candidate
		}
	}
}

// allIDs collects every project and story ID; story IDs share one namespace
// with project IDs so a task's @slug tag is globally unambiguous.
func allIDs(projects []Project) map[string]bool {
	ids := map[string]bool{}
	for _, p := range projects {
		if p.ID != "" {
			ids[p.ID] = true
		}
		for _, st := range p.Stories {
			if st.ID != "" {
				ids[st.ID] = true
			}
		}
	}
	return ids
}

// assignMissingIDs fills any empty project/story ID with a unique slug and
// rejects duplicate explicit IDs (data corruption).
func assignMissingIDs(projects []Project) error {
	seen := map[string]bool{}
	for pi := range projects {
		if err := ensureID(&projects[pi].ID, projects[pi].Name, seen); err != nil {
			return err
		}
		for si := range projects[pi].Stories {
			if err := ensureID(&projects[pi].Stories[si].ID, projects[pi].Stories[si].Name, seen); err != nil {
				return err
			}
		}
	}
	return nil
}

func ensureID(id *string, name string, seen map[string]bool) error {
	if *id == "" {
		*id = uniqueSlug(slugify(name), seen)
	} else if seen[*id] {
		return fmt.Errorf("duplicate id %q", *id)
	}
	seen[*id] = true
	return nil
}

// findProject returns the index of the project with id, or -1.
func findProject(projects []Project, id string) int {
	for i, p := range projects {
		if p.ID == id {
			return i
		}
	}
	return -1
}

// findStory returns the (project index, story index) of the story with id, or
// (-1, -1).
func findStory(projects []Project, id string) (int, int) {
	for pi, p := range projects {
		for si, st := range p.Stories {
			if st.ID == id {
				return pi, si
			}
		}
	}
	return -1, -1
}

// AddProject creates a new open project and returns its stable ID.
func (s *Store) AddProject(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("project name must not be empty")
	}
	projects, err := s.LoadProjects()
	if err != nil {
		return "", err
	}
	id := uniqueSlug(slugify(name), allIDs(projects))
	projects = append(projects, Project{ID: id, Name: name, Status: StatusOpen})
	return id, s.SaveProjects(projects)
}

// AddStory creates a new open story under projectID and returns its stable ID.
func (s *Store) AddStory(projectID, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("story name must not be empty")
	}
	projects, err := s.LoadProjects()
	if err != nil {
		return "", err
	}
	pi := findProject(projects, projectID)
	if pi < 0 {
		return "", fmt.Errorf("project %q not found", projectID)
	}
	id := uniqueSlug(slugify(name), allIDs(projects))
	projects[pi].Stories = append(projects[pi].Stories, Story{ID: id, Name: name, Status: StatusOpen})
	return id, s.SaveProjects(projects)
}

// RenameProject changes a project's display name, keeping its ID stable.
func (s *Store) RenameProject(projectID, newName string) error {
	newName = strings.TrimSpace(newName)
	if newName == "" {
		return errors.New("project name must not be empty")
	}
	projects, err := s.LoadProjects()
	if err != nil {
		return err
	}
	pi := findProject(projects, projectID)
	if pi < 0 {
		return fmt.Errorf("project %q not found", projectID)
	}
	projects[pi].Name = newName
	return s.SaveProjects(projects)
}

// RenameStory changes a story's display name, keeping its ID stable.
func (s *Store) RenameStory(storyID, newName string) error {
	newName = strings.TrimSpace(newName)
	if newName == "" {
		return errors.New("story name must not be empty")
	}
	projects, err := s.LoadProjects()
	if err != nil {
		return err
	}
	pi, si := findStory(projects, storyID)
	if pi < 0 {
		return fmt.Errorf("story %q not found", storyID)
	}
	projects[pi].Stories[si].Name = newName
	return s.SaveProjects(projects)
}

// SetProjectStatus closes or reopens a project.
func (s *Store) SetProjectStatus(projectID string, status ItemStatus) error {
	projects, err := s.LoadProjects()
	if err != nil {
		return err
	}
	pi := findProject(projects, projectID)
	if pi < 0 {
		return fmt.Errorf("project %q not found", projectID)
	}
	projects[pi].Status = status
	return s.SaveProjects(projects)
}

// SetStoryStatus closes or reopens a story.
func (s *Store) SetStoryStatus(storyID string, status ItemStatus) error {
	projects, err := s.LoadProjects()
	if err != nil {
		return err
	}
	pi, si := findStory(projects, storyID)
	if pi < 0 {
		return fmt.Errorf("story %q not found", storyID)
	}
	projects[pi].Stories[si].Status = status
	return s.SaveProjects(projects)
}

// MoveStory reparents a story to another project, keeping its ID (so the tasks
// tagged to it follow automatically) and its position at the end of the target.
func (s *Store) MoveStory(storyID, toProjectID string) error {
	projects, err := s.LoadProjects()
	if err != nil {
		return err
	}
	pi, si := findStory(projects, storyID)
	if pi < 0 {
		return fmt.Errorf("story %q not found", storyID)
	}
	ti := findProject(projects, toProjectID)
	if ti < 0 {
		return fmt.Errorf("project %q not found", toProjectID)
	}
	if pi == ti {
		return nil
	}
	story := projects[pi].Stories[si]
	projects[pi].Stories = slices.Delete(projects[pi].Stories, si, si+1)
	projects[ti].Stories = append(projects[ti].Stories, story)
	return s.SaveProjects(projects)
}

// splitStoryTag separates a trailing "@<slug>" story tag from a task's text.
// The tag is recognised only when <slug> is a known project/story ID, so an
// ordinary trailing "@mention" in the text is left untouched.
func splitStoryTag(text string, known map[string]bool) (clean, slug string) {
	trimmed := strings.TrimRight(text, " \t")
	space := strings.LastIndexByte(trimmed, ' ')
	last := trimmed[space+1:] // whole string when there is no space
	if !strings.HasPrefix(last, "@") {
		return text, ""
	}
	candidate := last[1:]
	if !known[candidate] {
		return text, ""
	}
	if space < 0 {
		return "", candidate
	}
	return strings.TrimRight(trimmed[:space], " \t"), candidate
}

// AssignTaskStory tags the plan item at index (on date) with storyID, replacing
// any existing story tag. UnassignTaskStory clears it.
func (s *Store) AssignTaskStory(date time.Time, index int, storyID string) error {
	projects, err := s.LoadProjects()
	if err != nil {
		return err
	}
	if pi, _ := findStory(projects, storyID); pi < 0 {
		return fmt.Errorf("story %q not found", storyID)
	}
	return s.retagTask(date, index, allIDs(projects), "@"+storyID)
}

// AddTaggedTask appends a new todo to date's plan already tagged with storyID
// (used when adding a task directly under a story in the tree).
func (s *Store) AddTaggedTask(date time.Time, text, storyID string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	projects, err := s.LoadProjects()
	if err != nil {
		return err
	}
	if pi, _ := findStory(projects, storyID); pi < 0 {
		return fmt.Errorf("story %q not found", storyID)
	}
	d, err := s.loadOrNewDaily(date)
	if err != nil {
		return err
	}
	tagged := text + " @" + storyID
	if d.hasPlanItem(tagged) {
		return nil
	}
	d.Plan = append(d.Plan, Item{Text: tagged, State: StateTodo})
	return s.SaveDaily(d)
}

// UnassignTaskStory removes any story tag from the plan item at index.
func (s *Store) UnassignTaskStory(date time.Time, index int) error {
	projects, err := s.LoadProjects()
	if err != nil {
		return err
	}
	return s.retagTask(date, index, allIDs(projects), "")
}

func (s *Store) retagTask(date time.Time, index int, known map[string]bool, tag string) error {
	d, err := s.loadOrNewDaily(date)
	if err != nil {
		return err
	}
	if index < 0 || index >= len(d.Plan) {
		return fmt.Errorf("plan item index %d out of range (%d items)", index, len(d.Plan))
	}
	clean, _ := splitStoryTag(d.Plan[index].Text, known)
	if tag != "" {
		clean = strings.TrimSpace(clean + " " + tag)
	}
	d.Plan[index].Text = clean
	return s.SaveDaily(d)
}

// FindTaskIndex returns the index of the plan item on date whose text, with any
// story tag stripped, matches displayText (normalized), or -1 when not found.
// The tree resolves a task to its daily item this way so actions hit the right
// row even after edits.
func (s *Store) FindTaskIndex(date time.Time, displayText string) (int, error) {
	projects, err := s.LoadProjects()
	if err != nil {
		return -1, err
	}
	known := allIDs(projects)
	d, exists, err := s.LoadDaily(date)
	if err != nil {
		return -1, err
	}
	if !exists {
		return -1, nil
	}
	norm := normalizeText(displayText)
	for i, it := range d.Plan {
		clean, _ := splitStoryTag(it.Text, known)
		if normalizeText(clean) == norm {
			return i, nil
		}
	}
	return -1, nil
}

// isOpenState reports whether a task still counts as open (not done).
func isOpenState(state ItemState) bool {
	return state == StateTodo || state == StatePostponed
}

// TreeTask is one daily task shown under a story (or Unfiled) in the tree; Text
// has the story tag stripped and Date identifies the daily file that holds it.
type TreeTask struct {
	Text  string
	State ItemState
	Date  time.Time
}

// TreeStory is a story with its open tasks and derived done state (has had
// tasks, none currently open).
type TreeStory struct {
	ID    string
	Name  string
	Done  bool
	Tasks []TreeTask
}

// TreeProject is an open project with its open stories.
type TreeProject struct {
	ID      string
	Name    string
	Done    bool
	Stories []TreeStory
}

// ProjectTree is the aggregated display model for the main window: open
// projects/stories with their open tasks, plus untagged open tasks (Unfiled).
type ProjectTree struct {
	Projects []TreeProject
	Unfiled  []TreeTask
}

// taggedTasks is the per-story aggregation scanned from the daily files: the
// open tasks under each story ID, the set of story IDs that have any task at
// all (open or done, for derived done state), and the untagged open tasks.
type taggedTasks struct {
	openByStory map[string][]TreeTask
	seenByStory map[string]bool
	unfiled     []TreeTask
}

// BuildProjectTree aggregates open tasks from every daily file under their
// tagged story, computes derived done state, and drops closed projects/stories.
// Untagged open tasks collect under Unfiled.
func (s *Store) BuildProjectTree() (*ProjectTree, error) {
	projects, err := s.LoadProjects()
	if err != nil {
		return nil, err
	}
	agg, err := s.scanTaggedTasks(allIDs(projects))
	if err != nil {
		return nil, err
	}
	return &ProjectTree{Projects: openProjectTree(projects, agg), Unfiled: agg.unfiled}, nil
}

// taskKey identifies a logical task by story tag and normalized text, so the
// same task carried across several days collapses to a single tree entry.
type taskKey struct{ slug, norm string }

// scannedTask is a TreeTask plus its index within its day's plan, so same-day
// tasks keep their original plan order after the dedup map (which is unordered).
type scannedTask struct {
	TreeTask

	order int
}

// scanTaggedTasks walks every daily file and buckets its plan items by story
// tag. A task carried across days (still open in each day's file) is deduped to
// its most recent occurrence, so its latest state wins and it is listed once.
func (s *Store) scanTaggedTasks(known map[string]bool) (taggedTasks, error) {
	agg := taggedTasks{openByStory: map[string][]TreeTask{}, seenByStory: map[string]bool{}}
	paths, err := filepath.Glob(filepath.Join(s.DataDir, "daily", "*", "*", "*.md"))
	if err != nil {
		return agg, fmt.Errorf("listing daily files: %w", err)
	}
	// Glob returns paths in lexical (chronological) order, so a later file
	// overwrites an earlier one and "latest occurrence wins".
	latest := map[taskKey]scannedTask{}
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
		for i, item := range d.Plan {
			clean, slug := splitStoryTag(item.Text, known)
			latest[taskKey{slug, normalizeText(clean)}] = scannedTask{
				TreeTask: TreeTask{Text: clean, State: item.State, Date: date},
				order:    i,
			}
		}
	}

	openScanned := map[string][]scannedTask{}
	var unfiledScanned []scannedTask
	for key, st := range latest {
		if key.slug == "" {
			if isOpenState(st.State) {
				unfiledScanned = append(unfiledScanned, st)
			}
			continue
		}
		agg.seenByStory[key.slug] = true
		if isOpenState(st.State) {
			openScanned[key.slug] = append(openScanned[key.slug], st)
		}
	}
	agg.unfiled = sortAndStrip(unfiledScanned)
	for slug, list := range openScanned {
		agg.openByStory[slug] = sortAndStrip(list)
	}
	return agg, nil
}

// sortAndStrip orders tasks by date, then original plan index, then text (a
// stable tree layout, chronological across days and plan-ordered within a day)
// and returns the bare TreeTasks.
func sortAndStrip(list []scannedTask) []TreeTask {
	sort.Slice(list, func(i, j int) bool {
		if !list[i].Date.Equal(list[j].Date) {
			return list[i].Date.Before(list[j].Date)
		}
		if list[i].order != list[j].order {
			return list[i].order < list[j].order
		}
		return list[i].Text < list[j].Text
	})
	out := make([]TreeTask, len(list))
	for i, st := range list {
		out[i] = st.TreeTask
	}
	return out
}

// openProjectTree assembles the open projects/stories with their open tasks and
// derived done state from the scanned aggregation.
func openProjectTree(projects []Project, agg taggedTasks) []TreeProject {
	var out []TreeProject
	for _, p := range projects {
		if p.Status == StatusClosed {
			continue
		}
		tp := TreeProject{ID: p.ID, Name: p.Name}
		projectSeen, projectOpen := false, false
		for _, st := range p.Stories {
			if st.Status == StatusClosed {
				continue
			}
			open := agg.openByStory[st.ID]
			tp.Stories = append(tp.Stories, TreeStory{
				ID: st.ID, Name: st.Name, Tasks: open,
				Done: agg.seenByStory[st.ID] && len(open) == 0,
			})
			projectSeen = projectSeen || agg.seenByStory[st.ID]
			projectOpen = projectOpen || len(open) > 0
		}
		tp.Done = projectSeen && !projectOpen
		out = append(out, tp)
	}
	return out
}
