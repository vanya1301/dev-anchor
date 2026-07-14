package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/devanchor/da/internal/app"
	"github.com/devanchor/da/internal/runner"
)

func main() {
	r := runner.ExecRunner{}

	var autoYes bool

	root := &cobra.Command{Use: "da", Short: "DevAnchor environment sync"}
	root.PersistentFlags().BoolVar(&autoYes, "yes", false, "non-interactive; default all conflict prompts to replace")

	pullCmd := &cobra.Command{
		Use:   "pull [profile]",
		Short: "Detect OS, install tools, symlink configs, decrypt secrets",
		RunE: func(cmd *cobra.Command, args []string) error {
			wd, _ := os.Getwd()
			paths := app.DefaultPaths(wd)
			rep, err := app.Pull(paths, r, autoYes)
			if err != nil {
				return err
			}
			fmt.Print(rep.String())
			return nil
		},
	}

	loginCmd := &cobra.Command{
		Use:   "login",
		Short: "Decrypt the age key for this session",
		RunE: func(cmd *cobra.Command, args []string) error {
			wd, _ := os.Getwd()
			paths := app.DefaultPaths(wd)
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
		Use:   "keygen",
		Short: "Generate and password-encrypt an age keypair",
		RunE: func(cmd *cobra.Command, args []string) error {
			wd, _ := os.Getwd()
			paths := app.DefaultPaths(wd)
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
		Use:   "anchor [path]",
		Short: "Capture local tools/configs/secrets back into the repo",
		RunE: func(cmd *cobra.Command, args []string) error {
			wd, _ := os.Getwd()
			paths := app.DefaultPaths(wd)
			newConfig := ""
			if len(args) > 0 {
				newConfig = args[0]
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
