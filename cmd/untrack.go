package cmd

import (
	"fmt"

	"github.com/liebl/stack/internal/git"
	"github.com/liebl/stack/internal/meta"
	"github.com/spf13/cobra"
)

var untrackCmd = &cobra.Command{
	Use:   "untrack [branch]",
	Short: "Remove a branch from the stack without deleting it",
	Long: `Stop tracking a branch, reparent its children to its parent,
but keep the git branch intact. If no branch is specified, untracks the current branch.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runUntrack,
}

func runUntrack(cmd *cobra.Command, args []string) error {
	var branch string
	if len(args) > 0 {
		branch = args[0]
	} else {
		var err error
		branch, err = git.CurrentBranch()
		if err != nil {
			return err
		}
	}

	m, err := meta.Get(branch)
	if err != nil {
		return err
	}
	if m == nil {
		return fmt.Errorf("branch %s is not tracked", branch)
	}

	all, err := meta.AllTracked()
	if err != nil {
		return err
	}

	// Reparent children
	children := meta.Children(branch, all)
	for _, child := range children {
		if err := meta.SetParent(child, m.Parent); err != nil {
			return fmt.Errorf("reparenting %s: %w", child, err)
		}
		fmt.Printf("  reparented %s -> %s\n", child, m.Parent)
	}

	meta.Delete(branch)
	fmt.Printf("Untracked %s (branch preserved)\n", branch)
	return nil
}
