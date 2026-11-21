package memory

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/sipeed/picoclaw/cmd/picoclaw/internal"
	"github.com/sipeed/picoclaw/pkg/agent"
	"github.com/sipeed/picoclaw/pkg/config"
	storage "github.com/sipeed/picoclaw/pkg/memory"
	"github.com/sipeed/picoclaw/pkg/pid"
)

func NewCommand() *cobra.Command {
	c := &cobra.Command{Use: "memory", Short: "Manage SQLite memory"}
	c.AddCommand(fileCommand("backup", false), fileCommand("restore", true))
	return c
}

func fileCommand(name string, restore bool) *cobra.Command {
	return &cobra.Command{
		Use:  name + " <file>",
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			home := config.GetHome()
			p := storage.Path(home)
			cfg, e := internal.LoadConfig()
			if e != nil {
				return e
			}
			if e = agent.CheckMemoryArchivePath(cfg, args[0]); e != nil {
				return e
			}
			if restore {
				if pid.ReadPidFileWithCheck(home) != nil {
					return fmt.Errorf("stop the Gateway before restoring memory")
				}
				release, lockErr := storage.Lease(p, true)
				if lockErr != nil {
					return lockErr
				}
				defer release()
				before, restoreErr := storage.Restore(c.Context(), p, args[0])
				if restoreErr != nil {
					return restoreErr
				}
				fmt.Fprintln(c.OutOrStdout(), "Memory restored. Previous snapshot:", before)
				return nil
			}
			release, e := storage.Lease(p, false)
			if e != nil {
				return e
			}
			defer release()
			if e = storage.Backup(c.Context(), p, args[0]); e != nil {
				return e
			}
			fmt.Fprintln(c.OutOrStdout(), "Memory snapshot written:", args[0])
			return nil
		},
	}
}
