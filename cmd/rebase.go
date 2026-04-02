package cmd

import (
	"fmt"

	"github.com/liebl/stack/internal/git"
	"github.com/liebl/stack/internal/meta"
	"github.com/spf13/cobra"
)

var rebaseCmd = &cobra.Command{
	Use:   "rebase",
	Short: "Rebase stack onto updated trunk",
	Long: `Pull latest trunk and rebase all tracked branches onto the new trunk tip.
Unlike sync, this does not detect or clean up merged branches.`,
	RunE: runRebase,
}

var (
	rebaseRemote string
	rebaseDryRun bool
)

func init() {
	rebaseCmd.Flags().StringVar(&rebaseRemote, "remote", "origin", "remote name")
	rebaseCmd.Flags().BoolVar(&rebaseDryRun, "dry-run", false, "show what would happen without doing it")
}

func runRebase(cmd *cobra.Command, args []string) error {
	trunk := meta.Trunk()

	if err := requireProvider(); err != nil {
		fmt.Printf("warning: %v (PR bases will not be updated)\n", err)
	}

	if err := recoverFromContinue(); err != nil {
		return err
	}

	pruned, err := meta.PruneStale()
	if err != nil {
		return err
	}
	for _, name := range pruned {
		fmt.Printf("  pruned stale branch: %s\n", name)
	}

	current, err := git.CurrentBranch()
	if err != nil {
		return err
	}
	if err := requireNotTrunk(current, trunk); err != nil {
		return err
	}

	all, err := meta.StackFromBranch(current)
	if err != nil {
		return err
	}
	if len(all) == 0 {
		return fmt.Errorf("no tracked branches in current stack")
	}

	origBranch, swState, err := syncPreamble(rebaseRemote, trunk, rebaseDryRun)
	if err != nil {
		return err
	}

	conflicted := false
	defer func() {
		if !conflicted && swState != nil {
			swState.Restore()
		}
		restoreCheckout(origBranch, rebaseDryRun)
	}()

	result, err := rebaseTrackedBranches(all, rebaseRemote, rebaseDryRun, host)
	if err != nil {
		if isConflictError(err) {
			conflicted = true
		}
		return err
	}

	if result.Rebased == 0 {
		fmt.Println("\nEverything is up to date.")
	} else {
		fmt.Printf("\nRebase complete. %d branches rebased.\n", result.Rebased)
	}
	return nil
}
