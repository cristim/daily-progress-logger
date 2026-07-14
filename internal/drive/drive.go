// Package drive is a thin, gomobile-safe wrapper over the user's Google Drive
// used to sync the app's markdown files under a single visible "DailyProgress"
// folder. It uses the least-privilege drive.file scope, so it only ever sees
// files this app created.
package drive

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	drivev3 "google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

const (
	// Scope is the OAuth scope: access only files created by this app.
	Scope = drivev3.DriveFileScope

	rootFolderName = "DailyProgress"
	folderMIME     = "application/vnd.google-apps.folder"
)

// Client talks to one user's Drive, rooted at the app's DailyProgress folder.
type Client struct {
	svc    *drivev3.Service
	rootID string
}

// File is a remote file's identity and content fingerprint. Path is relative to
// the root folder, using "/" separators, matching the local layout.
type File struct {
	Path     string
	ID       string
	MD5      string
	Modified time.Time
}

// folderNode is a folder's name and parent, for reconstructing paths.
type folderNode struct {
	name   string
	parent string
}

// New builds a Client from an authenticated HTTP client and ensures the root
// DailyProgress folder exists.
func New(ctx context.Context, httpClient *http.Client) (*Client, error) {
	svc, err := drivev3.NewService(ctx, option.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("drive service: %w", err)
	}
	c := &Client{svc: svc}
	rootID, err := c.ensureFolder(ctx, rootFolderName, "")
	if err != nil {
		return nil, err
	}
	c.rootID = rootID
	return c, nil
}

// List returns every (non-folder) file under the root, keyed by relative path.
func (c *Client) List(ctx context.Context) ([]File, error) {
	folders, raw, err := c.listAll(ctx)
	if err != nil {
		return nil, err
	}
	var files []File
	for _, f := range raw {
		if f.MimeType == folderMIME {
			continue
		}
		rel, ok := relPath(f.Name, parentOf(f), folders, c.rootID)
		if !ok {
			continue // not under our root
		}
		modified, _ := time.Parse(time.RFC3339, f.ModifiedTime)
		files = append(files, File{Path: rel, ID: f.Id, MD5: f.Md5Checksum, Modified: modified})
	}
	return files, nil
}

// Download fetches a file's bytes by ID.
func (c *Client) Download(ctx context.Context, id string) ([]byte, error) {
	resp, err := c.svc.Files.Get(id).Context(ctx).Download()
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", id, err)
	}
	defer func() { _ = resp.Body.Close() }()
	return io.ReadAll(resp.Body)
}

// Upload creates or updates the file at relative path with content, creating any
// missing parent folders, and returns its file ID. When id is non-empty it
// updates that file (used for known files); otherwise it creates a new one.
func (c *Client) Upload(ctx context.Context, relPath, id string, content []byte) (string, error) {
	dir, name := path.Split(relPath)
	parent, err := c.ensurePath(ctx, strings.Trim(dir, "/"))
	if err != nil {
		return "", err
	}
	media := bytes.NewReader(content)
	if id != "" {
		f, err := c.svc.Files.Update(id, &drivev3.File{}).Media(media).Context(ctx).Do()
		if err != nil {
			return "", fmt.Errorf("update %s: %w", relPath, err)
		}
		return f.Id, nil
	}
	f, err := c.svc.Files.Create(&drivev3.File{Name: name, Parents: []string{parent}}).
		Media(media).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("create %s: %w", relPath, err)
	}
	return f.Id, nil
}

// Delete removes a file (or folder) by ID.
func (c *Client) Delete(ctx context.Context, id string) error {
	if err := c.svc.Files.Delete(id).Context(ctx).Do(); err != nil {
		return fmt.Errorf("delete %s: %w", id, err)
	}
	return nil
}

// listAll fetches every app file/folder once and splits out the folder graph.
func (c *Client) listAll(ctx context.Context) (map[string]folderNode, []*drivev3.File, error) {
	folders := map[string]folderNode{}
	var all []*drivev3.File
	err := c.svc.Files.List().
		Q("trashed=false").
		Fields("nextPageToken, files(id,name,mimeType,parents,md5Checksum,modifiedTime)").
		PageSize(1000).
		Pages(ctx, func(fl *drivev3.FileList) error {
			for _, f := range fl.Files {
				all = append(all, f)
				if f.MimeType == folderMIME {
					folders[f.Id] = folderNode{name: f.Name, parent: parentOf(f)}
				}
			}
			return nil
		})
	if err != nil {
		return nil, nil, fmt.Errorf("listing drive files: %w", err)
	}
	return folders, all, nil
}

// driveQueryString single-quotes s for use in a Drive API query, escaping
// backslash and single-quote per the Drive query language spec (L5).
// Go's %q uses double-quote Go string escaping which is not the same as
// Drive's single-quoted escaping, so a segment containing ' or \ would
// produce an invalid/incorrect query.
func driveQueryString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `'`, `\'`)
	return "'" + s + "'"
}

// ensureFolder finds (or creates) a folder by name under parentID. An empty
// parentID means the user's My Drive root.
func (c *Client) ensureFolder(ctx context.Context, name, parentID string) (string, error) {
	q := "name = " + driveQueryString(name) +
		" and mimeType = " + driveQueryString(folderMIME) +
		" and trashed = false"
	if parentID != "" {
		q += " and " + driveQueryString(parentID) + " in parents"
	}
	list, err := c.svc.Files.List().Q(q).Fields("files(id)").PageSize(1).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("finding folder %q: %w", name, err)
	}
	if len(list.Files) > 0 {
		return list.Files[0].Id, nil
	}
	meta := &drivev3.File{Name: name, MimeType: folderMIME}
	if parentID != "" {
		meta.Parents = []string{parentID}
	}
	f, err := c.svc.Files.Create(meta).Fields("id").Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("creating folder %q: %w", name, err)
	}
	return f.Id, nil
}

// ensurePath resolves a "/"-joined relative folder path under the root, creating
// intermediate folders as needed, and returns the deepest folder's ID.
func (c *Client) ensurePath(ctx context.Context, relDir string) (string, error) {
	parent := c.rootID
	if relDir == "" {
		return parent, nil
	}
	for segment := range strings.SplitSeq(relDir, "/") {
		id, err := c.ensureFolder(ctx, segment, parent)
		if err != nil {
			return "", err
		}
		parent = id
	}
	return parent, nil
}

// parentOf returns a file's first parent ID, or "".
func parentOf(f *drivev3.File) string {
	if len(f.Parents) > 0 {
		return f.Parents[0]
	}
	return ""
}

// relPath reconstructs a file's path relative to rootID by walking the folder
// graph up from its parent. ok is false when the file is not under the root.
func relPath(name, parent string, folders map[string]folderNode, rootID string) (string, bool) {
	parts := []string{name}
	for parent != "" && parent != rootID {
		node, ok := folders[parent]
		if !ok {
			return "", false
		}
		parts = append(parts, node.name)
		parent = node.parent
	}
	if parent != rootID {
		return "", false
	}
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	return strings.Join(parts, "/"), true
}

// ConflictName inserts a " (conflict <device> <ts>)" marker before the extension
// of a "/"-separated relative path, so a conflicted copy never collides.
func ConflictName(relPath, device string, ts time.Time) string {
	dir, base := path.Split(relPath)
	ext := path.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	conflict := fmt.Sprintf("%s (conflict %s %s)%s", stem, device, ts.Format("2006-01-02 150405"), ext)
	return dir + conflict
}
