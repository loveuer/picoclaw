package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/constants"
	"github.com/sipeed/picoclaw/pkg/isolation"
	"github.com/sipeed/picoclaw/pkg/media"
	"github.com/sipeed/picoclaw/pkg/memory"
	"github.com/sipeed/picoclaw/pkg/providers"
	"github.com/sipeed/picoclaw/pkg/tools"
)

func remoteMemoryTurn(ts *turnState) bool {
	return ts.channel != "" && !constants.IsInternalChannel(ts.channel) || ts.opts.privateMemory
}

func memoryToolAvailable(ts *turnState, name string) bool {
	if name == memoryWriteName {
		return ts.opts.privateMemory && ts.opts.memoryManagement != nil && !ts.opts.NoHistory
	}
	if ts.opts.memoryManagement != nil && name != memorySearchName {
		return false
	}
	if name == memorySearchName {
		return ts.opts.privateMemory
	}
	if !remoteMemoryTurn(ts) {
		return true
	}
	switch name {
	case "exec":
		return isolation.RemoteExecAvailable()
	case "spawn",
		"spawn_status",
		"subagent",
		"delegate",
		tools.RegexSearchToolName,
		tools.BM25SearchToolName,
		"install_skill":
		return false
	}
	if strings.HasPrefix(name, "mcp_") {
		tool, ok := ts.agent.Tools.Get(name)
		if !ok {
			return false
		}
		remote, ok := tool.(interface{ MemorySafeRemoteTransport() bool })
		return ok && remote.MemorySafeRemoteTransport() && !ts.opts.privateMemory
	}
	return true
}

func memoryToolPolicy(
	ts *turnState,
	name string,
	args map[string]any,
	current []providers.Message,
	store media.MediaStore,
	artifacts map[string]bool,
) error {
	if !memoryToolAvailable(ts, name) {
		return fmt.Errorf("tool unavailable under the conversation's memory isolation policy")
	}
	if !remoteMemoryTurn(ts) {
		return nil
	}
	switch name {
	case "read_file", "write_file", "append_file", "edit_file", "send_file", "load_image", "list_dir":
		p, _ := args["path"].(string)
		if p == "" && name == "list_dir" {
			p = "."
		}
		if name == "send_file" && (knownTurnArtifact(artifacts, p) || currentTurnMediaPath(store, current, p)) {
			return nil
		}
		return workspaceToolPath(ts.agent.Workspace, p)
	case "message", "reaction":
		if ch, _ := args["channel"].(string); ch != "" && ch != ts.channel {
			return fmt.Errorf("message cannot change channel")
		}
		if chat, _ := args["chat_id"].(string); chat != "" && chat != ts.chatID {
			return fmt.Errorf("message cannot change chat")
		}
		for _, key := range []string{"reply_to_message_id", "message_id"} {
			if id, _ := args[key].(string); id != "" && id != ts.opts.Dispatch.MessageID() &&
				id != ts.opts.Dispatch.ReplyToMessageID() {
				return fmt.Errorf("message ID is outside the inbound context")
			}
		}
		if items, ok := args["media"].([]any); ok {
			for _, x := range items {
				if part, ok := x.(map[string]any); ok {
					p, _ := part["path"].(string)
					if err := workspaceToolPath(ts.agent.Workspace, p); err != nil {
						return err
					}
				}
			}
		}
	case "cron":
		if command, _ := args["command"].(string); command != "" {
			return fmt.Errorf("scheduled shell commands are unavailable from IM")
		}
	case "exec":
		p, _ := args["cwd"].(string)
		if p == "" {
			p = "."
		}
		if err := workspaceToolPath(ts.agent.Workspace, p); err != nil {
			return err
		}
	}
	return nil
}

func resolvedPath(path string) (string, error) {
	abs, e := filepath.Abs(path)
	if e != nil {
		return "", e
	}
	var tail []string
	for {
		res, e := filepath.EvalSymlinks(abs)
		if e == nil {
			for i := len(tail) - 1; i >= 0; i-- {
				res = filepath.Join(res, tail[i])
			}
			return res, nil
		}
		if !os.IsNotExist(e) {
			return "", e
		}
		if _, err := os.Lstat(abs); err == nil {
			return "", fmt.Errorf("unresolved symlink")
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return "", e
		}
		tail = append(tail, filepath.Base(abs))
		abs = parent
	}
}

func withinPath(root, path string) bool {
	rel, e := filepath.Rel(root, path)
	return e == nil && filepath.IsLocal(rel)
}

func workspaceToolPath(workspace, p string) error {
	if strings.TrimSpace(p) == "" {
		return fmt.Errorf("path is required")
	}
	root, e := resolvedPath(workspace)
	if e != nil {
		return fmt.Errorf("cannot resolve workspace")
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(root, p)
	}
	actual, e := resolvedPath(p)
	if e != nil || !withinPath(root, actual) {
		return fmt.Errorf("only files within the workspace are allowed")
	}
	memoryDir, e := resolvedPath(filepath.Dir(memory.Path(config.GetHome())))
	if e != nil || withinPath(memoryDir, actual) || withinPath(root, memoryDir) {
		return fmt.Errorf("memory storage cannot be accessed by tools")
	}
	for _, legacy := range []string{"memory", "sessions"} {
		dir, e := resolvedPath(filepath.Join(root, legacy))
		if e != nil || withinPath(dir, actual) {
			return fmt.Errorf("legacy memory files cannot be accessed by tools")
		}
	}
	return nil
}

func currentTurnMediaPath(store media.MediaStore, messages []providers.Message, path string) bool {
	if store == nil || !filepath.IsAbs(path) {
		return false
	}
	path, err := resolvedPath(path)
	if err != nil {
		return false
	}
	root, err := resolvedPath(media.TempDir())
	if err != nil || !withinPath(root, path) {
		return false
	}
	for _, m := range messages {
		for _, ref := range m.Media {
			if !strings.HasPrefix(ref, "media://") {
				continue
			}
			file, err := store.Resolve(ref)
			if err == nil {
				if actual, e := resolvedPath(file); e == nil && actual == path {
					return true
				}
			}
		}
	}
	return false
}

func turnArtifactPath(path string) string {
	if !filepath.IsAbs(path) {
		return ""
	}
	p, e := resolvedPath(path)
	if e != nil {
		return ""
	}
	root, e := resolvedPath(media.TempDir())
	if e != nil || !withinPath(root, p) {
		return ""
	}
	return p
}

func knownTurnArtifact(artifacts map[string]bool, path string) bool {
	p := turnArtifactPath(path)
	return p != "" && artifacts[p]
}
