package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cristim/daily-progress-logger/internal/recur"
)

// ItemStatus is the lifecycle state of a Project.
type ItemStatus int

const (
	// StatusOpen is an active project.
	StatusOpen ItemStatus = iota
	// StatusClosed is an archived project, hidden from the active tree.
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

// Project is a top-level container of daily tasks (and their subtasks).
type Project struct {
	ID     string
	Name   string
	Status ItemStatus
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

// parseProjects reads the markdown: `## Name` starts a project; `id:` /
// `status:` lines apply to the most recent one.
func parseProjects(content string) ([]Project, error) {
	var projects []Project
	pi := -1 // index of the current project (-1 = none)
	for lineNo, raw := range strings.Split(content, "\n") {
		line := strings.TrimRight(raw, " \t")
		switch {
		case line == "" || strings.HasPrefix(line, "# "):
			// Blank line or the page title.
		case strings.HasPrefix(line, "## "):
			projects = append(projects, Project{Name: strings.TrimSpace(line[3:])})
			pi = len(projects) - 1
		case strings.Contains(line, ":"):
			if pi < 0 {
				return nil, fmt.Errorf("line %d: field before any project: %q", lineNo+1, line)
			}
			key, value, _ := strings.Cut(line, ":")
			if err := applyProjectField(&projects[pi], strings.TrimSpace(key), strings.TrimSpace(value)); err != nil {
				return nil, fmt.Errorf("line %d: %w", lineNo+1, err)
			}
		default:
			return nil, fmt.Errorf("line %d: unexpected content outside a heading: %q", lineNo+1, line)
		}
	}
	return projects, nil
}

// applyProjectField sets an id/status field on the current project.
func applyProjectField(p *Project, key, value string) error {
	switch key {
	case "id":
		p.ID = value
	case "status":
		st, err := parseStatus(value)
		if err != nil {
			return err
		}
		p.Status = st
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

// allIDs collects every project ID.
func allIDs(projects []Project) map[string]bool {
	ids := map[string]bool{}
	for _, p := range projects {
		if p.ID != "" {
			ids[p.ID] = true
		}
	}
	return ids
}

// assignMissingIDs fills any empty project ID with a unique slug and rejects
// duplicate explicit IDs (data corruption).
func assignMissingIDs(projects []Project) error {
	seen := map[string]bool{}
	for pi := range projects {
		if err := ensureID(&projects[pi].ID, projects[pi].Name, seen); err != nil {
			return err
		}
	}
	return nil
}

// ensureID assigns id a unique slug derived from name when empty, and
// rejects a non-empty id that collides with one already in seen.
//
// Recurrence keyword reservation: slugs that collide with a recurrence token
// recognised by recur.Parse (daily, weekly, monthly, weekday(s), weekday
// names/abbreviations, integers 1-31, HH:MM shapes) are treated as already
// taken so uniqueSlug suffixes them (e.g. "daily" -> "daily-2"). This applies
// to both auto-derived slugs (empty *id) and hand-edited id: fields in
// projects.md, preventing a project from silently hijacking @daily/@weekly/…
// tokens in recurring templates and the MigrateRefTags rewrite path.
func ensureID(id *string, name string, seen map[string]bool) error {
	if *id == "" {
		base := slugify(name)
		if recur.IsReservedSlug(base) {
			seen[base] = true // mark as taken so uniqueSlug appends a suffix
		}
		*id = uniqueSlug(base, seen)
	} else {
		if seen[*id] {
			return fmt.Errorf("duplicate id %q", *id)
		}
		// Normalize a hand-edited id that collides with a recurrence keyword.
		if recur.IsReservedSlug(*id) {
			base := *id
			seen[base] = true
			*id = uniqueSlug(base, seen)
		}
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

// AddProject creates a new open project and returns its stable ID.
// If the name slugifies to a recurrence keyword (daily, weekly, monthly,
// weekday names, 1-31, HH:MM), a numeric suffix is appended automatically
// (e.g. "Daily" -> "daily-2") to prevent the slug from shadowing @recurrence
// tokens in recur.Parse and in the MigrateRefTags rewrite path.
func (s *Store) AddProject(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("project name must not be empty")
	}
	projects, err := s.LoadProjects()
	if err != nil {
		return "", err
	}
	taken := allIDs(projects)
	base := slugify(name)
	if recur.IsReservedSlug(base) {
		taken[base] = true // treat as taken so uniqueSlug appends a suffix
	}
	id := uniqueSlug(base, taken)
	projects = append(projects, Project{ID: id, Name: name, Status: StatusOpen})
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
		return fmt.Errorf("project %q: %w", projectID, ErrProjectNotFound)
	}
	projects[pi].Name = newName
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
		return fmt.Errorf("project %q: %w", projectID, ErrProjectNotFound)
	}
	projects[pi].Status = status
	return s.SaveProjects(projects)
}

// splitProjectTag separates a trailing "#<slug>" or legacy "@<slug>" project
// tag from a task's text. "#" is the canonical prefix; "@" is accepted for
// backward compatibility with files written before the @->## migration. The
// tag is recognised only when <slug> is a known project ID, so an ordinary
// trailing "#hashtag" or "@mention" whose body is not a known ID is left
// untouched. Only the last token is examined (one trailing ref tag per task).
// Only depth-0 tasks carry a project tag; callers must not apply this to a
// subtask's text.
func splitProjectTag(text string, known map[string]bool) (clean, slug string) {
	trimmed := strings.TrimRight(text, " \t")
	space := strings.LastIndexByte(trimmed, ' ')
	last := trimmed[space+1:] // whole string when there is no space
	var candidate string
	switch {
	case strings.HasPrefix(last, "#"):
		candidate = last[1:]
	case strings.HasPrefix(last, "@"):
		candidate = last[1:]
	default:
		return text, ""
	}
	if !known[candidate] {
		return text, ""
	}
	if space < 0 {
		return "", candidate
	}
	return strings.TrimRight(trimmed[:space], " \t"), candidate
}

// AssignTaskProject tags the depth-0 plan item at index (on date) with
// projectID, replacing any existing project tag. UnassignTaskProject clears
// it.
func (s *Store) AssignTaskProject(date time.Time, index int, projectID string) error {
	projects, err := s.LoadProjects()
	if err != nil {
		return err
	}
	if findProject(projects, projectID) < 0 {
		return fmt.Errorf("project %q: %w", projectID, ErrProjectNotFound)
	}
	return s.retagTask(date, index, allIDs(projects), "#"+projectID)
}

// UnassignTaskProject removes any project tag from the plan item at index.
func (s *Store) UnassignTaskProject(date time.Time, index int) error {
	projects, err := s.LoadProjects()
	if err != nil {
		return err
	}
	return s.retagTask(date, index, allIDs(projects), "")
}

// ErrProjectNotFound is returned by AddTaggedTask when projectID does not
// match any project in the store. Callers can test with errors.Is to
// distinguish a missing project (safe to fall back to untagged) from genuine
// I/O or other errors (which must be propagated).
var ErrProjectNotFound = errors.New("project not found")

// AddTaggedTask appends a new depth-0 todo to date's plan already tagged with
// projectID (used when adding a task directly under a project in the tree).
// Returns an error wrapping ErrProjectNotFound when projectID is unknown.
func (s *Store) AddTaggedTask(date time.Time, text, projectID string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	projects, err := s.LoadProjects()
	if err != nil {
		return err
	}
	if findProject(projects, projectID) < 0 {
		return fmt.Errorf("project %q: %w", projectID, ErrProjectNotFound)
	}
	d, err := s.loadOrNewDaily(date)
	if err != nil {
		return err
	}
	tagged := text + " #" + projectID
	if d.hasPlanItem(tagged) {
		return nil
	}
	d.Plan = append(d.Plan, Item{Text: tagged, State: StateTodo})
	return s.SaveDaily(d)
}

// retagTask replaces the depth-0 plan item at index's trailing project tag
// (if any, recognised against known) with tag ("" to remove it).
func (s *Store) retagTask(date time.Time, index int, known map[string]bool, tag string) error {
	d, err := s.loadOrNewDaily(date)
	if err != nil {
		return err
	}
	if index < 0 || index >= len(d.Plan) {
		return fmt.Errorf("plan item index %d out of range (%d items)", index, len(d.Plan))
	}
	clean, _ := splitProjectTag(d.Plan[index].Text, known)
	if tag != "" {
		clean = strings.TrimSpace(clean + " " + tag)
	}
	d.Plan[index].Text = clean
	return s.SaveDaily(d)
}
