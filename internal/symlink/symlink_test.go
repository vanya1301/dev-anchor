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
