package store

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/cristim/daily-progress-logger/internal/recur"
)

// MigrateRefTags rewrites legacy " @<id>" project ref tags to the canonical
// " #<id>" form across all data files (daily, weekly, backlog, recycle bin,
// recurring templates). It is safe to call on every startup: when no files
// carry the old format the function returns immediately without touching any
// file (idempotent). When files do need rewriting, the relevant data files
// are backed up to <dataDir>/.pre-hashtag-backup first (the backup directory
// is created at most once; later runs that find nothing to migrate skip the
// backup step). Files that cannot be read are logged and skipped; files that
// cannot be written cause MigrateRefTags to return an error.
func (s *Store) MigrateRefTags() error {
	projects, err := s.LoadProjects()
	if err != nil {
		return fmt.Errorf("MigrateRefTags: loading projects: %w", err)
	}
	known := allIDs(projects)
	if len(known) == 0 {
		return nil // no projects defined, nothing to migrate
	}

	paths, err := s.migrateRefTagFilePaths()
	if err != nil {
		return err
	}

	// Identify which files carry legacy " @<knownid>" ref tags.
	var toMigrate []string
	for _, p := range paths {
		if fileHasLegacyRefTag(p, known) {
			toMigrate = append(toMigrate, p)
		}
	}
	if len(toMigrate) == 0 {
		return nil // nothing to do; already migrated or no ref tags at all
	}

	// Back up the data files we scanned, once. If the backup dir already exists
	// the migration was performed on a previous startup.
	backupDir := filepath.Join(s.DataDir, ".pre-hashtag-backup")
	if _, serr := os.Stat(backupDir); os.IsNotExist(serr) {
		if berr := backupRefTagFiles(s.DataDir, backupDir, paths); berr != nil {
			return fmt.Errorf("MigrateRefTags: backup: %w", berr)
		}
		slog.Info("MigrateRefTags: data files backed up", "dir", backupDir)
	}

	// Rewrite the files that need migration.
	migrated := 0
	for _, p := range toMigrate {
		if rerr := migrateRefTagFile(p, known); rerr != nil {
			slog.Warn("MigrateRefTags: skipping file", "path", p, "error", rerr)
			continue
		}
		migrated++
	}
	slog.Info("MigrateRefTags: migrated legacy @ref tags to #ref", "files", migrated)
	return nil
}

// migrateRefTagFilePaths returns the paths of all data files that may carry
// ref tags: daily files, weekly files, backlog, recycle bin, and recurring
// templates. Missing files are silently omitted.
func (s *Store) migrateRefTagFilePaths() ([]string, error) {
	var paths []string

	dailyGlob := filepath.Join(s.DataDir, "daily", "*", "*", "*.md")
	dailies, err := filepath.Glob(dailyGlob)
	if err != nil {
		return nil, fmt.Errorf("MigrateRefTags: listing daily files: %w", err)
	}
	paths = append(paths, dailies...)

	weeklyGlob := filepath.Join(s.DataDir, "weekly", "*", "*.md")
	weeklies, err := filepath.Glob(weeklyGlob)
	if err != nil {
		return nil, fmt.Errorf("MigrateRefTags: listing weekly files: %w", err)
	}
	paths = append(paths, weeklies...)

	for _, name := range []string{"backlog.md", "recycle.md", recurringFileName} {
		p := filepath.Join(s.DataDir, name)
		if _, serr := os.Stat(p); serr == nil {
			paths = append(paths, p)
		}
	}
	return paths, nil
}

// fileHasLegacyRefTag reports whether the file at path contains any line whose
// last token is " @<known_id>" (i.e. a legacy ref tag that needs migration).
func fileHasLegacyRefTag(path string, known map[string]bool) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		if migrateLineRefTag(line, known) != line {
			return true
		}
	}
	return false
}

// migrateRefTagFile rewrites path in place, replacing each trailing
// " @<known_id>" with " #<known_id>". Uses an atomic write (write-to-tmp,
// rename) so the file is never left in a partial state.
func migrateRefTagFile(path string, known map[string]bool) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	lines := strings.Split(string(data), "\n")
	changed := false
	for i, line := range lines {
		if migrated := migrateLineRefTag(line, known); migrated != line {
			lines[i] = migrated
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return writeFile(path, strings.Join(lines, "\n"))
}

// migrateLineRefTag replaces a trailing " @<known_id>" with " #<known_id>" on
// a single text line. It mirrors the splitProjectTag logic: only the very last
// token is examined; non-ref tokens (unknown ids, recurrence keywords, times)
// are left untouched because they are not in the known-id set.
//
// Belt-and-braces guard: even if a project happens to carry a slug that
// collides with a recurrence keyword (possible with hand-edited projects.md
// predating the H1 fix), that candidate is never rewritten — it is a
// recurrence token, not a project ref, and rewriting it would permanently
// destroy the template's schedule.
func migrateLineRefTag(line string, known map[string]bool) string {
	trimmed := strings.TrimRight(line, " \t")
	sep := strings.LastIndexByte(trimmed, ' ')
	last := trimmed[sep+1:]
	if !strings.HasPrefix(last, "@") {
		return line
	}
	candidate := last[1:]
	if !known[candidate] || recur.IsReservedSlug(candidate) {
		return line
	}
	// Replace the trailing "@<id>" with "#<id>", preserving the rest of the line.
	if sep < 0 {
		return "#" + candidate
	}
	return trimmed[:sep+1] + "#" + candidate
}

// backupRefTagFiles copies each file in paths (rooted under src) to the
// corresponding path under dst, creating parent directories as needed. Files
// that cannot be read are skipped with a warning; an error creating a backup
// destination is fatal.
func backupRefTagFiles(src, dst string, paths []string) error {
	for _, srcPath := range paths {
		rel, err := filepath.Rel(src, srcPath)
		if err != nil {
			slog.Warn("MigrateRefTags: backup: cannot relativize path", "path", srcPath, "error", err)
			continue
		}
		dstPath := filepath.Join(dst, rel)
		if err := copyFileContents(srcPath, dstPath); err != nil {
			return fmt.Errorf("copying %s: %w", srcPath, err)
		}
	}
	return nil
}

// copyFileContents copies src to dst using an atomic write, creating parent
// directories as needed.
func copyFileContents(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		// File disappeared between the Glob and the copy - benign.
		slog.Warn("MigrateRefTags: backup: skipping unreadable file", "path", src, "error", err)
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return fmt.Errorf("creating dir for %s: %w", dst, err)
	}
	//nolint:gosec // dst is constructed from s.DataDir (user-configured data dir), not user input.
	return os.WriteFile(dst, data, 0o600)
}
