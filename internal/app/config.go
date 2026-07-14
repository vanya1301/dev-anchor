package app

import (
	"path/filepath"

	"github.com/BurntSushi/toml"

	"github.com/devanchor/da/internal/symlink"
)

type Paths struct {
	Repo         string
	Configs      string
	Backup       string
	StateFile    string
	ManifestFile string
	ToolsFile    string
	SymlinksFile string
	SopsFile     string
	EncKey       string
	PubKey       string
}

func DefaultPaths(repo string) Paths {
	return Paths{
		Repo:         repo,
		Configs:      filepath.Join(repo, "configs"),
		Backup:       filepath.Join(repo, ".backup"),
		StateFile:    filepath.Join(repo, ".da", "state.json"),
		ManifestFile: filepath.Join(repo, ".da", "manifest.json"),
		ToolsFile:    filepath.Join(repo, "tools.toml"),
		SymlinksFile: filepath.Join(repo, "symlinks.toml"),
		SopsFile:     filepath.Join(repo, ".sops.yaml"),
		EncKey:       filepath.Join(repo, "age.key.enc"),
		PubKey:       filepath.Join(repo, "age.pub"),
	}
}

type linkFile struct {
	Links []struct {
		Source string `toml:"source"`
		Target string `toml:"target"`
	} `toml:"link"`
}

func LoadSymlinks(b []byte) ([]symlink.Link, error) {
	var lf linkFile
	if err := toml.Unmarshal(b, &lf); err != nil {
		return nil, err
	}
	out := make([]symlink.Link, 0, len(lf.Links))
	for _, l := range lf.Links {
		out = append(out, symlink.Link{Source: l.Source, Target: l.Target})
	}
	return out, nil
}
