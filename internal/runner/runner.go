package runner

import (
	"bytes"
	"os/exec"
	"strings"
)

type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

type Call struct {
	Name  string
	Args  []string
	Stdin string
}

type CommandRunner interface {
	Run(name string, args ...string) (Result, error)
	RunInput(stdin, name string, args ...string) (Result, error)
}

type ExecRunner struct{}

func (ExecRunner) Run(name string, args ...string) (Result, error) {
	return ExecRunner{}.RunInput("", name, args...)
}

func (ExecRunner) RunInput(stdin, name string, args ...string) (Result, error) {
	cmd := exec.Command(name, args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err := cmd.Run()
	res := Result{Stdout: out.String(), Stderr: errb.String()}
	if ee, ok := err.(*exec.ExitError); ok {
		res.ExitCode = ee.ExitCode()
		return res, nil
	}
	if err != nil {
		return res, err
	}
	return res, nil
}

type MockRunner struct {
	Calls     []Call
	Responses map[string]Result
	Errors    map[string]error
}

func (m *MockRunner) key(name string, args []string) string {
	return strings.TrimSpace(name + " " + strings.Join(args, " "))
}

func (m *MockRunner) Run(name string, args ...string) (Result, error) {
	return m.RunInput("", name, args...)
}

func (m *MockRunner) RunInput(stdin, name string, args ...string) (Result, error) {
	m.Calls = append(m.Calls, Call{Name: name, Args: args, Stdin: stdin})
	k := m.key(name, args)
	if m.Errors != nil {
		if e, ok := m.Errors[k]; ok {
			return Result{}, e
		}
	}
	if m.Responses != nil {
		if r, ok := m.Responses[k]; ok {
			return r, nil
		}
	}
	return Result{}, nil
}
