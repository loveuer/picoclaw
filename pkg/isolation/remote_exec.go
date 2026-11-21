package isolation

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/sipeed/picoclaw/pkg/config"
)

func RemoteExecAvailable() bool {
	if runtime.GOOS != "linux" || !CurrentConfig().Enabled {
		return false
	}
	_, err := exec.LookPath("bwrap")
	return err == nil
}

// StartRemoteExec uses a smaller mount view than trusted local commands.
// Never mount the instance root, configured exposed paths, or host sockets.
func StartRemoteExec(cmd *exec.Cmd, workspace string) error {
	if !RemoteExecAvailable() {
		return fmt.Errorf("IM shell requires Linux bubblewrap isolation")
	}
	bwrap, err := exec.LookPath("bwrap")
	if err != nil {
		return err
	}
	workspace, err = filepath.EvalSymlinks(workspace)
	if err != nil {
		return err
	}
	workspace, err = filepath.Abs(workspace)
	if err != nil {
		return err
	}
	home, err := filepath.EvalSymlinks(config.GetHome())
	if err != nil {
		return err
	}
	home, err = filepath.Abs(home)
	if err != nil {
		return err
	}
	data := filepath.Join(home, "state", "memory")
	for _, pair := range [][2]string{{workspace, data}, {data, workspace}} {
		rel, e := filepath.Rel(pair[0], pair[1])
		if e != nil || filepath.IsLocal(rel) {
			return fmt.Errorf("workspace overlaps protected memory storage")
		}
	}
	cwd, err := filepath.EvalSymlinks(cmd.Dir)
	if err != nil {
		return err
	}
	cwd, err = filepath.Abs(cwd)
	if err != nil {
		return err
	}
	if rel, e := filepath.Rel(workspace, cwd); e != nil || !filepath.IsLocal(rel) {
		return fmt.Errorf("working directory is outside the workspace")
	}
	args := []string{
		bwrap,
		"--die-with-parent",
		"--unshare-all",
		"--new-session",
		"--cap-drop",
		"ALL",
		"--proc",
		"/proc",
		"--dev",
		"/dev",
		"--tmpfs",
		"/tmp",
	}
	// Keep the workspace at its normal path for scripts, but no host IPC or
	// inherited credentials. The system executable directories are read-only.
	for _, p := range []string{"/usr", "/bin", "/lib", "/lib64", "/etc/alternatives", "/etc/ld.so.cache"} {
		if _, e := os.Stat(p); e == nil {
			if rel, e := filepath.Rel(p, data); e != nil || filepath.IsLocal(rel) {
				return fmt.Errorf("memory overlaps a system mount")
			}
			args = append(args, "--ro-bind", p, p)
		}
	}
	args = append(args, "--bind", workspace, workspace)
	for _, name := range []string{"memory", "sessions"} {
		p := filepath.Join(workspace, name)
		if info, e := os.Lstat(p); e == nil {
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("legacy memory path must be archived before IM shell is enabled")
			}
			args = append(args, "--tmpfs", p)
		} else if !os.IsNotExist(e) {
			return e
		}
	}
	args = append(
		args,
		"--clearenv",
		"--setenv",
		"HOME",
		workspace,
		"--setenv",
		"PATH",
		"/usr/local/bin:/usr/bin:/bin",
		"--chdir",
		cwd,
		"--",
		"/bin/sh",
	)
	args = append(args, cmd.Args[1:]...)
	cmd.Path, cmd.Args, cmd.Dir = bwrap, args, ""
	cmd.Env = []string{"PATH=/usr/local/bin:/usr/bin:/bin"}
	return cmd.Start()
}
