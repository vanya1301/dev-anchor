package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/devanchor/da/internal/app"
	"github.com/devanchor/da/internal/runner"
)

// resolveRepoArg turns the required <repo> positional argument into a
// local filesystem path, cloning it first if it's a remote git URL. da
// never falls back to the current directory.
func resolveRepoArg(arg string, r runner.CommandRunner) (string, error) {
	cacheDir, err := app.DefaultRepoCacheDir()
	if err != nil {
		return "", err
	}
	return app.ResolveRepo(arg, cacheDir, r)
}

func main() {
	r := runner.ExecRunner{}

	var autoYes bool

	root := &cobra.Command{Use: "da", Short: "DevAnchor environment sync"}
	root.PersistentFlags().BoolVar(&autoYes, "yes", false, "non-interactive; default all conflict prompts to replace")

	pullCmd := &cobra.Command{
		Use:   "pull <repo>",
		Short: "Detect OS, install tools, symlink configs, decrypt secrets",
		Long: "Detect OS, install tools, symlink configs, decrypt secrets.\n\n" +
			"<repo> is required: a local path to your DevAnchor repo, or a git\n" +
			"URL to clone (cached under $XDG_DATA_HOME/devanchor/repos, or\n" +
			"~/.local/share/devanchor/repos). da never operates on the current\n" +
			"directory implicitly.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repoPath, err := resolveRepoArg(args[0], r)
			if err != nil {
				return err
			}
			paths := app.DefaultPaths(repoPath)
			rep, err := app.Pull(paths, r, autoYes)
			if err != nil {
				return err
			}
			fmt.Print(rep.String())
			return nil
		},
	}

	loginCmd := &cobra.Command{
		Use:   "login <repo>",
		Short: "Decrypt the age key for this session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repoPath, err := resolveRepoArg(args[0], r)
			if err != nil {
				return err
			}
			paths := app.DefaultPaths(repoPath)
			pw, err := app.ReadPassword("DevAnchor password: ")
			if err != nil {
				return err
			}
			sess, tmp, err := app.RunLogin(paths, r, pw)
			if err != nil {
				return err
			}
			defer sess.Close()
			defer os.RemoveAll(tmp)
			fmt.Println("login OK; session key established")
			return nil
		},
	}

	keygenCmd := &cobra.Command{
		Use:   "keygen <repo>",
		Short: "Generate and password-encrypt an age keypair",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repoPath, err := resolveRepoArg(args[0], r)
			if err != nil {
				return err
			}
			paths := app.DefaultPaths(repoPath)
			pw, err := app.ReadPassword("New DevAnchor password: ")
			if err != nil {
				return err
			}
			if err := app.RunKeygen(paths, r, pw); err != nil {
				return err
			}
			fmt.Println("Generated age.pub and age.key.enc. Commit them: git add age.pub age.key.enc .sops.yaml")
			return nil
		},
	}

	anchorCmd := &cobra.Command{
		Use:   "anchor <repo> [config-path]",
		Short: "Capture local tools/configs/secrets back into the repo",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			repoPath, err := resolveRepoArg(args[0], r)
			if err != nil {
				return err
			}
			paths := app.DefaultPaths(repoPath)
			newConfig := ""
			if len(args) > 1 {
				newConfig = args[1]
			}
			rep, err := app.RunAnchor(paths, r, nil, newConfig)
			if err != nil {
				return err
			}
			fmt.Print(rep.String())
			fmt.Println("Review with git diff, then git commit && git push")
			return nil
		},
	}

	root.AddCommand(pullCmd, anchorCmd, loginCmd, keygenCmd)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
