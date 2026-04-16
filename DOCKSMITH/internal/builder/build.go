package builder

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"docksmith/internal/cache"
	"docksmith/internal/image"
	"docksmith/internal/parser"
	"docksmith/internal/runtime"
)

type BuildState struct {
	Layers     []image.LayerEntry
	EnvMap     map[string]string
	WorkDir    string
	Cmd        []string
	PrevDigest string
	CacheMiss  bool
}

func RunBuild(args []string) {
	var tag string
	var noCache bool
	var contextDir string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-t":
			i++
			tag = args[i]
		case "--no-cache":
			noCache = true
		default:
			contextDir = args[i]
		}
	}
	if tag == "" || contextDir == "" {
		fmt.Fprintln(os.Stderr, "usage: docksmith build -t <name:tag> [--no-cache] <context>")
		os.Exit(1)
	}

	name, imgTag := image.ParseNameTag(tag)
	docksmithfilePath := filepath.Join(contextDir, "Docksmithfile")

	instructions, err := parser.ParseFile(docksmithfilePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "parse error:", err)
		os.Exit(1)
	}

	if err := image.Init(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	cacheIdx, err := cache.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "cache load error:", err)
		os.Exit(1)
	}

	state := &BuildState{
		EnvMap: make(map[string]string),
	}

	totalStart := time.Now()
	stepTotal := len(instructions)
	allHits := true
	existingManifest, _ := image.LoadManifest(name, imgTag)

	for stepIdx, instr := range instructions {
		stepNum := stepIdx + 1

		switch instr.Type {

		case parser.FROM:
			fmt.Printf("Step %d/%d : FROM %s\n", stepNum, stepTotal, instr.Args)
			fromName, fromTag := image.ParseNameTag(instr.Args)
			m, err := image.LoadManifest(fromName, fromTag)
			if err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
				os.Exit(1)
			}
			state.Layers = append([]image.LayerEntry{}, m.Layers...)
			state.PrevDigest = m.Digest
			for _, kv := range m.Config.Env {
				parts := strings.SplitN(kv, "=", 2)
				if len(parts) == 2 {
					state.EnvMap[parts[0]] = parts[1]
				}
			}

		case parser.WORKDIR:
			state.WorkDir = instr.Args
			fmt.Printf("Step %d/%d : WORKDIR %s\n", stepNum, stepTotal, instr.Args)

		case parser.ENV:
			k, v, err := parser.ParseENV(instr.Args)
			if err != nil {
				fmt.Fprintf(os.Stderr, "line %d: %v\n", instr.LineNum, err)
				os.Exit(1)
			}
			state.EnvMap[k] = v
			fmt.Printf("Step %d/%d : ENV %s\n", stepNum, stepTotal, instr.Args)

		case parser.CMD:
			cmd, err := parser.ParseCMD(instr.Args)
			if err != nil {
				fmt.Fprintf(os.Stderr, "line %d: %v\n", instr.LineNum, err)
				os.Exit(1)
			}
			state.Cmd = cmd
			fmt.Printf("Step %d/%d : CMD %s\n", stepNum, stepTotal, instr.Args)

		case parser.COPY:
			src, dest, err := parser.ParseCOPY(instr.Args)
			if err != nil {
				fmt.Fprintf(os.Stderr, "line %d: %v\n", instr.LineNum, err)
				os.Exit(1)
			}
			instrText := string(instr.Type) + " " + instr.Args

			// collect all source files (handles files and directories)
			srcFiles, err := collectSources(contextDir, src)
			if err != nil {
				fmt.Fprintf(os.Stderr, "line %d: COPY error: %v\n", instr.LineNum, err)
				os.Exit(1)
			}

			// hash each source file for cache key
			var srcHashes []string
			for _, sf := range srcFiles {
				info, err := os.Stat(sf)
				if err != nil || info.IsDir() {
					continue
				}
				h, err := cache.HashFile(sf)
				if err != nil {
					fmt.Fprintln(os.Stderr, err)
					os.Exit(1)
				}
				srcHashes = append(srcHashes, sf+":"+h)
			}
			sort.Strings(srcHashes)

			cacheKey := cache.ComputeKey(state.PrevDigest, instrText, state.WorkDir, state.EnvMap, srcHashes)

			stepStart := time.Now()
			fmt.Printf("Step %d/%d : %s ", stepNum, stepTotal, instrText)

			var layerDigest string
			var layerSize int64

			if !noCache && !state.CacheMiss {
				if d, hit := cacheIdx.Lookup(cacheKey); hit {
					layerDigest = d
					if info, err := os.Stat(image.LayerPath(d)); err == nil {
						layerSize = info.Size()
					}
					fmt.Printf("[CACHE HIT] %.2fs\n", time.Since(stepStart).Seconds())
				}
			}

			if layerDigest == "" {
				state.CacheMiss = true
				allHits = false

				// stage files into a temp dir mirroring dest structure
				stagingDir, err := os.MkdirTemp("", "docksmith-copy-*")
				if err != nil {
					fmt.Fprintln(os.Stderr, err)
					os.Exit(1)
				}
				defer os.RemoveAll(stagingDir)

				srcAbs := filepath.Join(contextDir, src)
				srcInfo, err := os.Stat(srcAbs)
				if err != nil {
					fmt.Fprintln(os.Stderr, err)
					os.Exit(1)
				}

				if srcInfo.IsDir() {
					// COPY dir/ /dest/ — walk and preserve structure
					err = filepath.Walk(srcAbs, func(path string, info os.FileInfo, werr error) error {
						if werr != nil {
							return werr
						}
						rel, _ := filepath.Rel(srcAbs, path)
						target := filepath.Join(stagingDir, dest, rel)
						if info.IsDir() {
							return os.MkdirAll(target, 0755)
						}
						os.MkdirAll(filepath.Dir(target), 0755)
						return copyFile(path, target)
					})
				} else {
					// COPY file /dest
					target := filepath.Join(stagingDir, dest)
					os.MkdirAll(filepath.Dir(target), 0755)
					err = copyFile(srcAbs, target)
				}
				if err != nil {
					fmt.Fprintln(os.Stderr, "COPY error:", err)
					os.Exit(1)
				}

				layerDigest, layerSize, err = WriteLayerFromDir(stagingDir, "")
				if err != nil {
					fmt.Fprintln(os.Stderr, "layer write error:", err)
					os.Exit(1)
				}
				if !noCache {
					cacheIdx.Store(cacheKey, layerDigest)
					cacheIdx.Save()
				}
				fmt.Printf("[CACHE MISS] %.2fs\n", time.Since(stepStart).Seconds())
			}

			state.Layers = append(state.Layers, image.LayerEntry{
				Digest:    layerDigest,
				Size:      layerSize,
				CreatedBy: instrText,
			})
			state.PrevDigest = layerDigest

		case parser.RUN:
			instrText := string(instr.Type) + " " + instr.Args
			cacheKey := cache.ComputeKey(state.PrevDigest, instrText, state.WorkDir, state.EnvMap, nil)

			stepStart := time.Now()
			fmt.Printf("Step %d/%d : %s ", stepNum, stepTotal, instrText)

			var layerDigest string
			var layerSize int64

			if !noCache && !state.CacheMiss {
				if d, hit := cacheIdx.Lookup(cacheKey); hit {
					layerDigest = d
					if info, err := os.Stat(image.LayerPath(d)); err == nil {
						layerSize = info.Size()
					}
					fmt.Printf("[CACHE HIT] %.2fs\n", time.Since(stepStart).Seconds())
				}
			}

			if layerDigest == "" {
				state.CacheMiss = true
				allHits = false

				rootfs, err := os.MkdirTemp("", "docksmith-run-*")
				if err != nil {
					fmt.Fprintln(os.Stderr, err)
					os.Exit(1)
				}
				defer os.RemoveAll(rootfs)

				if err := runtime.AssembleLayers(state.Layers, rootfs); err != nil {
					fmt.Fprintln(os.Stderr, "assemble error:", err)
					os.Exit(1)
				}

				if state.WorkDir != "" {
					os.MkdirAll(filepath.Join(rootfs, state.WorkDir), 0755)
				}

				beforeSnap, _ := snapshotDir(rootfs)

				envSlice := cache.EnvMapToSlice(state.EnvMap)
				command := []string{"/bin/sh", "-c", instr.Args}

				if err := runtime.RunInRoot(rootfs, command, envSlice, state.WorkDir); err != nil {
					fmt.Fprintf(os.Stderr, "\nRUN failed: %v\n", err)
					os.Exit(1)
				}

				afterSnap, _ := snapshotDir(rootfs)
				delta := computeDelta(beforeSnap, afterSnap, rootfs)

				layerDigest, layerSize, err = WriteLayerFromFiles(delta)
				if err != nil {
					fmt.Fprintln(os.Stderr, "layer write error:", err)
					os.Exit(1)
				}
				if !noCache {
					cacheIdx.Store(cacheKey, layerDigest)
					cacheIdx.Save()
				}
				fmt.Printf("[CACHE MISS] %.2fs\n", time.Since(stepStart).Seconds())
			}

			state.Layers = append(state.Layers, image.LayerEntry{
				Digest:    layerDigest,
				Size:      layerSize,
				CreatedBy: instrText,
			})
			state.PrevDigest = layerDigest
		}
	}

	createdAt := time.Now().UTC().Format(time.RFC3339)
	if allHits && existingManifest != nil {
		createdAt = existingManifest.Created
	}

	m := &image.Manifest{
		Name:    name,
		Tag:     imgTag,
		Created: createdAt,
		Config: image.Config{
			Env:        cache.EnvMapToSlice(state.EnvMap),
			Cmd:        state.Cmd,
			WorkingDir: state.WorkDir,
		},
		Layers: state.Layers,
	}
	if err := image.WriteManifest(m); err != nil {
		fmt.Fprintln(os.Stderr, "manifest write error:", err)
		os.Exit(1)
	}

	short := strings.TrimPrefix(m.Digest, "sha256:")
	if len(short) > 8 {
		short = short[:8]
	}
	fmt.Printf("Successfully built sha256:%s %s:%s (%.2fs)\n", short, name, imgTag, time.Since(totalStart).Seconds())
}

func snapshotDir(dir string) (map[string]string, error) {
	snap := map[string]string{}
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		if rel == "." {
			return nil
		}
		snap[rel] = fmt.Sprintf("%d:%d", info.ModTime().UnixNano(), info.Size())
		return nil
	})
	return snap, err
}

func computeDelta(before, after map[string]string, rootfs string) map[string]string {
	delta := map[string]string{}
	for rel, sig := range after {
		if before[rel] != sig {
			hostPath := filepath.Join(rootfs, rel)
			info, err := os.Stat(hostPath)
			if err == nil && !info.IsDir() {
				delta[rel] = hostPath
			}
		}
	}
	return delta
}

// collectSources returns all file paths under contextDir/src (file or dir)
func collectSources(contextDir, src string) ([]string, error) {
	srcAbs := filepath.Join(contextDir, src)
	info, err := os.Stat(srcAbs)
	if err != nil {
		// try glob
		matches, gerr := filepath.Glob(srcAbs)
		if gerr != nil || len(matches) == 0 {
			return nil, fmt.Errorf("no files matched %q", src)
		}
		return matches, nil
	}
	if !info.IsDir() {
		return []string{srcAbs}, nil
	}
	var files []string
	filepath.Walk(srcAbs, func(path string, fi os.FileInfo, e error) error {
		if e == nil {
			files = append(files, path)
		}
		return nil
	})
	return files, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
