package app

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/devanchor/da/internal/runner"
)

// ErrNoRepo is returned when the caller didn't supply a repo argument.
// da never falls back to the current directory: an implicit cwd repo is
// exactly how a real, unrelated file (a user's ~/.zshrc) once got adopted
// into this project's own template repo.
var ErrNoRepo = errors.New("no DevAnchor repo specified: pass a local path or a git URL (e.g. `da pull ~/dotfiles` or `da pull git@github.com:you/dotfiles.git`)")

// ResolveRepo turns a user-supplied repo argument into a local filesystem
// path da can operate on. It never falls back to any implicit directory.
//
//   - If arg names an existing local directory, its absolute path is
//     returned as-is.
//   - Otherwise arg must look like a git remote (git@, ssh://, git://,
//     http(s)://): it is cloned into cacheDir (keyed by a hash of the
//     remote URL) if not already present there, or updated with
//     `git pull --ff-only` if it is.
//   - Anything else is an error — da does not guess.
func ResolveRepo(arg string, cacheDir string, r runner.CommandRunner) (string, error) {
	if strings.TrimSpace(arg) == "" {
		return "", ErrNoRepo
	}

	if fi, err := os.Stat(arg); err == nil && fi.IsDir() {
		return filepath.Abs(arg)
	}

	if !looksLikeGitRemote(arg) {
		return "", fmt.Errorf("%q is not an existing local directory and doesn't look like a git URL", arg)
	}

	dest := filepath.Join(cacheDir, cloneDirName(arg))
	if fi, err := os.Stat(dest); err == nil && fi.IsDir() {
		res, err := r.Run("git", "-C", dest, "pull", "--ff-only")
		if err != nil {
			return "", err
		}
		if res.ExitCode != 0 {
			return "", fmt.Errorf("git pull in %s failed: %s", dest, res.Stderr)
		}
		return dest, nil
	}

	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", err
	}
	res, err := r.Run("git", "clone", arg, dest)
	if err != nil {
		return "", err
	}
	if res.ExitCode != 0 {
		return "", fmt.Errorf("git clone %s failed: %s", arg, res.Stderr)
	}
	return dest, nil
}

func looksLikeGitRemote(s string) bool {
	switch {
	case strings.HasPrefix(s, "git@"), strings.HasPrefix(s, "ssh://"), strings.HasPrefix(s, "git://"):
		return true
	case strings.HasPrefix(s, "http://"), strings.HasPrefix(s, "https://"):
		u, err := url.Parse(s)
		return err == nil && u.Host != ""
	default:
		return false
	}
}

// cloneDirName derives a stable, filesystem-safe directory name for a
// remote so repeated ResolveRepo calls for the same remote reuse the same
// clone instead of re-cloning.
func cloneDirName(remote string) string {
	name := strings.TrimSuffix(filepath.Base(remote), ".git")
	sum := sha256.Sum256([]byte(remote))
	return fmt.Sprintf("%s-%x", name, sum[:4])
}

// DefaultRepoCacheDir is where remote repos passed to da are cloned.
func DefaultRepoCacheDir() (string, error) {
	if x := os.Getenv("XDG_DATA_HOME"); x != "" {
		return filepath.Join(x, "devanchor", "repos"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "devanchor", "repos"), nil
}
