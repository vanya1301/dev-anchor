package app

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/term"

	"github.com/devanchor/da/internal/inventory"
	"github.com/devanchor/da/internal/platform"
	"github.com/devanchor/da/internal/reporter"
	"github.com/devanchor/da/internal/runner"
	"github.com/devanchor/da/internal/symlink"
)

// promptChoiceFrom reads a single conflict-resolution choice from in.
//
// If in hits EOF before any byte is read (no line at all — e.g. a piped,
// already-closed stdin from `curl | sh`), it MUST NOT default to a
// destructive choice: it returns Leave. Only an actual line read (even an
// empty one, meaning the user pressed Enter at a real prompt) falls
// through to the printed [recommended] default of CopyReplace.
func promptChoiceFrom(target string, in io.Reader) symlink.Choice {
	fmt.Printf("⚠  %s exists and is a real file (not a DevAnchor symlink).\n", target)
	fmt.Println("   a) leave as-is")
	fmt.Println("   b) copy into configs/, keep file")
	fmt.Println("   c) copy into configs/, replace with symlink  [recommended]")
	fmt.Print("choice [c]: ")
	sc := bufio.NewScanner(in)
	if !sc.Scan() {
		fmt.Println("\n(no input received; leaving file as-is)")
		return symlink.Leave
	}
	switch strings.TrimSpace(sc.Text()) {
	case "a":
		return symlink.Leave
	case "b":
		return symlink.CopyKeep
	default:
		return symlink.CopyReplace
	}
}

func promptChoice(target string) symlink.Choice {
	return promptChoiceFrom(target, os.Stdin)
}

// nonInteractivePrompt is used when stdin isn't a real terminal and --yes
// wasn't passed: we cannot safely ask the user anything, so we leave every
// conflicting file untouched rather than guessing.
func nonInteractivePrompt(target string) symlink.Choice {
	fmt.Printf("⚠  %s exists and stdin is not interactive; leaving as-is.\n", target)
	fmt.Println("   Re-run `da pull` from a real terminal to resolve, or pass --yes to replace.")
	return symlink.Leave
}

func Pull(paths Paths, r runner.CommandRunner, autoYes bool) (reporter.Report, error) {
	var rep reporter.Report

	info, err := platform.Detect()
	if err != nil {
		return rep, err
	}

	// tools
	toolBytes, err := os.ReadFile(paths.ToolsFile)
	if err == nil {
		inv, perr := inventory.Parse(toolBytes)
		if perr != nil {
			return rep, perr
		}
		installed, skipped, serr := inventory.Sync(inv, info, r, exec.LookPath)
		if serr != nil {
			return rep, serr
		}
		rep.InstalledTools = installed
		rep.SkippedTools = skipped
	}

	// symlinks
	linkBytes, err := os.ReadFile(paths.SymlinksFile)
	if err == nil {
		links, lerr := LoadSymlinks(linkBytes)
		if lerr != nil {
			return rep, lerr
		}
		var prompt symlink.Prompter
		switch {
		case autoYes:
			prompt = func(string) symlink.Choice { return symlink.CopyReplace }
		case !term.IsTerminal(int(os.Stdin.Fd())):
			prompt = nonInteractivePrompt
		default:
			prompt = promptChoice
		}
		res, entries, aerr := symlink.Apply(links, paths.Configs, paths.Backup, prompt)
		if aerr != nil {
			return rep, aerr
		}
		rep.SymlinksCreated = res.Created
		rep.SymlinksSkipped = res.Skipped
		rep.Conflicts = res.Conflicts
		rep.Adopted = res.Adopted
		existing, _ := symlink.LoadManifest(paths.ManifestFile)
		if serr := append(existing, entries...).Save(paths.ManifestFile); serr != nil {
			return rep, serr
		}
	}

	// secrets (best-effort; skipped if no .sops.yaml or no login)
	if _, statErr := os.Stat(paths.SopsFile); statErr == nil {
		pw, perr := ReadPassword("DevAnchor password: ")
		if perr == nil && pw != "" {
			if sess, tmp, lerr := RunLogin(paths, r, pw); lerr == nil {
				defer sess.Close()
				defer os.RemoveAll(tmp)
				if dec, derr := DecryptSecrets(paths, sess, tmp); derr == nil {
					rep.DecryptedFiles = dec
				}
			}
		}
	}

	return rep, nil
}
