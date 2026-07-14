package app

import (
	"strings"
	"testing"

	"github.com/devanchor/da/internal/symlink"
)

func TestPromptChoiceFromEOFLeavesFile(t *testing.T) {
	// Simulates a piped, non-interactive stdin that is already closed
	// (e.g. `curl | sh` handing an empty/closed pipe to the exec'd binary).
	// Must NEVER default to a destructive choice.
	got := promptChoiceFrom("/home/u/.zshrc", strings.NewReader(""))
	if got != symlink.Leave {
		t.Fatalf("EOF on stdin must default to Leave, got %v", got)
	}
}

func TestPromptChoiceFromEmptyLineDefaultsToReplace(t *testing.T) {
	// A real interactive user pressing Enter (an actual newline byte was
	// read, not EOF) accepts the printed [recommended] default.
	got := promptChoiceFrom("/home/u/.zshrc", strings.NewReader("\n"))
	if got != symlink.CopyReplace {
		t.Fatalf("explicit empty line should default to CopyReplace, got %v", got)
	}
}

func TestPromptChoiceFromLeave(t *testing.T) {
	got := promptChoiceFrom("/home/u/.zshrc", strings.NewReader("a\n"))
	if got != symlink.Leave {
		t.Fatalf("got %v", got)
	}
}

func TestPromptChoiceFromCopyKeep(t *testing.T) {
	got := promptChoiceFrom("/home/u/.zshrc", strings.NewReader("b\n"))
	if got != symlink.CopyKeep {
		t.Fatalf("got %v", got)
	}
}
