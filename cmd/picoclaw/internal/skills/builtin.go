package skills

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	picoclaw "github.com/sipeed/picoclaw"
)

func builtinSkillNames() ([]string, error) {
	entries, err := fs.ReadDir(picoclaw.OnboardWorkspace, "workspace/skills")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := fs.Stat(picoclaw.OnboardWorkspace, "workspace/skills/"+entry.Name()+"/SKILL.md"); err != nil {
			return nil, err
		}
		names = append(names, entry.Name())
	}
	return names, nil
}

func listBuiltinSkills(out io.Writer) error {
	names, err := builtinSkillNames()
	if err != nil {
		return err
	}
	for _, name := range names {
		data, err := picoclaw.OnboardWorkspace.ReadFile("workspace/skills/" + name + "/SKILL.md")
		if err != nil {
			return err
		}
		var meta struct {
			Description string `yaml:"description"`
		}
		parts := strings.SplitN(string(data), "---", 3)
		if len(parts) == 3 {
			if err := yaml.Unmarshal([]byte(parts[1]), &meta); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		}
		fmt.Fprintf(out, "%s\n  %s\n", name, meta.Description)
	}
	return nil
}

func installBuiltinSkills(workspace string, names []string, force bool, out io.Writer) error {
	available, err := builtinSkillNames()
	if err != nil {
		return err
	}
	known := make(map[string]bool, len(available))
	for _, name := range available {
		known[name] = true
	}
	if len(names) == 0 {
		names = available
	}
	// Validate the whole request before writing anything (also rejects path traversal).
	for _, name := range names {
		if !known[name] {
			return fmt.Errorf("unknown builtin skill %q", name)
		}
	}
	office := false
	for _, name := range names {
		if name == "docx" || name == "xlsx" || name == "pptx" {
			office = true
		}
		if err := copyBuiltinTree(
			"workspace/skills/"+name,
			filepath.Join(workspace, "skills", name),
			force,
			out,
		); err != nil {
			return err
		}
	}
	if office {
		return copyBuiltinTree("workspace/tools/office", filepath.Join(workspace, "tools", "office"), force, out)
	}
	return nil
}

func copyBuiltinTree(source, target string, force bool, out io.Writer) error {
	if info, err := os.Lstat(target); err == nil {
		if !force {
			fmt.Fprintf(out, "Skipped existing %s\n", target)
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to replace symlink %s", target)
		}
		if removeErr := os.RemoveAll(target); removeErr != nil {
			return removeErr
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	err := fs.WalkDir(picoclaw.OnboardWorkspace, source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(path, source), "/")
		dest := filepath.Join(target, filepath.FromSlash(rel))
		if entry.IsDir() {
			return os.MkdirAll(dest, 0o755)
		}
		data, err := picoclaw.OnboardWorkspace.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dest, data, 0o644)
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Installed %s\n", target)
	return nil
}
