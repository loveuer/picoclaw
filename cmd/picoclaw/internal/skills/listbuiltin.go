package skills

import "github.com/spf13/cobra"

func newListBuiltinCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list-builtin",
		Short:   "List available builtin skills",
		Example: `picoclaw skills list-builtin`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return listBuiltinSkills(cmd.OutOrStdout())
		},
	}

	return cmd
}
