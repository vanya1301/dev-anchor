package reporter

import (
	"fmt"
	"strings"
)

type Report struct {
	InstalledTools  []string
	SkippedTools    []string
	SymlinksCreated []string
	SymlinksSkipped []string
	Conflicts       []string
	Adopted         []string
	DecryptedFiles  []string
}

func section(b *strings.Builder, title string, items []string) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(b, "%s (%d):\n", title, len(items))
	for _, it := range items {
		fmt.Fprintf(b, "  - %s\n", it)
	}
}

func (r Report) String() string {
	var b strings.Builder
	section(&b, "Installed tools", r.InstalledTools)
	section(&b, "Already present", r.SkippedTools)
	section(&b, "Symlinks created", r.SymlinksCreated)
	section(&b, "Symlinks skipped", r.SymlinksSkipped)
	section(&b, "Adopted configs", r.Adopted)
	section(&b, "Conflicts", r.Conflicts)
	section(&b, "Decrypted files", r.DecryptedFiles)
	if b.Len() == 0 {
		return "No changes.\n"
	}
	return b.String()
}
