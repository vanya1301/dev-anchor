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
