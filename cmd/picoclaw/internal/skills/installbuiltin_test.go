package skills

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewInstallbuiltinSubcommand(t *testing.T) {
	cmd := newInstallBuiltinCommand(nil)

	require.NotNil(t, cmd)

	assert.Equal(t, "install-builtin [name...]", cmd.Use)
	assert.Equal(t, "Install builtin skills to workspace (preserve existing by default)", cmd.Short)

	assert.Nil(t, cmd.Run)
	assert.NotNil(t, cmd.RunE)

	assert.True(t, cmd.HasExample())
	assert.False(t, cmd.HasSubCommands())

	assert.True(t, cmd.HasFlags())

	assert.Len(t, cmd.Aliases, 0)
}
