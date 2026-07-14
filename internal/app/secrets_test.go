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
