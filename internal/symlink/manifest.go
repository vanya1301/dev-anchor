package symlink

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type ManifestEntry struct {
	Target     string `json:"target"`
	Source     string `json:"source"`
	BackupPath string `json:"backup_path,omitempty"`
}

type ManifestEntries []ManifestEntry

func LoadManifest(path string) (ManifestEntries, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return ManifestEntries{}, nil
	}
	if err != nil {
		return nil, err
	}
	var m ManifestEntries
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func (m ManifestEntries) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}
