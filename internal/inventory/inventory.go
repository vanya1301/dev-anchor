package inventory

import (
	"github.com/BurntSushi/toml"

	"github.com/devanchor/da/internal/pkgmgr"
	"github.com/devanchor/da/internal/platform"
	"github.com/devanchor/da/internal/runner"
)

type Tool struct {
	Name      string            `toml:"name"`
	Manager   string            `toml:"manager"`
	Package   string            `toml:"package"`
	Version   string            `toml:"version"`
	Binary    string            `toml:"binary"`
	Platform  []string          `toml:"platform"`
	Overrides map[string]string `toml:"overrides"`
}

type Inventory struct {
	Tools []Tool `toml:"tool"`
}

func Parse(b []byte) (Inventory, error) {
	var inv Inventory
	err := toml.Unmarshal(b, &inv)
	return inv, err
}

func (t Tool) AppliesTo(info platform.Info) bool {
	if len(t.Platform) == 0 {
		return true
	}
	for _, p := range t.Platform {
		if p == string(info.OS) {
			return true
		}
	}
	return false
}

func (t Tool) ResolvedPackage(manager string) string {
	if t.Overrides != nil {
		if v, ok := t.Overrides[manager]; ok {
			return v
		}
	}
	return t.Package
}

type LookPathFunc func(string) (string, error)

func Sync(inv Inventory, info platform.Info, r runner.CommandRunner, look LookPathFunc) (installed, skipped []string, err error) {
	for _, tool := range inv.Tools {
		if !tool.AppliesTo(info) {
			continue
		}
		if _, e := look(tool.Binary); e == nil {
			skipped = append(skipped, tool.Name)
			continue
		}
		pkg := tool.ResolvedPackage(tool.Manager)
		if e := pkgmgr.InstallVia(tool.Manager, pkg, r); e != nil {
			return installed, skipped, e
		}
		installed = append(installed, tool.Name)
	}
	return installed, skipped, nil
}
