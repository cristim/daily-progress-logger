package store

import (
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// backupDirName is the one-time snapshot of the pre-migration markdown set,
// created under DataDir before the story->project migration touches anything.
const backupDirName = ".pre-subtasks-backup"

// legacyStory mirrors the pre-refactor Story shape (name/id only: status is
// irrelevant since stories are dropped entirely by the migration). It exists
// solely to parse an old projects.md that still has "### " story headings.
type legacyStory struct {
	ID   string
	Name string
}

// legacyProject mirrors the pre-refactor Project shape, nested stories
// included, for the one-time migration parse.
type legacyProject struct {
	ID      string
	Name    string
	Status  ItemStatus
	Stories []legacyStory
}

// parseLegacyProjects parses projects.md's old story-nested format ("## Name"
// project headings with nested "### Name" story headings). Missing IDs are
// assigned the same deterministic slug the pre-refactor code would have used,
// so they match whatever tag already sits on daily plan items.
func parseLegacyProjects(content string) ([]legacyProject, error) {
	var projects []legacyProject
	pi, si := -1, -1
	for lineNo, raw := range strings.Split(content, "\n") {
		line := strings.TrimRight(raw, " \t")
		switch {
		case line == "" || strings.HasPrefix(line, "# "):
			// Blank line or the page title.
		case strings.HasPrefix(line, "### "):
			if pi < 0 {
				return nil, fmt.Errorf("line %d: story before any project", lineNo+1)
			}
			projects[pi].Stories = append(projects[pi].Stories, legacyStory{Name: strings.TrimSpace(line[4:])})
			si = len(projects[pi].Stories) - 1
		case strings.HasPrefix(line, "## "):
			projects = append(projects, legacyProject{Name: strings.TrimSpace(line[3:])})
			pi, si = len(projects)-1, -1
		case strings.Contains(line, ":"):
			if pi < 0 {
				return nil, fmt.Errorf("line %d: field before any project: %q", lineNo+1, line)
			}
			key, value, _ := strings.Cut(line, ":")
			key, value = strings.TrimSpace(key), strings.TrimSpace(value)
			if si >= 0 {
				if key == "id" {
					projects[pi].Stories[si].ID = value
				}
				continue
			}
			switch key {
			case "id":
				projects[pi].ID = value
			case "status":
				st, err := parseStatus(value)
				if err != nil {
					return nil, fmt.Errorf("line %d: %w", lineNo+1, err)
				}
				projects[pi].Status = st
			}
		default:
			return nil, fmt.Errorf("line %d: unexpected content outside a heading: %q", lineNo+1, line)
		}
	}
	seen := map[string]bool{}
	for pi := range projects {
		if err := ensureID(&projects[pi].ID, projects[pi].Name, seen); err != nil {
			return nil, err
		}
		for si := range projects[pi].Stories {
			if err := ensureID(&projects[pi].Stories[si].ID, projects[pi].Stories[si].Name, seen); err != nil {
				return nil, err
			}
		}
	}
	return projects, nil
}

// legacyToProjects drops every story, returning the plain project list (ID,
// Name, Status preserved) plus the storyID -> parentProjectID map used to
// retag daily plan items.
func legacyToProjects(legacy []legacyProject) (projects []Project, storyToProject map[string]string) {
	storyToProject = map[string]string{}
	projects = make([]Project, len(legacy))
	for i, p := range legacy {
		projects[i] = Project{ID: p.ID, Name: p.Name, Status: p.Status}
		for _, st := range p.Stories {
			storyToProject[st.ID] = p.ID
		}
	}
	return projects, storyToProject
}

// migrateStoriesToProjects performs the one-time, idempotent migration off
// the removed Story concept: if projects.md still has "### " story headings,
// it backs up the whole markdown data set, retags every daily plan item
// carrying a former story tag with its parent project's ID, and rewrites
// projects.md in the new story-free format. A projects.md with no story
// headings (the common case, including every run after the first) is a
// cheap no-op: no directory is scanned and nothing is written.
func (s *Store) migrateStoriesToProjects() error {
	content, err := os.ReadFile(s.ProjectsPath())
	if os.IsNotExist(err) {
		return nil // no projects.md yet: nothing to migrate.
	}
	if err != nil {
		return fmt.Errorf("reading %s: %w", s.ProjectsPath(), err)
	}
	if !strings.Contains(string(content), "\n### ") && !strings.HasPrefix(string(content), "### ") {
		return nil // already story-free.
	}

	legacy, err := parseLegacyProjects(string(content))
	if err != nil {
		return fmt.Errorf("parsing legacy %s: %w", s.ProjectsPath(), err)
	}
	projects, storyToProject := legacyToProjects(legacy)

	if err := s.backupBeforeMigration(); err != nil {
		return fmt.Errorf("backing up before migration: %w", err)
	}
	rewrittenDaily, err := s.migrateDailyStoryTags(storyToProject)
	if err != nil {
		return fmt.Errorf("migrating daily story tags: %w", err)
	}
	if err := s.SaveProjects(projects); err != nil {
		return fmt.Errorf("rewriting %s: %w", s.ProjectsPath(), err)
	}
	slog.Info("migrated stories to projects",
		"projects", len(projects), "stories", len(storyToProject), "daily_files_updated", rewrittenDaily)
	return nil
}

// migrateDailyStoryTags rewrites every daily file's depth-0 plan items whose
// trailing tag names a former story ID, replacing it with the parent
// project's ID. Text, state and order are otherwise untouched; items keep
// depth 0. Returns the number of files actually rewritten.
func (s *Store) migrateDailyStoryTags(storyToProject map[string]string) (int, error) {
	storyIDs := make(map[string]bool, len(storyToProject))
	for id := range storyToProject {
		storyIDs[id] = true
	}
	paths, err := filepath.Glob(filepath.Join(s.DataDir, "daily", "*", "*", "*.md"))
	if err != nil {
		return 0, fmt.Errorf("listing daily files: %w", err)
	}
	rewritten := 0
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			return rewritten, fmt.Errorf("reading %s: %w", path, err)
		}
		d, err := parseDaily(string(content))
		if err != nil {
			return rewritten, fmt.Errorf("parsing %s: %w", path, err)
		}
		changed := false
		for i := range d.Plan {
			item := &d.Plan[i]
			if item.Depth != 0 {
				continue
			}
			clean, storyID := splitProjectTag(item.Text, storyIDs)
			if storyID == "" {
				continue
			}
			item.Text = strings.TrimSpace(clean + " @" + storyToProject[storyID])
			changed = true
		}
		if !changed {
			continue
		}
		if err := writeFile(path, d.render()); err != nil {
			return rewritten, fmt.Errorf("writing %s: %w", path, err)
		}
		rewritten++
	}
	return rewritten, nil
}

// backupBeforeMigration copies projects.md and the whole daily/ tree into
// <DataDir>/.pre-subtasks-backup/, atomically (build under a .tmp sibling,
// then rename). A pre-existing backup is left untouched: this only ever runs
// once per data directory, and never deletes or overwrites user data.
func (s *Store) backupBeforeMigration() error {
	backupDir := filepath.Join(s.DataDir, backupDirName)
	if _, err := os.Stat(backupDir); err == nil {
		return nil // never overwrite an existing backup.
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("checking %s: %w", backupDir, err)
	}

	tmp := backupDir + ".tmp"
	if err := os.RemoveAll(tmp); err != nil {
		return fmt.Errorf("clearing stale %s: %w", tmp, err)
	}
	if err := copyFileIfExists(s.ProjectsPath(), filepath.Join(tmp, "projects.md")); err != nil {
		return err
	}
	if err := copyTree(filepath.Join(s.DataDir, "daily"), filepath.Join(tmp, "daily")); err != nil {
		return err
	}
	return os.Rename(tmp, backupDir)
}

// copyFileIfExists copies src to dst (creating dst's parent directories), or
// is a silent no-op when src does not exist.
func copyFileIfExists(src, dst string) error {
	data, err := os.ReadFile(src)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading %s: %w", src, err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(dst), err)
	}
	if err := os.WriteFile(dst, data, 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", dst, err)
	}
	return nil
}

// copyTree recursively copies srcDir to dstDir, or is a silent no-op when
// srcDir does not exist (e.g. no daily files yet).
func copyTree(srcDir, dstDir string) error {
	if _, err := os.Stat(srcDir); os.IsNotExist(err) {
		return nil
	}
	return filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, rerr := filepath.Rel(srcDir, path)
		if rerr != nil {
			return rerr
		}
		target := filepath.Join(dstDir, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o750)
		}
		return copyFileIfExists(path, target)
	})
}
