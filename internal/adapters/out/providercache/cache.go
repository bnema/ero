package providercache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"ero/internal/core"
)

// Store persists normalized provider snapshots under cache and preferences under config.
type Store struct {
	cacheDir  string
	configDir string
}

func NewStore(cacheDir, configDir string) *Store {
	return &Store{cacheDir: cacheDir, configDir: configDir}
}

func NewXDGStore() *Store {
	return NewStore(xdgDir("XDG_CACHE_HOME", ".cache", "ero"), xdgDir("XDG_CONFIG_HOME", ".config", "ero"))
}

func (s *Store) LoadProviderSnapshot(_ context.Context, key core.ReviewContextKey) (core.ProviderSnapshot, bool, error) {
	path := s.snapshotPath(key)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return core.ProviderSnapshot{}, false, nil
	}
	if err != nil {
		return core.ProviderSnapshot{}, false, err
	}
	var snapshot core.ProviderSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return core.ProviderSnapshot{}, false, err
	}
	return snapshot, true, nil
}

func (s *Store) SaveProviderSnapshot(_ context.Context, snapshot core.ProviderSnapshot) error {
	return writeJSONAtomic(s.snapshotPath(snapshot.ContextKey), snapshot)
}

func (s *Store) LoadActiveProviderKey(_ context.Context, repositoryIdentity string) (string, bool, error) {
	data, err := os.ReadFile(s.preferencePath(repositoryIdentity))
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	var pref struct {
		StableProviderKey string `json:"stable_provider_key"`
	}
	if err := json.Unmarshal(data, &pref); err != nil {
		return "", false, err
	}
	if pref.StableProviderKey == "" {
		return "", false, nil
	}
	return pref.StableProviderKey, true, nil
}

func (s *Store) SaveActiveProviderKey(_ context.Context, repositoryIdentity string, stableProviderKey string) error {
	pref := struct {
		RepositoryIdentity string `json:"repository_identity"`
		StableProviderKey  string `json:"stable_provider_key"`
	}{repositoryIdentity, stableProviderKey}
	return writeJSONAtomic(s.preferencePath(repositoryIdentity), pref)
}

func (s *Store) snapshotPath(key core.ReviewContextKey) string {
	return filepath.Join(s.cacheDir, "provider-snapshots", safeName(key.StableProviderKey), key.Digest()+".json")
}

func (s *Store) preferencePath(repositoryIdentity string) string {
	return filepath.Join(s.configDir, "provider-preferences", safeName(repositoryIdentity)+".json")
}

func writeJSONAtomic(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func safeName(raw string) string {
	replacer := strings.NewReplacer("/", "_", "\\", "_", ":", "_", "#", "_", " ", "_")
	name := replacer.Replace(raw)
	if len(name) <= 80 {
		return name
	}
	sum := sha256.Sum256([]byte(raw))
	return name[:40] + "-" + hex.EncodeToString(sum[:8])
}

func xdgDir(envVar, defaultBase, appName string) string {
	if dir := os.Getenv(envVar); dir != "" {
		return filepath.Join(dir, appName)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", appName)
	}
	return filepath.Join(home, defaultBase, appName)
}
