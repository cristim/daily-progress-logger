package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
