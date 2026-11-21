package agent

import (
	"fmt"
	"path/filepath"

	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/memory"
)

func memoryWorkspaces(cfg *config.Config) []string {
	out := make([]string, 0, 1+len(cfg.Agents.List))
	out = append(out, resolveAgentWorkspace(nil, &cfg.Agents.Defaults))
	for i := range cfg.Agents.List {
		out = append(out, resolveAgentWorkspace(&cfg.Agents.List[i], &cfg.Agents.Defaults))
	}
	return out
}

func PrepareMemory(cfg *config.Config) (func(), error) {
	path := memory.Path(config.GetHome())
	if err := validateMemoryWorkspacePaths(cfg, path); err != nil {
		return nil, err
	}
	release, e := memory.Lease(path, false)
	if e != nil {
		return nil, e
	}
	db, e := memory.Open(path)
	if e != nil {
		release()
		return nil, e
	}
	if e = db.Close(); e != nil {
		release()
		return nil, e
	}
	return release, nil
}

func (al *AgentLoop) MemoryError() error {
	for _, id := range al.registry.ListAgentIDs() {
		a, _ := al.registry.GetAgent(id)
		if a != nil && a.memoryErr != nil {
			return fmt.Errorf("agent %s: %w", id, a.memoryErr)
		}
	}
	return nil
}

func validateMemoryWorkspacePaths(cfg *config.Config, path string) error {
	dir, e := resolvedPath(filepath.Dir(path))
	if e != nil {
		return e
	}
	for _, ws := range memoryWorkspaces(cfg) {
		root, e := resolvedPath(ws)
		if e != nil {
			return e
		}
		if withinPath(root, dir) || withinPath(dir, root) {
			return fmt.Errorf("workspace must not overlap the protected memory directory: %s", ws)
		}
	}
	return nil
}

// CheckMemoryArchivePath rejects snapshots that could be read as work files.
func CheckMemoryArchivePath(cfg *config.Config, path string) error {
	path, e := resolvedPath(path)
	if e != nil {
		return e
	}
	for _, ws := range memoryWorkspaces(cfg) {
		root, e := resolvedPath(ws)
		if e != nil {
			return e
		}
		if withinPath(root, path) {
			return fmt.Errorf("memory snapshots must be outside all workspaces")
		}
	}
	return nil
}
