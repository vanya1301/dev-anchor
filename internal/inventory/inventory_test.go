package inventory

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/devanchor/da/internal/platform"
	"github.com/devanchor/da/internal/runner"
)

const sample = `
[[tool]]
name = "ripgrep"
manager = "brew"
package = "ripgrep"
version = "latest"
binary = "rg"
platform = ["macos", "linux"]

[[tool]]
name = "ruff"
manager = "uv"
package = "ruff"
version = "latest"
binary = "ruff"

[[tool]]
name = "maconly"
manager = "brew"
package = "maconly"
version = "latest"
binary = "maconly"
platform = ["macos"]
`

func TestParseAndApplies(t *testing.T) {
	inv, err := Parse([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.Tools) != 3 {
		t.Fatalf("tools=%d", len(inv.Tools))
	}
	linux := platform.Info{OS: platform.Linux, Manager: platform.Apt}
	if !inv.Tools[0].AppliesTo(linux) {
		t.Fatal("ripgrep should apply to linux")
	}
	if inv.Tools[2].AppliesTo(linux) {
		t.Fatal("maconly should not apply to linux")
	}
}

var errNotFound = errors.New("not found")

func TestSyncInstallsMissingOnly(t *testing.T) {
	inv, _ := Parse([]byte(sample))
	info := platform.Info{OS: platform.MacOS, Manager: platform.Brew}
	m := &runner.MockRunner{}
	// rg present, ruff missing, maconly present
	look := func(bin string) (string, error) {
		if bin == "ruff" {
			return "", errNotFound
		}
		return "/usr/bin/" + bin, nil
	}
	installed, skipped, err := Sync(inv, info, m, look)
	if err != nil {
		t.Fatal(err)
	}
	if len(installed) != 1 || installed[0] != "ruff" {
		t.Fatalf("installed=%v", installed)
	}
	if len(skipped) != 2 {
		t.Fatalf("skipped=%v", skipped)
	}
}

// A tool declared with a primary "manager" (used on the platform it was
// authored for, e.g. brew on macOS) must install via the *detected
// platform's* package manager when an override exists for it — never via
// the literal `manager` field on a platform where that manager doesn't
// even exist.
const overrideSample = `
[[tool]]
name = "ripgrep"
manager = "brew"
package = "ripgrep"
binary = "rg"

[tool.overrides]
apt = "ripgrep"
`

func TestSyncUsesPlatformManagerWhenOverridePresent(t *testing.T) {
	inv, err := Parse([]byte(overrideSample))
	if err != nil {
		t.Fatal(err)
	}
	info := platform.Info{OS: platform.Linux, Manager: platform.Apt}
	m := &runner.MockRunner{}
	look := func(string) (string, error) { return "", errNotFound }

	installed, _, err := Sync(inv, info, m, look)
	if err != nil {
		t.Fatal(err)
	}
	if len(installed) != 1 {
		t.Fatalf("installed=%v", installed)
	}
	if len(m.Calls) != 1 {
		t.Fatalf("calls=%+v", m.Calls)
	}
	// must run apt-get via sudo, NOT `brew install ripgrep` (brew doesn't
	// exist on this platform).
	if m.Calls[0].Name != "sudo" || m.Calls[0].Args[0] != "apt-get" {
		t.Fatalf("expected apt-get install, got %+v", m.Calls[0])
	}
}

func TestStateRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "state.json")
	s := State{Tools: map[string]ToolState{"ruff": {Version: "0.5.0", Manager: "uv"}}}
	if err := s.Save(p); err != nil {
		t.Fatal(err)
	}
	got, err := LoadState(p)
	if err != nil {
		t.Fatal(err)
	}
	if got.Tools["ruff"].Version != "0.5.0" {
		t.Fatalf("got %+v", got)
	}
}
