package cmd

import (
	"fmt"

	"github.com/liebl/stack/internal/git"
	"github.com/liebl/stack/internal/meta"
	"github.com/spf13/cobra"
)

var deleteCmd = &cobra.Command{
	Use:   "delete [branch]",
	Short: "Delete a branch and remove it from the stack",
	Long: `Delete a tracked branch, reparent its children to its parent,
and clean up metadata. If no branch is specified, deletes the current branch.`,
	Aliases: []string{"del"},
	Args:    cobra.MaximumNArgs(1),
	RunE:    runDelete,
}

var deleteRemote bool

func init() {
	deleteCmd.Flags().BoolVar(&deleteRemote, "remote", false, "also delete the remote branch")
}

func runDelete(cmd *cobra.Command, args []string) error {
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

	// If we're on this branch, checkout parent first
	current, _ := git.CurrentBranch()
	if current == branch {
		if err := git.Checkout(m.Parent); err != nil {
			return fmt.Errorf("checking out %s: %w", m.Parent, err)
		}
	}

	meta.Delete(branch)
	git.DeleteLocalBranch(branch)

	if deleteRemote {
		if err := git.DeleteRemoteBranch("origin", branch); err != nil {
			fmt.Printf("  warning: could not delete remote branch: %v\n", err)
		}
	}

	fmt.Printf("Deleted %s\n", branch)
	return nil
}
