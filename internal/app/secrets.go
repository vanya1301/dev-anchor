package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/term"
	"gopkg.in/yaml.v3"

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
	return rep, nil
}
