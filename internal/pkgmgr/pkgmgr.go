package pkgmgr

import (
	"fmt"
	"strings"

	"github.com/devanchor/da/internal/platform"
	"github.com/devanchor/da/internal/runner"
)

type Manager struct {
	Kind   platform.PkgManager
	Runner runner.CommandRunner
}

func New(kind platform.PkgManager, r runner.CommandRunner) *Manager {
	return &Manager{Kind: kind, Runner: r}
}

func (m *Manager) Install(pkg string) error {
	var name string
	var args []string
	switch m.Kind {
	case platform.Brew:
		name, args = "brew", []string{"install", pkg}
	case platform.Apt:
		name, args = "sudo", []string{"apt-get", "install", "-y", pkg}
	case platform.Dnf:
		name, args = "sudo", []string{"dnf", "install", "-y", pkg}
	case platform.Pacman:
		name, args = "sudo", []string{"pacman", "-S", "--noconfirm", pkg}
	default:
		return fmt.Errorf("unsupported manager: %s", m.Kind)
	}
	res, err := m.Runner.Run(name, args...)
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("install %s failed: %s", pkg, res.Stderr)
	}
	return nil
}

func (m *Manager) ListInstalled() ([]string, error) {
	var name string
	var args []string
	switch m.Kind {
	case platform.Brew:
		name, args = "brew", []string{"leaves"}
	case platform.Apt:
		name, args = "apt-mark", []string{"showmanual"}
	case platform.Dnf:
		name, args = "dnf", []string{"repoquery", "--userinstalled", "--qf", "%{name}"}
	case platform.Pacman:
		name, args = "pacman", []string{"-Qeq"}
	default:
		return nil, fmt.Errorf("unsupported manager: %s", m.Kind)
	}
	res, err := m.Runner.Run(name, args...)
	if err != nil {
		return nil, err
	}
	return splitLines(res.Stdout), nil
}

func splitLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		l = strings.TrimSpace(l)
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

// InstallVia routes tool-source installs used by the inventory (Task 4).
func InstallVia(source, pkg string, r runner.CommandRunner) error {
	var name string
	var args []string
	switch source {
	case "uv":
		name, args = "uv", []string{"tool", "install", pkg}
	case "cargo":
		name, args = "cargo", []string{"install", pkg}
	case "npm":
		name, args = "npm", []string{"install", "-g", pkg}
	case "go":
		name, args = "go", []string{"install", pkg}
	case "brew", "apt", "dnf", "pacman":
		return New(platform.PkgManager(source), r).Install(pkg)
	default:
		return fmt.Errorf("unsupported source: %s", source)
	}
	res, err := r.Run(name, args...)
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("install %s via %s failed: %s", pkg, source, res.Stderr)
	}
	return nil
}
