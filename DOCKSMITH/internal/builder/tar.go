package builder

import (
	"archive/tar"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"docksmith/internal/image"
)

var zeroTime = time.Time{}

// WriteLayerFromFiles creates a deterministic tar. files = map[tarPath]hostPath
func WriteLayerFromFiles(files map[string]string) (digest string, size int64, err error) {
	if err := os.MkdirAll(image.LayersDir(), 0755); err != nil {
		return "", 0, err
	}

	tarPaths := make([]string, 0, len(files))
	for tp := range files {
		tarPaths = append(tarPaths, tp)
	}
	sort.Strings(tarPaths)

	tmp, err := os.CreateTemp(image.LayersDir(), "layer-*.tar.tmp")
	if err != nil {
		return "", 0, err
	}
	tmpPath := tmp.Name()
	defer func() {
		if err != nil {
			os.Remove(tmpPath)
		}
	}()

	h := sha256.New()
	mw := io.MultiWriter(tmp, h)
	tw := tar.NewWriter(mw)

	for _, tp := range tarPaths {
		hp := files[tp]
		info, statErr := os.Lstat(hp)
		if statErr != nil {
			return "", 0, statErr
		}
		if info.IsDir() {
			name := strings.TrimSuffix(tp, "/") + "/"
			hdr := &tar.Header{
				Name:     name,
				Typeflag: tar.TypeDir,
				Mode:     0755,
				ModTime:  zeroTime,
			}
			if err = tw.WriteHeader(hdr); err != nil {
				return "", 0, err
			}
			continue
		}
		hdr := &tar.Header{
			Name:     tp,
			Typeflag: tar.TypeReg,
			Mode:     int64(info.Mode()),
			Size:     info.Size(),
			ModTime:  zeroTime,
		}
		if err = tw.WriteHeader(hdr); err != nil {
			return "", 0, err
		}
		f, ferr := os.Open(hp)
		if ferr != nil {
			return "", 0, ferr
		}
		if _, err = io.Copy(tw, f); err != nil {
			f.Close()
			return "", 0, err
		}
		f.Close()
	}

	if err = tw.Close(); err != nil {
		return "", 0, err
	}

	info, err := tmp.Stat()
	if err != nil {
		return "", 0, err
	}
	size = info.Size()
	tmp.Close()

	digestStr := fmt.Sprintf("sha256:%x", h.Sum(nil))
	finalPath := image.LayerPath(digestStr)
	if err = os.Rename(tmpPath, finalPath); err != nil {
		return "", 0, err
	}
	return digestStr, size, nil
}

// WriteLayerFromDir tars all files under dir with tarRoot as prefix
func WriteLayerFromDir(dir, tarRoot string) (string, int64, error) {
	files := map[string]string{}
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		if rel == "." {
			return nil
		}
		tp := filepath.Join(tarRoot, rel)
		if info.IsDir() {
			tp += "/"
		}
		files[tp] = path
		return nil
	})
	if err != nil {
		return "", 0, err
	}
	return WriteLayerFromFiles(files)
}
