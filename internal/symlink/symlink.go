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
