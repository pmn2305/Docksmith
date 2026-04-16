package image

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type LayerEntry struct {
	Digest    string `json:"digest"`
	Size      int64  `json:"size"`
	CreatedBy string `json:"createdBy"`
}

type Config struct {
	Env        []string `json:"Env"`
	Cmd        []string `json:"Cmd"`
	WorkingDir string   `json:"WorkingDir"`
}

type Manifest struct {
	Name    string       `json:"name"`
	Tag     string       `json:"tag"`
	Digest  string       `json:"digest"`
	Created string       `json:"created"`
	Config  Config       `json:"config"`
	Layers  []LayerEntry `json:"layers"`
}

func DocksmithDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".docksmith")
}

func ImagesDir() string  { return filepath.Join(DocksmithDir(), "images") }
func LayersDir() string  { return filepath.Join(DocksmithDir(), "layers") }
func CacheDir() string   { return filepath.Join(DocksmithDir(), "cache") }

func Init() error {
	for _, d := range []string{ImagesDir(), LayersDir(), CacheDir()} {
		if err := os.MkdirAll(d, 0755); err != nil {
			return err
		}
	}
	return nil
}

func ManifestPath(name, tag string) string {
	return filepath.Join(ImagesDir(), name+":"+tag+".json")
}

func LayerPath(digest string) string {
	hex := strings.TrimPrefix(digest, "sha256:")
	return filepath.Join(LayersDir(), "sha256:"+hex+".tar")
}

func ComputeManifestDigest(m *Manifest) (string, error) {
	tmp := *m
	tmp.Digest = ""
	b, err := json.Marshal(tmp)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return fmt.Sprintf("sha256:%x", sum), nil
}

func WriteManifest(m *Manifest) error {
	if err := Init(); err != nil {
		return err
	}
	digest, err := ComputeManifestDigest(m)
	if err != nil {
		return err
	}
	m.Digest = digest
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(ManifestPath(m.Name, m.Tag), b, 0644)
}

func LoadManifest(name, tag string) (*Manifest, error) {
	b, err := os.ReadFile(ManifestPath(name, tag))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("image %s:%s not found in local store", name, tag)
		}
		return nil, err
	}
	var m Manifest
	return &m, json.Unmarshal(b, &m)
}

func ParseNameTag(s string) (name, tag string) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return parts[0], "latest"
}

func RunImages() {
	if err := Init(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	entries, err := os.ReadDir(ImagesDir())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("%-20s %-10s %-14s %s\n", "NAME", "TAG", "ID", "CREATED")
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(ImagesDir(), e.Name()))
		if err != nil {
			continue
		}
		var m Manifest
		if err := json.Unmarshal(b, &m); err != nil {
			continue
		}
		id := strings.TrimPrefix(m.Digest, "sha256:")
		if len(id) > 12 {
			id = id[:12]
		}
		t, _ := time.Parse(time.RFC3339, m.Created)
		fmt.Printf("%-20s %-10s %-14s %s\n", m.Name, m.Tag, id, t.Format("2006-01-02 15:04"))
	}
}

func RunRmi(nameTag string) {
	name, tag := ParseNameTag(nameTag)
	m, err := LoadManifest(name, tag)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	for _, l := range m.Layers {
		path := LayerPath(l.Digest)
		os.Remove(path)
	}
	if err := os.Remove(ManifestPath(name, tag)); err != nil {
		fmt.Fprintln(os.Stderr, "error removing manifest:", err)
		os.Exit(1)
	}
	fmt.Printf("Removed %s:%s\n", name, tag)
}
