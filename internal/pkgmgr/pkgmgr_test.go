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
