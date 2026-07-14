package app

import (
	"os"

	"github.com/devanchor/da/internal/inventory"
	"github.com/devanchor/da/internal/pkgmgr"
	"github.com/devanchor/da/internal/platform"
	"github.com/devanchor/da/internal/runner"
)

// CaptureNewTools returns package names installed locally but absent from tools.toml.
func CaptureNewTools(paths Paths, r runner.CommandRunner, info platform.Info) ([]string, error) {
	mgr := pkgmgr.New(info.Manager, r)
	installedLocal, err := mgr.ListInstalled()
	if err != nil {
		return nil, err
	}
	known := map[string]bool{}
	if b, e := os.ReadFile(paths.ToolsFile); e == nil {
		inv, perr := inventory.Parse(b)
		if perr != nil {
			return nil, perr
		}
		for _, tl := range inv.Tools {
			known[tl.ResolvedPackage(string(info.Manager))] = true
			known[tl.Package] = true
		}
	}
	var newTools []string
	for _, p := range installedLocal {
		if !known[p] {
			newTools = append(newTools, p)
		}
	}
	return newTools, nil
}
