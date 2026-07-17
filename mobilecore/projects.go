package mobilecore

import "github.com/cristim/daily-progress-logger/internal/store"

// projectJSON is the wire form of a project for ProjectsJSON.
type projectJSON struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"` // "open" or "closed"
}

// AddProject creates a project and returns its ID.
func (c *Core) AddProject(name string) (string, error) {
	return c.store.AddProject(name)
}

// ProjectsJSON returns all projects (open and closed) as a JSON array.
// Each element has id, name, and status ("open" or "closed").
func (c *Core) ProjectsJSON() (string, error) {
	projects, err := c.store.LoadProjects()
	if err != nil {
		return "", err
	}
	out := make([]projectJSON, len(projects))
	for i, p := range projects {
		status := "open"
		if p.Status == store.StatusClosed {
			status = "closed"
		}
		out[i] = projectJSON{ID: p.ID, Name: p.Name, Status: status}
	}
	return toJSON(out)
}

// RenameProject changes the display name of the project with the given ID.
func (c *Core) RenameProject(id, newName string) error {
	return c.store.RenameProject(id, newName)
}

// CloseProject archives the project with the given ID.
func (c *Core) CloseProject(id string) error {
	return c.store.SetProjectStatus(id, store.StatusClosed)
}

// ReopenProject re-opens an archived project.
func (c *Core) ReopenProject(id string) error {
	return c.store.SetProjectStatus(id, store.StatusOpen)
}
