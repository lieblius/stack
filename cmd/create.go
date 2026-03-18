package cmd

import (
	"fmt"

	"github.com/liebl/stack/internal/git"
	"github.com/liebl/stack/internal/meta"
	"github.com/spf13/cobra"
)

var createCmd = &cobra.Command{
	Use:   "create <branch-name>",
	Short: "Create a new branch in the stack",
	Long: `Create a new branch stacked on the current branch. The new branch
is automatically tracked with the current branch as its parent.

Optionally commit staged changes with -m. Use -a to stage all changes first.`,
	Args: cobra.ExactArgs(1),
	RunE: runCreate,
}

var (
	createMessage string
	createAll     bool
)

func init() {
	createCmd.Flags().StringVarP(&createMessage, "message", "m", "", "commit staged changes with this message")
	createCmd.Flags().BoolVarP(&createAll, "all", "a", false, "stage all changes before committing (requires -m)")
}

func runCreate(cmd *cobra.Command, args []string) error {
	name := args[0]

	if git.BranchExists(name) {
		return fmt.Errorf("branch %s already exists", name)
	}

	if createAll && createMessage == "" {
		return fmt.Errorf("-a requires -m")
	}

	current, err := git.CurrentBranch()
	if err != nil {
		return err
	}

	base, err := git.SHA(current)
	if err != nil {
		return err
	}

	if _, err := git.Run("checkout", "-b", name); err != nil {
		return fmt.Errorf("creating branch: %w", err)
	}

	if createMessage != "" {
		commitArgs := []string{"commit"}
		if createAll {
			commitArgs = append(commitArgs, "-a")
		}
		commitArgs = append(commitArgs, "-m", createMessage)
		if _, err := git.Run(commitArgs...); err != nil {
			return fmt.Errorf("committing: %w", err)
		}
	}

	if err := meta.Set(name, current, base); err != nil {
		return err
	}

	fmt.Printf("Created %s (parent: %s)\n", name, current)
	return nil
}
