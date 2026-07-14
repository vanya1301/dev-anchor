package app

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/devanchor/da/internal/inventory"
	"github.com/devanchor/da/internal/platform"
	"github.com/devanchor/da/internal/reporter"
	"github.com/devanchor/da/internal/runner"
	"github.com/devanchor/da/internal/symlink"
)

func promptChoice(target string) symlink.Choice {
	fmt.Printf("⚠  %s exists and is a real file (not a DevAnchor symlink).\n", target)
	fmt.Println("   a) leave as-is")
	fmt.Println("   b) copy into configs/, keep file")
	fmt.Println("   c) copy into configs/, replace with symlink  [recommended]")
	fmt.Print("choice [c]: ")
	sc := bufio.NewScanner(os.Stdin)
	sc.Scan()
	switch strings.TrimSpace(sc.Text()) {
	case "a":
		return symlink.Leave
	case "b":
		return symlink.CopyKeep
	default:
		return symlink.CopyReplace
	}
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
		prompt := promptChoice
		if autoYes {
			prompt = func(string) symlink.Choice { return symlink.CopyReplace }
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
