package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devanchor/da/internal/runner"
)

func TestResolveRepoEmptyReturnsError(t *testing.T) {
	if _, err := ResolveRepo("", t.TempDir(), &runner.MockRunner{}); err == nil {
		t.Fatal("expected error for empty repo arg")
	}
}

func TestResolveRepoLocalDirUsedAsIs(t *testing.T) {
	dir := t.TempDir()
	got, err := ResolveRepo(dir, t.TempDir(), &runner.MockRunner{})
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.Abs(dir)
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestResolveRepoRejectsGarbageArg(t *testing.T) {
	// Not an existing directory and doesn't look like a git remote — must
	// error rather than silently falling back to any implicit directory
	// (e.g. cwd).
	if _, err := ResolveRepo("not-a-path-or-url", t.TempDir(), &runner.MockRunner{}); err == nil {
		t.Fatal("expected error for garbage repo arg")
	}
}

func TestResolveRepoClonesRemote(t *testing.T) {
	cache := t.TempDir()
	remote := "git@github.com:me/dotfiles.git"
	m := &runner.MockRunner{}
	got, err := ResolveRepo(remote, cache, m)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, cache) {
		t.Fatalf("dest %q not under cache dir %q", got, cache)
	}
	if len(m.Calls) != 1 || m.Calls[0].Name != "git" {
		t.Fatalf("calls=%+v", m.Calls)
	}
	args := m.Calls[0].Args
	if len(args) != 3 || args[0] != "clone" || args[1] != remote || args[2] != got {
		t.Fatalf("clone args=%v", args)
	}
}

func TestResolveRepoUpdatesExistingClone(t *testing.T) {
	cache := t.TempDir()
	remote := "https://github.com/me/dotfiles.git"

	m1 := &runner.MockRunner{}
	dest, err := ResolveRepo(remote, cache, m1)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate the clone having actually happened on disk.
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}

	m2 := &runner.MockRunner{}
	got, err := ResolveRepo(remote, cache, m2)
	if err != nil {
		t.Fatal(err)
	}
	if got != dest {
		t.Fatalf("got %q want %q", got, dest)
	}
	if len(m2.Calls) != 1 || m2.Calls[0].Name != "git" {
		t.Fatalf("calls=%+v", m2.Calls)
	}
	args := m2.Calls[0].Args
	if len(args) != 4 || args[0] != "-C" || args[1] != dest || args[2] != "pull" || args[3] != "--ff-only" {
		t.Fatalf("pull args=%v", args)
	}
}

func TestDefaultRepoCacheDirRespectsXDG(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/xdg-data")
	got, err := DefaultRepoCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	if got != "/xdg-data/devanchor/repos" {
		t.Fatalf("got %q", got)
	}
}

func TestDefaultRepoCacheDirFallsBackToHome(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "")
	home, _ := os.UserHomeDir()
	got, err := DefaultRepoCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".local", "share", "devanchor", "repos")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
