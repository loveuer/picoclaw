package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Opt in on a machine with the installed office runtime. Uses the same restricted
// execution tool as the gateway without relaxing its default command policies.
func TestOfficeRuntimeViaExecTool(t *testing.T) {
	workspace := os.Getenv("PICOCLAW_OFFICE_TEST_WORKSPACE")
	if workspace == "" {
		t.Skip("set PICOCLAW_OFFICE_TEST_WORKSPACE to test the installed Docker runtime")
	}
	workspace, err := filepath.Abs(workspace)
	require.NoError(t, err)
	tool, err := NewExecTool(workspace, true)
	require.NoError(t, err)
	quote := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }
	wrapper := quote(filepath.Join(workspace, "tools", "office", "office.sh"))
	result := tool.Execute(
		context.Background(),
		map[string]any{"action": "run", "command": "bash " + wrapper + " check"},
	)
	require.False(t, result.IsError, result.ForLLM)
	require.Contains(t, result.ForLLM, "Office runtime ready")
	result = tool.Execute(
		context.Background(),
		map[string]any{
			"action": "run",
			"command": "bash " + wrapper + " run -- python -c " + quote(
				"from pathlib import Path; Path('office-exec-check.txt').write_text('办公工具执行成功', encoding='utf-8')",
			),
		},
	)
	require.False(t, result.IsError, result.ForLLM)
	data, err := os.ReadFile(filepath.Join(workspace, "office-exec-check.txt"))
	require.NoError(t, err)
	require.Equal(t, "办公工具执行成功", string(data))
	require.NoError(t, os.Remove(filepath.Join(workspace, "office-exec-check.txt")))
}
