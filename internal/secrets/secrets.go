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
