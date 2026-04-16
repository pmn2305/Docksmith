package main

import (
	"fmt"
	"os"

	"docksmith/internal/builder"
	"docksmith/internal/image"
	"docksmith/internal/runtime"
)

func main() {
	// Child process mode — must be checked before anything else
	if os.Getenv("DOCKSMITH_CHILD") == "1" {
		runtime.ChildMain()
		return
	}

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}
	switch os.Args[1] {
	case "build":
		builder.RunBuild(os.Args[2:])
	case "images":
		image.RunImages()
	case "rmi":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: docksmith rmi <name:tag>")
			os.Exit(1)
		}
		image.RunRmi(os.Args[2])
	case "run":
		runtime.RunContainer(os.Args[2:])
	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`Usage:
  docksmith build -t <name:tag> [--no-cache] <context>
  docksmith images
  docksmith rmi <name:tag>
  docksmith run [-e KEY=VAL] <name:tag> [cmd...]`)
}
