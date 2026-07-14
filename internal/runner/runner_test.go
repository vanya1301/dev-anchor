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
