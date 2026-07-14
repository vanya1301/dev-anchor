package reporter

import (
	"strings"
	"testing"
)

func TestReportStringIncludesCounts(t *testing.T) {
	r := Report{
		InstalledTools:  []string{"ruff"},
		SymlinksCreated: []string{"/home/u/.zshrc"},
		Conflicts:       []string{"/home/u/.vimrc"},
	}
	out := r.String()
	if !strings.Contains(out, "Installed tools (1)") {
		t.Fatalf("missing installed section: %s", out)
	}
	if !strings.Contains(out, "ruff") {
		t.Fatalf("missing tool name: %s", out)
	}
	if !strings.Contains(out, "Conflicts (1)") {
		t.Fatalf("missing conflicts: %s", out)
	}
}

func TestReportOmitsEmpty(t *testing.T) {
	r := Report{InstalledTools: []string{"x"}}
	out := r.String()
	if strings.Contains(out, "Conflicts") {
		t.Fatalf("should omit empty conflicts: %s", out)
	}
}
