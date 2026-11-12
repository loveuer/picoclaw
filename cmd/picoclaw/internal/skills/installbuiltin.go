package skills

import "github.com/spf13/cobra"

func newInstallBuiltinCommand(workspaceFn func() (string, error)) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:     "install-builtin [name...]",
		Short:   "Install builtin skills to workspace (preserve existing by default)",
		Example: `picoclaw skills install-builtin`,
		RunE: func(command *cobra.Command, names []string) error {
			workspace, err := workspaceFn()
			if err != nil {
				return err
			}
			return installBuiltinSkills(workspace, names, force, command.OutOrStdout())
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Replace existing builtin skills and shared resources")
	return cmd
}
