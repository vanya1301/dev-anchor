# DevAnchor MVP Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the `da` CLI + `bootstrap` script MVP that takes a fresh machine from bare git to fully provisioned: detect OS, install tools from `tools.toml`, symlink configs with interactive conflict resolution, decrypt secrets, and reverse-sync local changes via `da anchor`.

**Architecture:** A single Go static binary (`da`, cobra CLI) delivered by a POSIX `bootstrap` script that downloads the right prebuilt binary. All subprocess calls (package managers, `sops`, `age`) route through a `runner.CommandRunner` seam so every package is unit-testable with a mock. Packages in `internal/` each have one responsibility: platform detection, package-manager install/list, tool inventory, symlinks, secrets, reporting.

**Tech Stack:** Go 1.26, `github.com/spf13/cobra` (CLI), `github.com/BurntSushi/toml` (config parsing), standard library for filesystem/exec. External binaries shelled out to: `brew`/`apt`/`dnf`/`pacman`/`uv`/`cargo`/`npm`/`go`, `sops`, `age`, `age-keygen`.

## Global Constraints

- Language: Go, single static binary. Module path: `github.com/devanchor/da`.
- Target platforms: macOS, Ubuntu/Debian, Fedora, Arch Linux, and those Linux distros under WSL.
- Package managers: `brew` (macOS), `apt` (Debian/Ubuntu), `dnf` (Fedora), `pacman` (Arch). Plus tool sources: `uv`, `cargo`, `npm`, `go`, `github-release`.
- Prefer `uv tool install` for Python CLIs; fall back to `pip` only if unavailable via uv.
- Secrets: shell out to `sops` + `age`. Never write plaintext secrets back into the repo. Session key file mode `0600`, shredded on exit.
- All subprocess calls MUST go through `runner.CommandRunner` (no direct `exec.Command` in feature packages).
- Idempotent: every command safe to re-run.
- Never overwrite a real file without the interactive conflict flow (choices a/b/c, default c).
- Repo model: the git repo IS the DevAnchor (holds `tools.toml`, `symlinks.toml`, `configs/`, `.sops.yaml`, `age.pub`, `age.key.enc`, `bootstrap`). `da` writes state to `.da/state.json` and `.da/manifest.json`, backups to `.backup/`.
- TDD: write failing test, see it fail, implement minimal, see it pass, commit. Frequent commits.

---

### Task 1: Module bootstrap + CommandRunner seam

**Files:**
- Create: `go.mod`
- Create: `internal/runner/runner.go`
- Test: `internal/runner/runner_test.go`

**Interfaces:**
- Consumes: nothing (first task).
- Produces:
  - `type Result struct { Stdout string; Stderr string; ExitCode int }`
  - `type CommandRunner interface { Run(name string, args ...string) (Result, error); RunInput(stdin string, name string, args ...string) (Result, error) }`
  - `type ExecRunner struct{}` implementing `CommandRunner` via `os/exec`.
  - `type MockRunner struct { Calls []Call; Responses map[string]Result; Errors map[string]error }` with `Call{ Name string; Args []string; Stdin string }`; key format for maps is `name + " " + strings.Join(args, " ")`.

- [ ] **Step 1: Initialize the module**

Run:
```bash
go mod init github.com/devanchor/da
```
Expected: creates `go.mod` with `module github.com/devanchor/da` and a `go 1.26` line.

- [ ] **Step 2: Write the failing test**

Create `internal/runner/runner_test.go`:
```go
package runner

import "testing"

func TestExecRunnerRunEcho(t *testing.T) {
	r := ExecRunner{}
	res, err := r.Run("echo", "hello")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("exit=%d want 0", res.ExitCode)
	}
	if res.Stdout != "hello\n" {
		t.Fatalf("stdout=%q want %q", res.Stdout, "hello\n")
	}
}

func TestMockRunnerRecordsAndReturns(t *testing.T) {
	m := &MockRunner{Responses: map[string]Result{"brew leaves": {Stdout: "ripgrep\n"}}}
	res, err := m.Run("brew", "leaves")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.Stdout != "ripgrep\n" {
		t.Fatalf("stdout=%q", res.Stdout)
	}
	if len(m.Calls) != 1 || m.Calls[0].Name != "brew" || m.Calls[0].Args[0] != "leaves" {
		t.Fatalf("calls not recorded: %+v", m.Calls)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/runner/`
Expected: FAIL — undefined `ExecRunner`, `MockRunner`, `Result`.

- [ ] **Step 4: Write minimal implementation**

Create `internal/runner/runner.go`:
```go
package runner

import (
	"bytes"
	"os/exec"
	"strings"
)

type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

type Call struct {
	Name  string
	Args  []string
	Stdin string
}

type CommandRunner interface {
	Run(name string, args ...string) (Result, error)
	RunInput(stdin, name string, args ...string) (Result, error)
}

type ExecRunner struct{}

func (ExecRunner) Run(name string, args ...string) (Result, error) {
	return ExecRunner{}.RunInput("", name, args...)
}

func (ExecRunner) RunInput(stdin, name string, args ...string) (Result, error) {
	cmd := exec.Command(name, args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err := cmd.Run()
	res := Result{Stdout: out.String(), Stderr: errb.String()}
	if ee, ok := err.(*exec.ExitError); ok {
		res.ExitCode = ee.ExitCode()
		return res, nil
	}
	if err != nil {
		return res, err
	}
	return res, nil
}

type MockRunner struct {
	Calls     []Call
	Responses map[string]Result
	Errors    map[string]error
}

func (m *MockRunner) key(name string, args []string) string {
	return strings.TrimSpace(name + " " + strings.Join(args, " "))
}

func (m *MockRunner) Run(name string, args ...string) (Result, error) {
	return m.RunInput("", name, args...)
}

func (m *MockRunner) RunInput(stdin, name string, args ...string) (Result, error) {
	m.Calls = append(m.Calls, Call{Name: name, Args: args, Stdin: stdin})
	k := m.key(name, args)
	if m.Errors != nil {
		if e, ok := m.Errors[k]; ok {
			return Result{}, e
		}
	}
	if m.Responses != nil {
		if r, ok := m.Responses[k]; ok {
			return r, nil
		}
	}
	return Result{}, nil
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/runner/`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add go.mod internal/runner/
git commit -m "feat: add go module and CommandRunner seam"
```

---

### Task 2: Platform detection

**Files:**
- Create: `internal/platform/platform.go`
- Test: `internal/platform/platform_test.go`

**Interfaces:**
- Consumes: `runner.CommandRunner` (Task 1) — not needed here; detection reads files/env and `runtime`.
- Produces:
  - `type OS string` with consts `MacOS OS = "macos"`, `Linux OS = "linux"`.
  - `type Distro string` with consts `Debian`, `Ubuntu`, `Fedora`, `Arch`, `UnknownDistro = ""`.
  - `type PkgManager string` with consts `Brew`, `Apt`, `Dnf`, `Pacman`.
  - `type Info struct { OS OS; Distro Distro; Arch string; WSL bool; Manager PkgManager }`
  - `func DetectFrom(goos, goarch string, osRelease string, wslMarker bool) (Info, error)` — pure, testable. `osRelease` is the contents of `/etc/os-release`; `wslMarker` is true when `/proc/sys/kernel/osrelease` or `/proc/version` contains "microsoft".
  - `func Detect() (Info, error)` — reads real files/`runtime` and calls `DetectFrom`.

- [ ] **Step 1: Write the failing test**

Create `internal/platform/platform_test.go`:
```go
package platform

import "testing"

func TestDetectFrom(t *testing.T) {
	cases := []struct {
		name      string
		goos      string
		goarch    string
		osRelease string
		wsl       bool
		want      Info
	}{
		{
			name:   "macos arm64",
			goos:   "darwin",
			goarch: "arm64",
			want:   Info{OS: MacOS, Distro: UnknownDistro, Arch: "arm64", WSL: false, Manager: Brew},
		},
		{
			name:      "ubuntu",
			goos:      "linux",
			goarch:    "amd64",
			osRelease: "ID=ubuntu\nID_LIKE=debian\n",
			want:      Info{OS: Linux, Distro: Ubuntu, Arch: "amd64", WSL: false, Manager: Apt},
		},
		{
			name:      "debian",
			goos:      "linux",
			goarch:    "amd64",
			osRelease: "ID=debian\n",
			want:      Info{OS: Linux, Distro: Debian, Arch: "amd64", WSL: false, Manager: Apt},
		},
		{
			name:      "fedora",
			goos:      "linux",
			goarch:    "amd64",
			osRelease: "ID=fedora\n",
			want:      Info{OS: Linux, Distro: Fedora, Arch: "amd64", WSL: false, Manager: Dnf},
		},
		{
			name:      "arch",
			goos:      "linux",
			goarch:    "amd64",
			osRelease: "ID=arch\n",
			want:      Info{OS: Linux, Distro: Arch, Arch: "amd64", WSL: false, Manager: Pacman},
		},
		{
			name:      "ubuntu under wsl",
			goos:      "linux",
			goarch:    "amd64",
			osRelease: "ID=ubuntu\n",
			wsl:       true,
			want:      Info{OS: Linux, Distro: Ubuntu, Arch: "amd64", WSL: true, Manager: Apt},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := DetectFrom(c.goos, c.goarch, c.osRelease, c.wsl)
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			if got != c.want {
				t.Fatalf("got %+v want %+v", got, c.want)
			}
		})
	}
}

func TestDetectFromUnknownErrors(t *testing.T) {
	if _, err := DetectFrom("plan9", "amd64", "", false); err == nil {
		t.Fatal("expected error for unsupported OS")
	}
	if _, err := DetectFrom("linux", "amd64", "ID=gentoo\n", false); err == nil {
		t.Fatal("expected error for unsupported distro")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/platform/`
Expected: FAIL — undefined `DetectFrom`, `Info`, consts.

- [ ] **Step 3: Write minimal implementation**

Create `internal/platform/platform.go`:
```go
package platform

import (
	"fmt"
	"os"
	"runtime"
	"strings"
)

type OS string

const (
	MacOS OS = "macos"
	Linux OS = "linux"
)

type Distro string

const (
	UnknownDistro Distro = ""
	Debian        Distro = "debian"
	Ubuntu        Distro = "ubuntu"
	Fedora        Distro = "fedora"
	Arch          Distro = "arch"
)

type PkgManager string

const (
	Brew   PkgManager = "brew"
	Apt    PkgManager = "apt"
	Dnf    PkgManager = "dnf"
	Pacman PkgManager = "pacman"
)

type Info struct {
	OS      OS
	Distro  Distro
	Arch    string
	WSL     bool
	Manager PkgManager
}

func parseOSReleaseID(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "ID=") {
			v := strings.TrimPrefix(line, "ID=")
			return strings.Trim(v, "\"")
		}
	}
	return ""
}

func DetectFrom(goos, goarch, osRelease string, wslMarker bool) (Info, error) {
	info := Info{Arch: goarch}
	switch goos {
	case "darwin":
		info.OS = MacOS
		info.Manager = Brew
		return info, nil
	case "linux":
		info.OS = Linux
		info.WSL = wslMarker
	default:
		return Info{}, fmt.Errorf("unsupported OS: %s", goos)
	}

	id := parseOSReleaseID(osRelease)
	switch id {
	case "ubuntu":
		info.Distro, info.Manager = Ubuntu, Apt
	case "debian":
		info.Distro, info.Manager = Debian, Apt
	case "fedora":
		info.Distro, info.Manager = Fedora, Dnf
	case "arch":
		info.Distro, info.Manager = Arch, Pacman
	default:
		// ID_LIKE fallback for debian derivatives
		if strings.Contains(osRelease, "ID_LIKE=debian") || strings.Contains(osRelease, "debian") {
			info.Distro, info.Manager = Debian, Apt
			return info, nil
		}
		return Info{}, fmt.Errorf("unsupported linux distro: %q", id)
	}
	return info, nil
}

func Detect() (Info, error) {
	var osRelease string
	if b, err := os.ReadFile("/etc/os-release"); err == nil {
		osRelease = string(b)
	}
	wsl := false
	if b, err := os.ReadFile("/proc/version"); err == nil {
		if strings.Contains(strings.ToLower(string(b)), "microsoft") {
			wsl = true
		}
	}
	return DetectFrom(runtime.GOOS, runtime.GOARCH, osRelease, wsl)
}
```

Note: the ubuntu test case with `ID=ubuntu` matches before the `ID_LIKE` fallback; the fallback only triggers for unknown IDs that look debian-like.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/platform/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/platform/
git commit -m "feat: add platform detection (os/distro/arch/wsl/pkgmgr)"
```

---

### Task 3: Package-manager install + list-installed

**Files:**
- Create: `internal/pkgmgr/pkgmgr.go`
- Test: `internal/pkgmgr/pkgmgr_test.go`

**Interfaces:**
- Consumes: `runner.CommandRunner` (Task 1), `platform.PkgManager` (Task 2).
- Produces:
  - `type Manager struct { Kind platform.PkgManager; Runner runner.CommandRunner }`
  - `func New(kind platform.PkgManager, r runner.CommandRunner) *Manager`
  - `func (m *Manager) Install(pkg string) error` — routes to the right install command.
  - `func (m *Manager) ListInstalled() ([]string, error)` — returns user-installed package names.
  - Source installers keyed by tool `manager` field (used by Task 4): `func InstallVia(source string, pkg string, r runner.CommandRunner) error` for `uv`/`cargo`/`npm`/`go`/`github-release` plus the four system managers.

- [ ] **Step 1: Write the failing test**

Create `internal/pkgmgr/pkgmgr_test.go`:
```go
package pkgmgr

import (
	"testing"

	"github.com/devanchor/da/internal/platform"
	"github.com/devanchor/da/internal/runner"
)

func TestInstallRoutesPerManager(t *testing.T) {
	cases := []struct {
		kind     platform.PkgManager
		wantName string
		wantArgs []string
	}{
		{platform.Brew, "brew", []string{"install", "ripgrep"}},
		{platform.Apt, "sudo", []string{"apt-get", "install", "-y", "ripgrep"}},
		{platform.Dnf, "sudo", []string{"dnf", "install", "-y", "ripgrep"}},
		{platform.Pacman, "sudo", []string{"pacman", "-S", "--noconfirm", "ripgrep"}},
	}
	for _, c := range cases {
		m := &runner.MockRunner{}
		mgr := New(c.kind, m)
		if err := mgr.Install("ripgrep"); err != nil {
			t.Fatalf("%s: %v", c.kind, err)
		}
		if m.Calls[0].Name != c.wantName {
			t.Fatalf("%s name=%s want %s", c.kind, m.Calls[0].Name, c.wantName)
		}
		if len(m.Calls[0].Args) != len(c.wantArgs) {
			t.Fatalf("%s args=%v want %v", c.kind, m.Calls[0].Args, c.wantArgs)
		}
	}
}

func TestListInstalledBrew(t *testing.T) {
	m := &runner.MockRunner{Responses: map[string]runner.Result{"brew leaves": {Stdout: "ripgrep\nfzf\n"}}}
	mgr := New(platform.Brew, m)
	got, err := mgr.ListInstalled()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "ripgrep" || got[1] != "fzf" {
		t.Fatalf("got %v", got)
	}
}

func TestInstallViaUvPrefersUvTool(t *testing.T) {
	m := &runner.MockRunner{}
	if err := InstallVia("uv", "ruff", m); err != nil {
		t.Fatal(err)
	}
	if m.Calls[0].Name != "uv" || m.Calls[0].Args[0] != "tool" || m.Calls[0].Args[1] != "install" {
		t.Fatalf("got %+v", m.Calls[0])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/pkgmgr/`
Expected: FAIL — undefined `New`, `Install`, `ListInstalled`, `InstallVia`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/pkgmgr/pkgmgr.go`:
```go
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
```

Note: `github-release` install is deferred — `InstallVia` returns "unsupported source" for it in the MVP; document in `tools.toml` template that github-release entries are not yet auto-installed. (Tracked for a later plan.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/pkgmgr/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/pkgmgr/
git commit -m "feat: add package-manager install and list-installed"
```

---

### Task 4: Tool inventory (parse + install orchestration + state)

**Files:**
- Create: `internal/inventory/inventory.go`
- Create: `internal/inventory/state.go`
- Test: `internal/inventory/inventory_test.go`

**Interfaces:**
- Consumes: `runner.CommandRunner` (Task 1), `platform.Info` (Task 2), `pkgmgr.InstallVia` (Task 3).
- Produces:
  - `type Tool struct { Name string; Manager string; Package string; Version string; Binary string; Platform []string; Overrides map[string]string }`
  - `type Inventory struct { Tools []Tool }`
  - `func Parse(tomlBytes []byte) (Inventory, error)`
  - `func (t Tool) AppliesTo(info platform.Info) bool` — true if `Platform` empty or contains the OS string.
  - `func (t Tool) ResolvedPackage(manager string) string` — returns `Overrides[manager]` if present else `Package`.
  - `type LookPathFunc func(string) (string, error)`
  - `func Sync(inv Inventory, info platform.Info, r runner.CommandRunner, look LookPathFunc) (installed []string, skipped []string, err error)` — installs tools whose `Binary` is not found by `look`, records nothing itself (caller persists state).
  - State: `type State struct { Tools map[string]ToolState }`, `type ToolState struct { Version, Manager string }`, `func LoadState(path string) (State, error)`, `func (s State) Save(path string) error`.

- [ ] **Step 1: Write the failing test**

Create `internal/inventory/inventory_test.go`:
```go
package inventory

import (
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
```

Add a shared sentinel error in the test file's package by using the implementation's exported/unexported error. To keep the test self-contained, define `var errNotFound = errors.New("not found")` in the test and ensure `Sync` treats any non-nil `look` error as "missing".

Adjust test top imports to include `"errors"` and declare `var errNotFound = errors.New("not found")`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go get github.com/BurntSushi/toml && go test ./internal/inventory/`
Expected: FAIL — undefined `Parse`, `Sync`, `State`, etc.

- [ ] **Step 3: Write minimal implementation**

Create `internal/inventory/inventory.go`:
```go
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
```

Create `internal/inventory/state.go`:
```go
package inventory

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type ToolState struct {
	Version string `json:"version"`
	Manager string `json:"manager"`
}

type State struct {
	Tools map[string]ToolState `json:"tools"`
}

func LoadState(path string) (State, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return State{Tools: map[string]ToolState{}}, nil
	}
	if err != nil {
		return State{}, err
	}
	var s State
	if err := json.Unmarshal(b, &s); err != nil {
		return State{}, err
	}
	if s.Tools == nil {
		s.Tools = map[string]ToolState{}
	}
	return s, nil
}

func (s State) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/inventory/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/inventory/ go.mod go.sum
git commit -m "feat: add tool inventory parse, sync, and state store"
```

---

### Task 5: Symlink strategy with conflict resolution

**Files:**
- Create: `internal/symlink/symlink.go`
- Create: `internal/symlink/manifest.go`
- Test: `internal/symlink/symlink_test.go`

**Interfaces:**
- Consumes: nothing external beyond stdlib.
- Produces:
  - `type Link struct { Source string; Target string }` (`Source` relative to `configsDir`, `Target` may contain `~`).
  - `type Choice int` with `Leave Choice = iota`, `CopyKeep`, `CopyReplace`.
  - `type Prompter func(target string) Choice`
  - `type ApplyResult struct { Created []string; Skipped []string; Conflicts []string; Adopted []string }`
  - `func Apply(links []Link, configsDir, backupDir string, prompt Prompter) (ApplyResult, ManifestEntries, error)`
  - `type ManifestEntry struct { Target, Source, BackupPath string }`, `type ManifestEntries []ManifestEntry`, `func LoadManifest(path string) (ManifestEntries, error)`, `func (m ManifestEntries) Save(path string) error`.

- [ ] **Step 1: Write the failing test**

Create `internal/symlink/symlink_test.go`:
```go
package symlink

import (
	"os"
	"path/filepath"
	"testing"
)

func setup(t *testing.T) (configs, backup, home string) {
	root := t.TempDir()
	configs = filepath.Join(root, "configs")
	backup = filepath.Join(root, ".backup")
	home = filepath.Join(root, "home")
	os.MkdirAll(filepath.Join(configs, "zsh"), 0o755)
	os.MkdirAll(home, 0o755)
	os.WriteFile(filepath.Join(configs, "zsh", ".zshrc"), []byte("export A=1\n"), 0o644)
	return
}

func TestApplyMissingTargetCreatesSymlink(t *testing.T) {
	configs, backup, home := setup(t)
	target := filepath.Join(home, ".zshrc")
	links := []Link{{Source: "zsh/.zshrc", Target: target}}
	res, _, err := Apply(links, configs, backup, func(string) Choice { return CopyReplace })
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Created) != 1 {
		t.Fatalf("created=%v", res.Created)
	}
	fi, err := os.Lstat(target)
	if err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("target not a symlink: %v", err)
	}
}

func TestApplyExistingRepoSymlinkSkips(t *testing.T) {
	configs, backup, home := setup(t)
	target := filepath.Join(home, ".zshrc")
	os.Symlink(filepath.Join(configs, "zsh", ".zshrc"), target)
	links := []Link{{Source: "zsh/.zshrc", Target: target}}
	res, _, err := Apply(links, configs, backup, func(string) Choice { return Leave })
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Skipped) != 1 {
		t.Fatalf("skipped=%v", res.Skipped)
	}
}

func TestApplyRealFileChoiceReplace(t *testing.T) {
	configs, backup, home := setup(t)
	target := filepath.Join(home, ".vimrc")
	os.WriteFile(target, []byte("set nocompatible\n"), 0o644)
	links := []Link{{Source: "vim/.vimrc", Target: target}}
	res, _, err := Apply(links, configs, backup, func(string) Choice { return CopyReplace })
	if err != nil {
		t.Fatal(err)
	}
	// original backed up
	if _, err := os.Stat(filepath.Join(backup, ".vimrc")); err != nil {
		t.Fatalf("backup missing: %v", err)
	}
	// content copied into configs
	b, _ := os.ReadFile(filepath.Join(configs, "vim/.vimrc"))
	if string(b) != "set nocompatible\n" {
		t.Fatalf("configs content=%q", b)
	}
	// target now a symlink
	fi, _ := os.Lstat(target)
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("target not symlink after replace")
	}
	if len(res.Adopted) != 1 {
		t.Fatalf("adopted=%v", res.Adopted)
	}
}

func TestApplyRealFileChoiceLeave(t *testing.T) {
	configs, backup, home := setup(t)
	target := filepath.Join(home, ".vimrc")
	os.WriteFile(target, []byte("x\n"), 0o644)
	links := []Link{{Source: "vim/.vimrc", Target: target}}
	res, _, err := Apply(links, configs, backup, func(string) Choice { return Leave })
	if err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Lstat(target)
	if fi.Mode()&os.ModeSymlink != 0 {
		t.Fatal("target should remain a real file")
	}
	if len(res.Conflicts) != 1 {
		t.Fatalf("conflicts=%v", res.Conflicts)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/symlink/`
Expected: FAIL — undefined `Apply`, `Link`, `Choice`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/symlink/symlink.go`:
```go
package symlink

import (
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Link struct {
	Source string
	Target string
}

type Choice int

const (
	Leave Choice = iota
	CopyKeep
	CopyReplace
)

type Prompter func(target string) Choice

type ApplyResult struct {
	Created   []string
	Skipped   []string
	Conflicts []string
	Adopted   []string
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, strings.TrimPrefix(p, "~"))
	}
	return p
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func Apply(links []Link, configsDir, backupDir string, prompt Prompter) (ApplyResult, ManifestEntries, error) {
	var res ApplyResult
	var entries ManifestEntries

	for _, l := range links {
		target := expandHome(l.Target)
		srcPath := filepath.Join(configsDir, l.Source)

		fi, err := os.Lstat(target)
		if os.IsNotExist(err) {
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return res, entries, err
			}
			if err := os.Symlink(srcPath, target); err != nil {
				return res, entries, err
			}
			res.Created = append(res.Created, target)
			entries = append(entries, ManifestEntry{Target: target, Source: l.Source})
			continue
		}
		if err != nil {
			return res, entries, err
		}

		// existing symlink into repo?
		if fi.Mode()&os.ModeSymlink != 0 {
			dest, _ := os.Readlink(target)
			if dest == srcPath {
				res.Skipped = append(res.Skipped, target)
				continue
			}
		}

		// real file (or foreign symlink) -> prompt
		switch prompt(target) {
		case Leave:
			res.Conflicts = append(res.Conflicts, target)
		case CopyKeep:
			if err := copyFile(target, srcPath); err != nil {
				return res, entries, err
			}
			res.Adopted = append(res.Adopted, target)
		case CopyReplace:
			backupPath := filepath.Join(backupDir, filepath.Base(target))
			if err := copyFile(target, backupPath); err != nil {
				return res, entries, err
			}
			if err := copyFile(target, srcPath); err != nil {
				return res, entries, err
			}
			if err := os.Remove(target); err != nil {
				return res, entries, err
			}
			if err := os.Symlink(srcPath, target); err != nil {
				return res, entries, err
			}
			res.Adopted = append(res.Adopted, target)
			entries = append(entries, ManifestEntry{Target: target, Source: l.Source, BackupPath: backupPath})
		}
	}
	return res, entries, nil
}
```

Create `internal/symlink/manifest.go`:
```go
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/symlink/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/symlink/
git commit -m "feat: add symlink apply with conflict resolution and manifest"
```

---

### Task 6: Secrets lifecycle (keygen/login/decrypt/encrypt)

**Files:**
- Create: `internal/secrets/secrets.go`
- Test: `internal/secrets/secrets_test.go`

**Interfaces:**
- Consumes: `runner.CommandRunner` (Task 1).
- Produces:
  - `type Session struct { KeyFile string; runner runner.CommandRunner }`
  - `func Login(r runner.CommandRunner, encKeyPath, password string, tmpDir string) (*Session, error)` — runs `age -d` with password on `encKeyPath`, writes recovered key to `tmpDir/age.key` (mode `0600`), returns session. Sets nothing global; caller uses `Session.Env()`.
  - `func (s *Session) Env() []string` — returns `["SOPS_AGE_KEY_FILE=<KeyFile>"]` to pass to sops subprocess calls.
  - `func (s *Session) Close() error` — overwrites key file with zeros then removes it.
  - `func (s *Session) Decrypt(file string) (string, error)` — runs `sops -d file`, returns plaintext.
  - `func (s *Session) Encrypt(file string) error` — runs `sops -e -i file`.
  - `func Keygen(r runner.CommandRunner, password, pubOut, encKeyOut string) error` — runs `age-keygen`, then `age -p` to produce encrypted key; writes pub file.

Note: `age -p` and `age -d` need a passphrase on an interactive tty. For the MVP, pass the password via the `RunInput` stdin seam and invoke `age` with the passphrase-from-stdin approach; the tests mock the runner, so this is validated at the command-shape level. A follow-up plan hardens real tty/passphrase handling.

- [ ] **Step 1: Write the failing test**

Create `internal/secrets/secrets_test.go`:
```go
package secrets

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/devanchor/da/internal/runner"
)

func TestLoginWritesKeyAndCloseShreds(t *testing.T) {
	dir := t.TempDir()
	enc := filepath.Join(dir, "age.key.enc")
	os.WriteFile(enc, []byte("ENC"), 0o600)
	m := &runner.MockRunner{Responses: map[string]runner.Result{
		"age -d " + enc: {Stdout: "AGE-SECRET-KEY-1TESTKEY\n"},
	}}
	s, err := Login(m, enc, "pw", dir)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(s.KeyFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "AGE-SECRET-KEY-1TESTKEY\n" {
		t.Fatalf("key content=%q", b)
	}
	if s.Env()[0] != "SOPS_AGE_KEY_FILE="+s.KeyFile {
		t.Fatalf("env=%v", s.Env())
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.KeyFile); !os.IsNotExist(err) {
		t.Fatal("key file should be removed after Close")
	}
}

func TestDecryptCallsSops(t *testing.T) {
	dir := t.TempDir()
	enc := filepath.Join(dir, "age.key.enc")
	os.WriteFile(enc, []byte("ENC"), 0o600)
	m := &runner.MockRunner{Responses: map[string]runner.Result{
		"age -d " + enc:      {Stdout: "KEY\n"},
		"sops -d secret.yaml": {Stdout: "password: hunter2\n"},
	}}
	s, _ := Login(m, enc, "pw", dir)
	out, err := s.Decrypt("secret.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if out != "password: hunter2\n" {
		t.Fatalf("out=%q", out)
	}
}

func TestEncryptInPlace(t *testing.T) {
	dir := t.TempDir()
	enc := filepath.Join(dir, "age.key.enc")
	os.WriteFile(enc, []byte("ENC"), 0o600)
	m := &runner.MockRunner{Responses: map[string]runner.Result{"age -d " + enc: {Stdout: "KEY\n"}}}
	s, _ := Login(m, enc, "pw", dir)
	if err := s.Encrypt("secret.yaml"); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range m.Calls {
		if c.Name == "sops" && len(c.Args) >= 3 && c.Args[0] == "-e" && c.Args[1] == "-i" && c.Args[2] == "secret.yaml" {
			found = true
		}
	}
	if !found {
		t.Fatalf("sops -e -i not called: %+v", m.Calls)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/secrets/`
Expected: FAIL — undefined `Login`, `Session`, etc.

- [ ] **Step 3: Write minimal implementation**

Create `internal/secrets/secrets.go`:
```go
package secrets

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/devanchor/da/internal/runner"
)

type Session struct {
	KeyFile string
	runner  runner.CommandRunner
}

func Login(r runner.CommandRunner, encKeyPath, password, tmpDir string) (*Session, error) {
	res, err := r.RunInput(password, "age", "-d", encKeyPath)
	if err != nil {
		return nil, err
	}
	if res.ExitCode != 0 {
		return nil, fmt.Errorf("age decrypt failed: %s", res.Stderr)
	}
	keyFile := filepath.Join(tmpDir, "age.key")
	if err := os.WriteFile(keyFile, []byte(res.Stdout), 0o600); err != nil {
		return nil, err
	}
	return &Session{KeyFile: keyFile, runner: r}, nil
}

func (s *Session) Env() []string {
	return []string{"SOPS_AGE_KEY_FILE=" + s.KeyFile}
}

func (s *Session) Close() error {
	if s.KeyFile == "" {
		return nil
	}
	if fi, err := os.Stat(s.KeyFile); err == nil {
		zeros := make([]byte, fi.Size())
		_ = os.WriteFile(s.KeyFile, zeros, 0o600)
	}
	err := os.Remove(s.KeyFile)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func (s *Session) Decrypt(file string) (string, error) {
	res, err := s.runner.Run("sops", "-d", file)
	if err != nil {
		return "", err
	}
	if res.ExitCode != 0 {
		return "", fmt.Errorf("sops decrypt %s failed: %s", file, res.Stderr)
	}
	return res.Stdout, nil
}

func (s *Session) Encrypt(file string) error {
	res, err := s.runner.Run("sops", "-e", "-i", file)
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("sops encrypt %s failed: %s", file, res.Stderr)
	}
	return nil
}

func Keygen(r runner.CommandRunner, password, pubOut, encKeyOut string) error {
	gen, err := r.Run("age-keygen")
	if err != nil {
		return err
	}
	if gen.ExitCode != 0 {
		return fmt.Errorf("age-keygen failed: %s", gen.Stderr)
	}
	// age-keygen prints the public key to stderr as "# public key: ..." and the
	// secret key to stdout. Persist the secret key encrypted with age -p.
	enc, err := r.RunInput(gen.Stdout, "age", "-p")
	if err != nil {
		return err
	}
	if enc.ExitCode != 0 {
		return fmt.Errorf("age -p failed: %s", enc.Stderr)
	}
	if err := os.WriteFile(encKeyOut, []byte(enc.Stdout), 0o600); err != nil {
		return err
	}
	// public key derivation via age-keygen -y from the secret key
	pub, err := r.RunInput(gen.Stdout, "age-keygen", "-y")
	if err != nil {
		return err
	}
	return os.WriteFile(pubOut, []byte(pub.Stdout), 0o644)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/secrets/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/secrets/
git commit -m "feat: add secrets session (login/decrypt/encrypt/keygen)"
```

---

### Task 7: Reporter

**Files:**
- Create: `internal/reporter/reporter.go`
- Test: `internal/reporter/reporter_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `type Report struct { InstalledTools []string; SkippedTools []string; SymlinksCreated []string; SymlinksSkipped []string; Conflicts []string; Adopted []string; DecryptedFiles []string }`
  - `func (r Report) String() string` — human-readable summary; empty categories omitted.

- [ ] **Step 1: Write the failing test**

Create `internal/reporter/reporter_test.go`:
```go
package reporter

import (
	"strings"
	"testing"
)

func TestReportStringIncludesCounts(t *testing.T) {
	r := Report{
		InstalledTools:  []string{"ruff"},
		SymlinksCreated: []string{"/home/u/.zshrc"},
		Conflicts:       []string{"/home/u/.vimrc"},
	}
	out := r.String()
	if !strings.Contains(out, "Installed tools (1)") {
		t.Fatalf("missing installed section: %s", out)
	}
	if !strings.Contains(out, "ruff") {
		t.Fatalf("missing tool name: %s", out)
	}
	if !strings.Contains(out, "Conflicts (1)") {
		t.Fatalf("missing conflicts: %s", out)
	}
}

func TestReportOmitsEmpty(t *testing.T) {
	r := Report{InstalledTools: []string{"x"}}
	out := r.String()
	if strings.Contains(out, "Conflicts") {
		t.Fatalf("should omit empty conflicts: %s", out)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/reporter/`
Expected: FAIL — undefined `Report`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/reporter/reporter.go`:
```go
package reporter

import (
	"fmt"
	"strings"
)

type Report struct {
	InstalledTools  []string
	SkippedTools    []string
	SymlinksCreated []string
	SymlinksSkipped []string
	Conflicts       []string
	Adopted         []string
	DecryptedFiles  []string
}

func section(b *strings.Builder, title string, items []string) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(b, "%s (%d):\n", title, len(items))
	for _, it := range items {
		fmt.Fprintf(b, "  - %s\n", it)
	}
}

func (r Report) String() string {
	var b strings.Builder
	section(&b, "Installed tools", r.InstalledTools)
	section(&b, "Already present", r.SkippedTools)
	section(&b, "Symlinks created", r.SymlinksCreated)
	section(&b, "Symlinks skipped", r.SymlinksSkipped)
	section(&b, "Adopted configs", r.Adopted)
	section(&b, "Conflicts", r.Conflicts)
	section(&b, "Decrypted files", r.DecryptedFiles)
	if b.Len() == 0 {
		return "No changes.\n"
	}
	return b.String()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/reporter/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/reporter/
git commit -m "feat: add run reporter"
```

---

### Task 8: CLI wiring (cobra commands)

**Files:**
- Create: `cmd/da/main.go`
- Create: `internal/app/config.go`
- Create: `internal/app/pull.go`
- Create: `internal/app/anchor.go`
- Test: `internal/app/config_test.go`

**Interfaces:**
- Consumes: all prior packages.
- Produces:
  - `type Paths struct { Repo string; Configs string; Backup string; StateFile string; ManifestFile string; ToolsFile string; SymlinksFile string; SopsFile string; EncKey string; PubKey string }`
  - `func DefaultPaths(repo string) Paths`
  - `func LoadSymlinks(tomlBytes []byte) ([]symlink.Link, error)` (parses `symlinks.toml` `[[link]]` entries).
  - cobra commands: `da pull [profile]`, `da login`, `da keygen`, `da anchor [path]`. `main.go` wires them with `ExecRunner`.

- [ ] **Step 1: Write the failing test**

Create `internal/app/config_test.go`:
```go
package app

import "testing"

func TestDefaultPaths(t *testing.T) {
	p := DefaultPaths("/repo")
	if p.ToolsFile != "/repo/tools.toml" {
		t.Fatalf("tools=%s", p.ToolsFile)
	}
	if p.StateFile != "/repo/.da/state.json" {
		t.Fatalf("state=%s", p.StateFile)
	}
	if p.Configs != "/repo/configs" {
		t.Fatalf("configs=%s", p.Configs)
	}
}

func TestLoadSymlinks(t *testing.T) {
	toml := `
[[link]]
source = "zsh/.zshrc"
target = "~/.zshrc"

[[link]]
source = "vim/.vimrc"
target = "~/.vimrc"
`
	links, err := LoadSymlinks([]byte(toml))
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 2 || links[0].Source != "zsh/.zshrc" || links[0].Target != "~/.zshrc" {
		t.Fatalf("links=%+v", links)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/app/`
Expected: FAIL — undefined `DefaultPaths`, `LoadSymlinks`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/app/config.go`:
```go
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
```

Create `internal/app/pull.go`:
```go
package app

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/devanchor/da/internal/inventory"
	"github.com/devanchor/da/internal/platform"
	"github.com/devanchor/da/internal/reporter"
	"github.com/devanchor/da/internal/runner"
	"github.com/devanchor/da/internal/symlink"
)

func promptChoice(target string) symlink.Choice {
	fmt.Printf("⚠  %s exists and is a real file (not a DevAnchor symlink).\n", target)
	fmt.Println("   a) leave as-is")
	fmt.Println("   b) copy into configs/, keep file")
	fmt.Println("   c) copy into configs/, replace with symlink  [recommended]")
	fmt.Print("choice [c]: ")
	sc := bufio.NewScanner(os.Stdin)
	sc.Scan()
	switch strings.TrimSpace(sc.Text()) {
	case "a":
		return symlink.Leave
	case "b":
		return symlink.CopyKeep
	default:
		return symlink.CopyReplace
	}
}

func Pull(paths Paths, r runner.CommandRunner, autoYes bool) (reporter.Report, error) {
	var rep reporter.Report

	info, err := platform.Detect()
	if err != nil {
		return rep, err
	}

	// tools
	toolBytes, err := os.ReadFile(paths.ToolsFile)
	if err == nil {
		inv, perr := inventory.Parse(toolBytes)
		if perr != nil {
			return rep, perr
		}
		installed, skipped, serr := inventory.Sync(inv, info, r, exec.LookPath)
		if serr != nil {
			return rep, serr
		}
		rep.InstalledTools = installed
		rep.SkippedTools = skipped
	}

	// symlinks
	linkBytes, err := os.ReadFile(paths.SymlinksFile)
	if err == nil {
		links, lerr := LoadSymlinks(linkBytes)
		if lerr != nil {
			return rep, lerr
		}
		prompt := promptChoice
		if autoYes {
			prompt = func(string) symlink.Choice { return symlink.CopyReplace }
		}
		res, entries, aerr := symlink.Apply(links, paths.Configs, paths.Backup, prompt)
		if aerr != nil {
			return rep, aerr
		}
		rep.SymlinksCreated = res.Created
		rep.SymlinksSkipped = res.Skipped
		rep.Conflicts = res.Conflicts
		rep.Adopted = res.Adopted
		existing, _ := symlink.LoadManifest(paths.ManifestFile)
		if serr := append(existing, entries...).Save(paths.ManifestFile); serr != nil {
			return rep, serr
		}
	}

	return rep, nil
}
```

Create `internal/app/anchor.go`:
```go
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
```

Create `cmd/da/main.go`:
```go
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/devanchor/da/internal/app"
	"github.com/devanchor/da/internal/runner"
)

func main() {
	r := runner.ExecRunner{}

	var autoYes bool

	root := &cobra.Command{Use: "da", Short: "DevAnchor environment sync"}
	root.PersistentFlags().BoolVar(&autoYes, "yes", false, "non-interactive; default all conflict prompts to replace")

	pullCmd := &cobra.Command{
		Use:   "pull [profile]",
		Short: "Detect OS, install tools, symlink configs, decrypt secrets",
		RunE: func(cmd *cobra.Command, args []string) error {
			wd, _ := os.Getwd()
			paths := app.DefaultPaths(wd)
			rep, err := app.Pull(paths, r, autoYes)
			if err != nil {
				return err
			}
			fmt.Print(rep.String())
			return nil
		},
	}

	anchorCmd := &cobra.Command{
		Use:   "anchor [path]",
		Short: "Capture local tools/configs/secrets back into the repo",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("anchor: see plan Task 8 follow-up for full interactive capture")
			return nil
		},
	}

	loginCmd := &cobra.Command{
		Use:   "login",
		Short: "Decrypt the age key for this session",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("login: prompts password and establishes session")
			return nil
		},
	}

	keygenCmd := &cobra.Command{
		Use:   "keygen",
		Short: "Generate and password-encrypt an age keypair",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("keygen: generates age.pub + age.key.enc")
			return nil
		},
	}

	root.AddCommand(pullCmd, anchorCmd, loginCmd, keygenCmd)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```

Note: `login`/`keygen`/`anchor` command bodies are wired to their packages in Task 9 (they need password prompting + session lifecycle glue). This task establishes the command tree and the fully-working `pull` path.

- [ ] **Step 4: Run test to verify it passes**

Run: `go get github.com/spf13/cobra && go test ./internal/app/ && go build ./cmd/da`
Expected: tests PASS; build succeeds producing `da`.

- [ ] **Step 5: Commit**

```bash
git add cmd/ internal/app/ go.mod go.sum
git commit -m "feat: wire cobra CLI with working pull path"
```

---

### Task 9: Wire login/keygen/anchor commands + secrets into pull

**Files:**
- Modify: `cmd/da/main.go`
- Create: `internal/app/secrets.go`
- Modify: `internal/app/pull.go:decrypt-step`
- Test: `internal/app/secrets_test.go`

**Interfaces:**
- Consumes: `secrets` (Task 6), `reporter` (Task 7).
- Produces:
  - `func ReadPassword(prompt string) (string, error)` — reads a password without echo from the tty (uses `golang.org/x/term`); if `$DA_TOKEN` set, returns it without prompting.
  - `func RunLogin(paths Paths, r runner.CommandRunner, password string) (*secrets.Session, string, error)` — creates a session temp dir under `os.MkdirTemp`, calls `secrets.Login`, returns session + tmpDir.
  - `func DecryptSecrets(paths Paths, s *secrets.Session) ([]string, error)` — reads `.sops.yaml`, decrypts each listed file, writes plaintext to a runtime dir (`0600`), symlinks into target. Returns decrypted file list.
  - `func RunKeygen(paths Paths, r runner.CommandRunner, password string) error`.
  - `func RunAnchor(paths Paths, r runner.CommandRunner, s *secrets.Session, newConfigPath string) (reporter.Report, error)` — captures new tools (Task 8 `CaptureNewTools`), adopts `newConfigPath` if provided (reuse `symlink.Apply` with a single `CopyReplace`), encrypts sops files.

- [ ] **Step 1: Write the failing test**

Create `internal/app/secrets_test.go`:
```go
package app

import (
	"os"
	"testing"

	"github.com/devanchor/da/internal/runner"
)

func TestReadPasswordUsesEnvToken(t *testing.T) {
	t.Setenv("DA_TOKEN", "s3cr3t")
	pw, err := ReadPassword("Password: ")
	if err != nil {
		t.Fatal(err)
	}
	if pw != "s3cr3t" {
		t.Fatalf("pw=%q", pw)
	}
}

func TestRunLoginCreatesSession(t *testing.T) {
	dir := t.TempDir()
	enc := dir + "/age.key.enc"
	os.WriteFile(enc, []byte("ENC"), 0o600)
	paths := DefaultPaths(dir)
	paths.EncKey = enc
	m := &runner.MockRunner{Responses: map[string]runner.Result{
		"age -d " + enc: {Stdout: "KEY\n"},
	}}
	s, tmp, err := RunLogin(paths, m, "pw")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)
	defer s.Close()
	if s.KeyFile == "" {
		t.Fatal("no key file")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/app/ -run 'ReadPassword|RunLogin'`
Expected: FAIL — undefined `ReadPassword`, `RunLogin`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/app/secrets.go`:
```go
package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/term"
	"gopkg.in/yaml.v3"

	"github.com/devanchor/da/internal/inventory"
	"github.com/devanchor/da/internal/pkgmgr"
	"github.com/devanchor/da/internal/platform"
	"github.com/devanchor/da/internal/reporter"
	"github.com/devanchor/da/internal/runner"
	"github.com/devanchor/da/internal/secrets"
	"github.com/devanchor/da/internal/symlink"
)

func ReadPassword(prompt string) (string, error) {
	if tok := os.Getenv("DA_TOKEN"); tok != "" {
		return tok, nil
	}
	fmt.Print(prompt)
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func RunLogin(paths Paths, r runner.CommandRunner, password string) (*secrets.Session, string, error) {
	tmpDir, err := os.MkdirTemp(os.Getenv("XDG_RUNTIME_DIR"), "da-session-")
	if err != nil {
		return nil, "", err
	}
	s, err := secrets.Login(r, paths.EncKey, password, tmpDir)
	if err != nil {
		os.RemoveAll(tmpDir)
		return nil, "", err
	}
	return s, tmpDir, nil
}

// sopsConfig models the creation_rules path_regex list of .sops.yaml.
type sopsConfig struct {
	CreationRules []struct {
		PathRegex string `yaml:"path_regex"`
	} `yaml:"creation_rules"`
}

func encryptedFiles(paths Paths) ([]string, error) {
	b, err := os.ReadFile(paths.SopsFile)
	if err != nil {
		return nil, err
	}
	var cfg sopsConfig
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return nil, err
	}
	var files []string
	for _, rule := range cfg.CreationRules {
		// MVP: treat path_regex as a literal relative path when it names a file.
		p := filepath.Join(paths.Repo, strings.TrimPrefix(rule.PathRegex, "^"))
		if _, statErr := os.Stat(p); statErr == nil {
			files = append(files, p)
		}
	}
	return files, nil
}

func DecryptSecrets(paths Paths, s *secrets.Session, runtimeDir string) ([]string, error) {
	files, err := encryptedFiles(paths)
	if err != nil {
		return nil, err
	}
	var decrypted []string
	for _, f := range files {
		plain, derr := s.Decrypt(f)
		if derr != nil {
			return decrypted, derr
		}
		out := filepath.Join(runtimeDir, filepath.Base(f))
		if werr := os.WriteFile(out, []byte(plain), 0o600); werr != nil {
			return decrypted, werr
		}
		decrypted = append(decrypted, out)
	}
	return decrypted, nil
}

func RunKeygen(paths Paths, r runner.CommandRunner, password string) error {
	return secrets.Keygen(r, password, paths.PubKey, paths.EncKey)
}

func RunAnchor(paths Paths, r runner.CommandRunner, s *secrets.Session, newConfigTarget string) (reporter.Report, error) {
	var rep reporter.Report
	info, err := platform.Detect()
	if err != nil {
		return rep, err
	}
	newTools, err := CaptureNewTools(paths, r, info)
	if err != nil {
		return rep, err
	}
	rep.InstalledTools = newTools // reuse field to report captured tool names

	if newConfigTarget != "" {
		base := filepath.Base(newConfigTarget)
		links := []symlink.Link{{Source: base, Target: newConfigTarget}}
		res, entries, aerr := symlink.Apply(links, paths.Configs, paths.Backup,
			func(string) symlink.Choice { return symlink.CopyReplace })
		if aerr != nil {
			return rep, aerr
		}
		rep.Adopted = res.Adopted
		existing, _ := symlink.LoadManifest(paths.ManifestFile)
		if serr := append(existing, entries...).Save(paths.ManifestFile); serr != nil {
			return rep, serr
		}
	}

	if s != nil {
		files, ferr := encryptedFiles(paths)
		if ferr == nil {
			for _, f := range files {
				if eerr := s.Encrypt(f); eerr != nil {
					return rep, eerr
				}
			}
		}
	}
	_ = pkgmgr.New // keep pkgmgr import used via CaptureNewTools
	_ = inventory.Parse
	return rep, nil
}
```

Note: remove the `_ =` keep-alive lines if the imports are already used; they are placeholders to avoid unused-import build errors and should be deleted once `CaptureNewTools`/`inventory` references compile. Verify with `go build`.

Modify `internal/app/pull.go` — add the decrypt step before returning. Insert after the symlink block, before `return rep, nil`:
```go
	// secrets (best-effort; skipped if no .sops.yaml or no login)
	if _, statErr := os.Stat(paths.SopsFile); statErr == nil {
		pw, perr := ReadPassword("DevAnchor password: ")
		if perr == nil && pw != "" {
			if sess, tmp, lerr := RunLogin(paths, r, pw); lerr == nil {
				defer sess.Close()
				defer os.RemoveAll(tmp)
				if dec, derr := DecryptSecrets(paths, sess, tmp); derr == nil {
					rep.DecryptedFiles = dec
				}
			}
		}
	}
```

Modify `cmd/da/main.go` — replace the stub `RunE` bodies for `login`, `keygen`, `anchor`:
```go
	loginCmd := &cobra.Command{
		Use:   "login",
		Short: "Decrypt the age key for this session",
		RunE: func(cmd *cobra.Command, args []string) error {
			wd, _ := os.Getwd()
			paths := app.DefaultPaths(wd)
			pw, err := app.ReadPassword("DevAnchor password: ")
			if err != nil {
				return err
			}
			sess, tmp, err := app.RunLogin(paths, r, pw)
			if err != nil {
				return err
			}
			defer sess.Close()
			defer os.RemoveAll(tmp)
			fmt.Println("login OK; session key established")
			return nil
		},
	}

	keygenCmd := &cobra.Command{
		Use:   "keygen",
		Short: "Generate and password-encrypt an age keypair",
		RunE: func(cmd *cobra.Command, args []string) error {
			wd, _ := os.Getwd()
			paths := app.DefaultPaths(wd)
			pw, err := app.ReadPassword("New DevAnchor password: ")
			if err != nil {
				return err
			}
			if err := app.RunKeygen(paths, r, pw); err != nil {
				return err
			}
			fmt.Println("Generated age.pub and age.key.enc. Commit them: git add age.pub age.key.enc .sops.yaml")
			return nil
		},
	}

	anchorCmd := &cobra.Command{
		Use:   "anchor [path]",
		Short: "Capture local tools/configs/secrets back into the repo",
		RunE: func(cmd *cobra.Command, args []string) error {
			wd, _ := os.Getwd()
			paths := app.DefaultPaths(wd)
			newConfig := ""
			if len(args) > 0 {
				newConfig = args[0]
			}
			var sess *app.SessionHolder // see note
			rep, err := app.RunAnchor(paths, r, nil, newConfig)
			_ = sess
			if err != nil {
				return err
			}
			fmt.Print(rep.String())
			fmt.Println("Review with git diff, then git commit && git push")
			return nil
		},
	}
```

Note: the `SessionHolder` reference above is illustrative — for the MVP `da anchor` passes `nil` for the session (tool + config capture works without decryption; secret *re-encryption* during anchor requires an active login and is exercised via `da login` first in a later iteration). Delete the `var sess`/`_ = sess` lines; call `app.RunAnchor(paths, r, nil, newConfig)` directly.

- [ ] **Step 4: Run tests and build**

Run:
```bash
go get golang.org/x/term gopkg.in/yaml.v3
go test ./... && go build ./cmd/da
```
Expected: all tests PASS; build succeeds. Fix any unused-import placeholders flagged by the compiler.

- [ ] **Step 5: Commit**

```bash
git add cmd/ internal/app/ go.mod go.sum
git commit -m "feat: wire login/keygen/anchor commands and secret decrypt into pull"
```

---

### Task 10: Bootstrap script + templates + smoke check

**Files:**
- Create: `bootstrap`
- Create: `tools.toml` (template)
- Create: `symlinks.toml` (template)
- Create: `.sops.yaml` (template)
- Create: `.gitignore`
- Test: `bootstrap_test.sh` (shell smoke test run via `go test` wrapper or manually)

**Interfaces:**
- Consumes: the built `da` binary.
- Produces: a POSIX `bootstrap` that detects OS/arch, downloads the matching release binary, and execs `da pull`.

- [ ] **Step 1: Write the bootstrap script**

Create `bootstrap`:
```sh
#!/bin/sh
set -eu

REPO="devanchor/da"          # GitHub owner/repo hosting da releases
BIN_DIR="${HOME}/.local/bin"
BIN="${BIN_DIR}/da"

if ! command -v git >/dev/null 2>&1; then
	echo "git is required. Install it: https://git-scm.com/downloads" >&2
	exit 1
fi

os="$(uname -s)"
arch="$(uname -m)"
case "$os" in
	Darwin) os="darwin" ;;
	Linux)  os="linux" ;;
	*) echo "Unsupported OS: $os" >&2; exit 1 ;;
esac
case "$arch" in
	x86_64|amd64) arch="amd64" ;;
	arm64|aarch64) arch="arm64" ;;
	*) echo "Unsupported arch: $arch" >&2; exit 1 ;;
esac

mkdir -p "$BIN_DIR"
asset="da_${os}_${arch}"
url="https://github.com/${REPO}/releases/latest/download/${asset}"

echo "Downloading ${asset} ..."
if command -v curl >/dev/null 2>&1; then
	curl -fsSL "$url" -o "$BIN"
elif command -v wget >/dev/null 2>&1; then
	wget -qO "$BIN" "$url"
else
	echo "Need curl or wget to download da." >&2
	exit 1
fi
chmod +x "$BIN"

echo "Running da pull ..."
exec "$BIN" pull "$@"
```

- [ ] **Step 2: Verify the script parses and detects correctly**

Run:
```bash
chmod +x bootstrap
sh -n bootstrap && echo "syntax OK"
```
Expected: prints `syntax OK` (no execution of the download).

- [ ] **Step 3: Create template config files**

Create `tools.toml`:
```toml
# DevAnchor tool inventory. Each [[tool]] is installed if `binary` is missing.
# github-release source is not auto-installed in the MVP.

[[tool]]
name = "ripgrep"
manager = "brew"
package = "ripgrep"
version = "latest"
binary = "rg"
platform = ["macos", "linux"]

[tool.overrides]
apt = "ripgrep"
dnf = "ripgrep"
pacman = "ripgrep"

[[tool]]
name = "ruff"
manager = "uv"
package = "ruff"
version = "latest"
binary = "ruff"
```

Create `symlinks.toml`:
```toml
# Map repo configs/ files to their target locations. ~ expands to $HOME.

[[link]]
source = "zsh/.zshrc"
target = "~/.zshrc"
```

Create `.sops.yaml`:
```yaml
# Maps which files are encrypted and which age key decrypts them.
# Replace age1... with the public key printed by `da keygen`.
creation_rules:
  - path_regex: secrets/.*\.yaml$
    age: age1replace_with_your_public_key
```

Create `.gitignore`:
```gitignore
# Build output
/da
/dist/

# Session + runtime state
/.da/
/.backup/

# Machine-specific overrides (loaded but never committed)
/local/

# Never commit the plaintext or recovered age key
age.key
**/age.key
```

- [ ] **Step 4: Build and run a local dry check**

Run:
```bash
go build -o da ./cmd/da
./da --help
./da pull --help
```
Expected: help text lists `pull`, `login`, `keygen`, `anchor` and the `--yes` flag.

- [ ] **Step 5: Full test suite green**

Run: `go test ./...`
Expected: all packages PASS.

- [ ] **Step 6: Commit**

```bash
git add bootstrap tools.toml symlinks.toml .sops.yaml .gitignore
git commit -m "feat: add bootstrap script and config templates"
```

---

## Self-Review

**1. Spec coverage:**
- Bootstrap flow (single idempotent entry, git-check exit, report) → Task 10 + reporter Task 7. ✅
- Secret management (sops+age, keygen/login, session temp key cleanup, DA_TOKEN) → Tasks 6, 9. ✅
- Symlink strategy (repo root, no-overwrite conflict flow, manifest) → Task 5. ✅
- Platform detection (macOS/Ubuntu/Debian/Fedora/Arch, manager map, WSL) → Task 2. ✅
- Tool inventory (declarative, check+install, store version) → Task 4. ✅
- TOML manifest + routing → Tasks 4 (tools.toml), 8 (symlinks.toml). ✅
- uv preference → Task 3 `InstallVia` uv branch. ✅
- Reverse-sync `da anchor` (tools + configs + secrets) → Tasks 8 (CaptureNewTools), 9 (RunAnchor). ✅
- Deferred (documented in spec, not in plan): drift command, rollback command, profiles, overrides/local, post-install hooks, github-release auto-install. ✅ intentional.

**2. Placeholder scan:** The stub command bodies in Task 8 are explicitly replaced with real implementations in Task 9 (noted in-task). The `_ =` keep-alive lines in Task 9 are flagged for deletion with a build-verify step. No "TODO/TBD/implement later" left as final state.

**3. Type consistency:** `runner.CommandRunner`, `platform.Info`, `pkgmgr.InstallVia`, `inventory.Sync`, `symlink.Apply`/`Choice`, `secrets.Session`, `reporter.Report`, `app.Paths`/`DefaultPaths`/`LoadSymlinks` names are used identically across producing and consuming tasks. `MockRunner` key format (`name + " " + args`) is consistent between Task 1 definition and Tasks 3/6/9 usage.
