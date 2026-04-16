package runtime

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"docksmith/internal/image"
)

func AssembleLayers(layers []image.LayerEntry, destDir string) error {
	for _, l := range layers {
		path := image.LayerPath(l.Digest)
		if err := extractTar(path, destDir); err != nil {
			return fmt.Errorf("extracting layer %s: %w", l.Digest, err)
		}
	}
	return nil
}

func extractTar(tarPath, destDir string) error {
	f, err := os.Open(tarPath)
	if err != nil {
		return err
	}
	defer f.Close()

	// read magic bytes to detect gzip
	magic := make([]byte, 2)
	_, err = io.ReadFull(f, magic)
	if err != nil {
		return err
	}
	// reopen cleanly — seek back to start
	f.Seek(0, io.SeekStart)

	var reader io.Reader
	if magic[0] == 0x1f && magic[1] == 0x8b {
		gz, err := gzip.NewReader(f)
		if err != nil {
			return fmt.Errorf("gzip open: %w", err)
		}
		defer gz.Close()
		reader = gz
	} else {
		reader = f
	}

	tr := tar.NewReader(reader)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar read: %w", err)
		}

		// strip leading slash, clean path
		name := strings.TrimPrefix(filepath.Clean(hdr.Name), "/")
		if name == "" || strings.HasPrefix(name, "..") {
			continue
		}

		target := filepath.Join(destDir, name)

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(hdr.Mode)|0111); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode)|0600)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
		case tar.TypeSymlink:
			os.Remove(target)
			os.Symlink(hdr.Linkname, target)
		case tar.TypeLink:
			linkTarget := filepath.Join(destDir, filepath.Clean(strings.TrimPrefix(hdr.Linkname, "/")))
			os.Remove(target)
			os.Link(linkTarget, target)
		default:
			// skip special files (devices, sockets, etc)
		}
	}
	return nil
}
