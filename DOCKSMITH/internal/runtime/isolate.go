package runtime

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// RunInRoot runs command inside rootfs using chroot + new namespaces.
// envVars are the full environment to inject.
// workdir is relative to the root (defaults to "/").
func RunInRoot(rootfs string, command []string, envVars []string, workdir string) error {
	if workdir == "" {
		workdir = "/"
	}

	// The binary we exec must exist inside the rootfs.
	// We re-exec ourselves with a special env var to act as the "child" process
	// that does the chroot and exec inside the namespace.
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot find self executable: %w", err)
	}

	absRoot, err := filepath.Abs(rootfs)
	if err != nil {
		return err
	}

	// Build child args: rootfs, workdir, then the command
	childArgs := append([]string{absRoot, workdir}, command...)

	cmd := exec.Command(self, childArgs...)
	cmd.Env = append([]string{"DOCKSMITH_CHILD=1"}, envVars...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUTS | syscall.CLONE_NEWPID | syscall.CLONE_NEWNS,
	}

	return cmd.Run()
}

// ChildMain is called when DOCKSMITH_CHILD=1 is set.
// It does the actual chroot + exec. os.Args = [self, rootfs, workdir, cmd, args...]
func ChildMain() {
	if len(os.Args) < 4 {
		fmt.Fprintln(os.Stderr, "docksmith child: invalid args")
		os.Exit(1)
	}
	rootfs := os.Args[1]
	workdir := os.Args[2]
	command := os.Args[3:]

	if err := syscall.Chroot(rootfs); err != nil {
		fmt.Fprintf(os.Stderr, "chroot %s: %v\n", rootfs, err)
		os.Exit(1)
	}
	if err := syscall.Chdir(workdir); err != nil {
		// fallback to root
		syscall.Chdir("/")
	}

	binary, err := exec.LookPath(command[0])
	if err != nil {
		// try as literal path
		binary = command[0]
	}

	if err := syscall.Exec(binary, command, os.Environ()); err != nil {
		fmt.Fprintf(os.Stderr, "exec %v: %v\n", command, err)
		os.Exit(1)
	}
}
