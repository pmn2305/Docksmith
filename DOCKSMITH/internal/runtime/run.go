package runtime

import (
	"fmt"
	"os"
	"strings"

	"docksmith/internal/cache"
	"docksmith/internal/image"
)

func RunContainer(args []string) {
	// parse -e flags
	var envOverrides []string
	var rest []string
	for i := 0; i < len(args); i++ {
		if args[i] == "-e" && i+1 < len(args) {
			envOverrides = append(envOverrides, args[i+1])
			i++
		} else if strings.HasPrefix(args[i], "-e=") {
			envOverrides = append(envOverrides, strings.TrimPrefix(args[i], "-e="))
		} else {
			rest = append(rest, args[i])
		}
	}
	if len(rest) == 0 {
		fmt.Fprintln(os.Stderr, "usage: docksmith run [-e K=V] <name:tag> [cmd...]")
		os.Exit(1)
	}

	name, tag := image.ParseNameTag(rest[0])
	cmdOverride := rest[1:]

	m, err := image.LoadManifest(name, tag)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	command := m.Config.Cmd
	if len(cmdOverride) > 0 {
		command = cmdOverride
	}
	if len(command) == 0 {
		fmt.Fprintln(os.Stderr, "error: no CMD defined and no command given")
		os.Exit(1)
	}

	// Build env: image env first, then overrides win
	envMap := cache.EnvSliceToMap(m.Config.Env)
	for _, kv := range envOverrides {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}
	envSlice := cache.EnvMapToSlice(envMap)

	// Assemble rootfs in temp dir
	rootfs, err := os.MkdirTemp("", "docksmith-run-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, "error creating temp dir:", err)
		os.Exit(1)
	}
	defer os.RemoveAll(rootfs)

	if err := AssembleLayers(m.Layers, rootfs); err != nil {
		fmt.Fprintln(os.Stderr, "error assembling layers:", err)
		os.Exit(1)
	}

	if err := RunInRoot(rootfs, command, envSlice, m.Config.WorkingDir); err != nil {
		fmt.Fprintln(os.Stderr, "container exited with error:", err)
		os.Exit(1)
	}
}
