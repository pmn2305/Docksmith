package cache

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"docksmith/internal/image"
)

type Index map[string]string // cacheKey -> layerDigest

func indexPath() string {
	return filepath.Join(image.CacheDir(), "index.json")
}

func Load() (Index, error) {
	b, err := os.ReadFile(indexPath())
	if os.IsNotExist(err) {
		return make(Index), nil
	}
	if err != nil {
		return nil, err
	}
	var idx Index
	return idx, json.Unmarshal(b, &idx)
}

func (idx Index) Save() error {
	b, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(indexPath(), b, 0644)
}

func (idx Index) Lookup(key string) (string, bool) {
	digest, ok := idx[key]
	if !ok {
		return "", false
	}
	// verify layer file exists
	p := image.LayerPath(digest)
	if _, err := os.Stat(p); err != nil {
		return "", false
	}
	return digest, true
}

func (idx Index) Store(key, digest string) {
	idx[key] = digest
}

// ComputeKey builds the cache key for a COPY or RUN instruction.
func ComputeKey(prevDigest, instrText, workdir string, envMap map[string]string, srcHashes []string) string {
	h := sha256.New()
	h.Write([]byte(prevDigest))
	h.Write([]byte("\x00"))
	h.Write([]byte(instrText))
	h.Write([]byte("\x00"))
	h.Write([]byte(workdir))
	h.Write([]byte("\x00"))

	// sorted env
	keys := make([]string, 0, len(envMap))
	for k := range envMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		h.Write([]byte(k + "=" + envMap[k] + "\x00"))
	}

	// sorted src hashes (COPY only)
	sort.Strings(srcHashes)
	for _, s := range srcHashes {
		h.Write([]byte(s + "\x00"))
	}

	return fmt.Sprintf("sha256:%x", h.Sum(nil))
}

// HashFile returns sha256 hex of file contents
func HashFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return fmt.Sprintf("%x", sum), nil
}

// EnvSliceToMap converts ["K=V"] to map
func EnvSliceToMap(envs []string) map[string]string {
	m := make(map[string]string)
	for _, e := range envs {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			m[parts[0]] = parts[1]
		}
	}
	return m
}

// EnvMapToSlice converts map to sorted ["K=V"]
func EnvMapToSlice(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(m))
	for _, k := range keys {
		out = append(out, k+"="+m[k])
	}
	return out
}
