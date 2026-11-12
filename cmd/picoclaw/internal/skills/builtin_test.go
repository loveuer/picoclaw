package skills

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	picoclaw "github.com/sipeed/picoclaw"
	"github.com/sipeed/picoclaw/pkg/skills"
)

func TestBuiltinOfficeInstall(t *testing.T) {
	workspace := t.TempDir()
	require.NoError(t, installBuiltinSkills(workspace, []string{"docx", "xlsx", "pptx"}, false, io.Discard))
	loader := skills.NewSkillsLoader(workspace, "", "")
	installed := loader.ListSkills()
	require.Len(t, installed, 3)
	for _, info := range installed {
		require.NotEmpty(t, info.Description)
		require.FileExists(
			t,
			filepath.Join(workspace, "skills", info.Name, "scripts", "office", "helpers", "__init__.py"),
		)
		require.FileExists(t, filepath.Join(workspace, "skills", info.Name, "LICENSE.txt"))
	}
	require.FileExists(t, filepath.Join(workspace, "tools", "office", "office.sh"))
	require.NoDirExists(t, filepath.Join(workspace, "skills", "weather"))
	// Keep local edits, including shared resources, unless explicitly forced.
	path := filepath.Join(workspace, "skills", "docx", "SKILL.md")
	runtime := filepath.Join(workspace, "tools", "office", "office.sh")
	for _, p := range []string{path, runtime} {
		require.NoError(t, os.WriteFile(p, []byte("local edit"), 0o644))
	}
	require.NoError(t, installBuiltinSkills(workspace, []string{"docx"}, false, io.Discard))
	for _, p := range []string{path, runtime} {
		data, err := os.ReadFile(p)
		require.NoError(t, err)
		require.Equal(t, "local edit", string(data))
	}
	require.NoError(t, installBuiltinSkills(workspace, []string{"docx"}, true, io.Discard))
	for _, p := range []string{path, runtime} {
		data, err := os.ReadFile(p)
		require.NoError(t, err)
		require.NotEqual(t, "local edit", string(data))
	}
}

func TestBuiltinEnumerationAndValidation(t *testing.T) {
	names, err := builtinSkillNames()
	require.NoError(t, err)
	require.Len(t, names, 10)
	require.NotContains(t, names, "hardware")
	var out bytes.Buffer
	require.NoError(t, listBuiltinSkills(&out))
	require.Contains(t, out.String(), "docx")
	workspace := t.TempDir()
	for _, invalid := range []string{"../docx", "news", "missing"} {
		require.Error(t, installBuiltinSkills(workspace, []string{"docx", invalid}, false, io.Discard))
		require.NoDirExists(t, filepath.Join(workspace, "skills"))
	}
	require.NoError(t, installBuiltinSkills(workspace, nil, false, io.Discard))
	for _, name := range names {
		require.FileExists(t, filepath.Join(workspace, "skills", name, "SKILL.md"))
	}
	// Embedded Python package initializers must survive Go's default underscore exclusion.
	_, err = picoclaw.OnboardWorkspace.ReadFile("workspace/skills/docx/scripts/office/helpers/__init__.py")
	require.NoError(t, err)
}
